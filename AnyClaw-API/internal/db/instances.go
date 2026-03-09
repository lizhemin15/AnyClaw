package db

import (
	"database/sql"
	"fmt"
	"strconv"
)

type Instance struct {
	ID               int64   `json:"id"`
	UserID           int64   `json:"user_id"`
	Name             string  `json:"name"`
	Status           string  `json:"status"`
	Energy           int     `json:"energy"`
	DailyConsume     int     `json:"daily_consume"`
	ZeroEnergySince  *string `json:"zero_energy_since,omitempty"`
	ContainerID      string  `json:"container_id,omitempty"`
	HostID           string  `json:"host_id,omitempty"`
	Token            string  `json:"-"` // never expose to client
	CreatedAt        string  `json:"created_at"`
}

func (d *DB) CreateInstance(userID int64, name, token string, initialEnergy int) (*Instance, error) {
	if initialEnergy <= 0 {
		initialEnergy = 100
	}
	res, err := d.Exec(
		"INSERT INTO instances (user_id, name, status, token, energy, daily_consume) VALUES (?, ?, 'creating', ?, ?, 10)",
		userID, name, token, initialEnergy,
	)
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}
	id, _ := res.LastInsertId()
	return d.GetInstanceByID(id)
}

func (d *DB) GetInstanceByID(id int64) (*Instance, error) {
	var i Instance
	var zeroSince sql.NullString
	err := d.QueryRow(
		`SELECT id, user_id, name, status, COALESCE(energy,100), COALESCE(daily_consume,10), zero_energy_since,
		 COALESCE(container_id,''), COALESCE(host_id,''), token, created_at FROM instances WHERE id = ?`,
		id,
	).Scan(&i.ID, &i.UserID, &i.Name, &i.Status, &i.Energy, &i.DailyConsume, &zeroSince, &i.ContainerID, &i.HostID, &i.Token, &i.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if zeroSince.Valid {
		i.ZeroEnergySince = &zeroSince.String
	}
	return &i, nil
}

func (d *DB) GetInstanceByToken(token string) (*Instance, error) {
	var i Instance
	var zeroSince sql.NullString
	err := d.QueryRow(
		`SELECT id, user_id, name, status, COALESCE(energy,100), COALESCE(daily_consume,10), zero_energy_since,
		 COALESCE(container_id,''), COALESCE(host_id,''), token, created_at FROM instances WHERE token = ?`,
		token,
	).Scan(&i.ID, &i.UserID, &i.Name, &i.Status, &i.Energy, &i.DailyConsume, &zeroSince, &i.ContainerID, &i.HostID, &i.Token, &i.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if zeroSince.Valid {
		i.ZeroEnergySince = &zeroSince.String
	}
	return &i, nil
}

func (d *DB) ListInstancesByUserID(userID int64) ([]*Instance, error) {
	rows, err := d.Query(
		`SELECT id, user_id, name, status, COALESCE(energy,100), COALESCE(daily_consume,10), zero_energy_since,
		 COALESCE(container_id,''), COALESCE(host_id,''), token, created_at FROM instances WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Instance
	for rows.Next() {
		var i Instance
		var zeroSince sql.NullString
		if err := rows.Scan(&i.ID, &i.UserID, &i.Name, &i.Status, &i.Energy, &i.DailyConsume, &zeroSince, &i.ContainerID, &i.HostID, &i.Token, &i.CreatedAt); err != nil {
			return nil, err
		}
		if zeroSince.Valid {
			i.ZeroEnergySince = &zeroSince.String
		}
		list = append(list, &i)
	}
	return list, nil
}

func (d *DB) UpdateInstanceStatus(id int64, status string) error {
	_, err := d.Exec("UPDATE instances SET status = ? WHERE id = ?", status, id)
	return err
}

func (d *DB) UpdateInstanceContainer(id int64, containerID, hostID string) error {
	_, err := d.Exec("UPDATE instances SET container_id = ?, host_id = ?, status = 'running' WHERE id = ?", containerID, hostID, id)
	return err
}

func (d *DB) DeleteInstance(id int64) error {
	_, err := d.Exec("DELETE FROM instances WHERE id = ?", id)
	return err
}

func (d *DB) DeductInstanceEnergy(id int64, amount int) (bool, error) {
	res, err := d.Exec("UPDATE instances SET energy = energy - ?, zero_energy_since = CASE WHEN energy - ? <= 0 THEN COALESCE(zero_energy_since, NOW()) ELSE NULL END WHERE id = ? AND energy >= ?", amount, amount, id, amount)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (d *DB) AddInstanceEnergy(id int64, amount int) error {
	_, err := d.Exec("UPDATE instances SET energy = energy + ?, zero_energy_since = NULL WHERE id = ?", amount, id)
	return err
}

func (d *DB) RunDailyConsume() error {
	_, err := d.Exec(`UPDATE instances SET 
		energy = GREATEST(0, COALESCE(energy,100) - COALESCE(daily_consume, 10)),
		zero_energy_since = CASE WHEN COALESCE(energy,100) - COALESCE(daily_consume, 10) <= 0 THEN COALESCE(zero_energy_since, NOW()) ELSE NULL END
		WHERE status = 'running'`)
	return err
}

func (d *DB) DeleteInstancesZeroOverDays(days int) (int64, error) {
	res, err := d.Exec("DELETE FROM instances WHERE zero_energy_since IS NOT NULL AND zero_energy_since < DATE_SUB(NOW(), INTERVAL ? DAY)", days)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ResolveToken returns instanceID and userID for a valid token, for LLM proxy.
func (d *DB) ResolveToken(token string) (instanceID, userID string, ok bool) {
	inst, err := d.GetInstanceByToken(token)
	if err != nil || inst == nil {
		return "", "", false
	}
	return strconv.FormatInt(inst.ID, 10), strconv.FormatInt(inst.UserID, 10), true
}
