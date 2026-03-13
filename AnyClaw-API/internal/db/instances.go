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
	Unread           bool    `json:"unread,omitempty"` // 是否有未读的 AI 回复
	SubscribedMonth  string  `json:"subscribed_month,omitempty"` // 已包月月份，如 "2025-03"，空表示未包月
}

func (d *DB) CreateInstance(userID int64, name, token string, initialEnergy int, dailyConsume int) (*Instance, error) {
	if initialEnergy < 0 {
		initialEnergy = 100
	}
	if dailyConsume <= 0 {
		dailyConsume = 10
	}
	res, err := d.Exec(
		"INSERT INTO instances (user_id, name, status, token, energy, daily_consume) VALUES (?, ?, 'creating', ?, ?, ?)",
		userID, name, token, initialEnergy, dailyConsume,
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

func (d *DB) CountInstancesByUserID(userID int64) (int, error) {
	var n int
	err := d.QueryRow("SELECT COUNT(*) FROM instances WHERE user_id = ?", userID).Scan(&n)
	return n, err
}

func (d *DB) ListRunningInstances() ([]*Instance, error) {
	rows, err := d.Query(
		`SELECT id, user_id, name, status, COALESCE(energy,100), COALESCE(daily_consume,10), zero_energy_since,
		 COALESCE(container_id,''), COALESCE(host_id,''), token, created_at FROM instances WHERE status = 'running'`,
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

// AdminInstance 管理员查看的实例，含用户邮箱和宿主机名
type AdminInstance struct {
	Instance
	UserEmail string `json:"user_email"`
	HostName  string `json:"host_name"`
}

// CountRunningInstancesByHostID 返回指定宿主机上运行中的实例数量
func (d *DB) CountRunningInstancesByHostID(hostID string) (int, error) {
	if hostID == "" {
		return 0, nil
	}
	var n int
	err := d.QueryRow("SELECT COUNT(*) FROM instances WHERE host_id = ? AND status = 'running'", hostID).Scan(&n)
	return n, err
}

// ListRunningInstancesByHostID 返回指定宿主机上运行中的实例
func (d *DB) ListRunningInstancesByHostID(hostID string) ([]*Instance, error) {
	if hostID == "" {
		return nil, nil
	}
	rows, err := d.Query(
		`SELECT i.id, i.user_id, i.name, i.status, COALESCE(i.energy,100), COALESCE(i.daily_consume,10), i.zero_energy_since,
		 COALESCE(i.container_id,''), COALESCE(i.host_id,''), i.token, i.created_at,
		 0 as unread
		 FROM instances i
		 WHERE i.host_id = ? AND i.status = 'running'`,
		hostID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Instance
	for rows.Next() {
		var i Instance
		var zeroSince sql.NullString
		if err := rows.Scan(&i.ID, &i.UserID, &i.Name, &i.Status, &i.Energy, &i.DailyConsume, &zeroSince,
			&i.ContainerID, &i.HostID, &i.Token, &i.CreatedAt, &i.Unread); err != nil {
			return nil, err
		}
		if zeroSince.Valid {
			i.ZeroEnergySince = &zeroSince.String
		}
		list = append(list, &i)
	}
	return list, nil
}

func (d *DB) ListAllInstancesAdmin() ([]*AdminInstance, error) {
	rows, err := d.Query(
		`SELECT i.id, i.user_id, i.name, i.status, COALESCE(i.energy,100), COALESCE(i.daily_consume,10), i.zero_energy_since,
		 i.container_id, i.host_id, i.created_at,
		 COALESCE(u.email,'') as user_email,
		 COALESCE(h.name,'') as host_name
		 FROM instances i
		 LEFT JOIN users u ON i.user_id = u.id
		 LEFT JOIN hosts h ON i.host_id = h.id
		 ORDER BY i.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*AdminInstance
	for rows.Next() {
		var a AdminInstance
		var zeroSince sql.NullString
		if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.Status, &a.Energy, &a.DailyConsume, &zeroSince,
			&a.ContainerID, &a.HostID, &a.CreatedAt, &a.UserEmail, &a.HostName); err != nil {
			return nil, err
		}
		if zeroSince.Valid {
			a.ZeroEnergySince = &zeroSince.String
		}
		list = append(list, &a)
	}
	return list, nil
}

func (d *DB) ListInstancesByUserID(userID int64) ([]*Instance, error) {
	rows, err := d.Query(
		`SELECT i.id, i.user_id, i.name, i.status, COALESCE(i.energy,100), COALESCE(i.daily_consume,10), i.zero_energy_since,
		 COALESCE(i.container_id,''), COALESCE(i.host_id,''), i.token, i.created_at,
		 (SELECT COUNT(*) > 0 FROM messages m WHERE m.instance_id = i.id AND m.role='assistant'
		  AND (i.last_read_at IS NULL OR m.created_at > i.last_read_at)) as unread
		 FROM instances i WHERE i.user_id = ? ORDER BY i.created_at DESC`,
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
		if err := rows.Scan(&i.ID, &i.UserID, &i.Name, &i.Status, &i.Energy, &i.DailyConsume, &zeroSince, &i.ContainerID, &i.HostID, &i.Token, &i.CreatedAt, &i.Unread); err != nil {
			return nil, err
		}
		if zeroSince.Valid {
			i.ZeroEnergySince = &zeroSince.String
		}
		list = append(list, &i)
	}
	return list, nil
}

func (d *DB) UpdateInstanceLastRead(instanceID, userID int64) error {
	_, err := d.Exec("UPDATE instances SET last_read_at = NOW() WHERE id = ? AND user_id = ?", instanceID, userID)
	return err
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
	_, _ = d.Exec("DELETE FROM instance_subscriptions WHERE instance_id = ?", id)
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
