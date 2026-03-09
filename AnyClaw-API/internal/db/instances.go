package db

import (
	"database/sql"
	"fmt"
	"strconv"
)

type Instance struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	ContainerID string `json:"container_id,omitempty"`
	HostID      string `json:"host_id,omitempty"`
	Token       string `json:"-"` // never expose to client
	CreatedAt   string `json:"created_at"`
}

func (d *DB) CreateInstance(userID int64, name, token string) (*Instance, error) {
	res, err := d.Exec(
		"INSERT INTO instances (user_id, name, status, token) VALUES (?, ?, 'creating', ?)",
		userID, name, token,
	)
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}
	id, _ := res.LastInsertId()
	return d.GetInstanceByID(id)
}

func (d *DB) GetInstanceByID(id int64) (*Instance, error) {
	var i Instance
	err := d.QueryRow(
		"SELECT id, user_id, name, status, COALESCE(container_id,''), COALESCE(host_id,''), token, created_at FROM instances WHERE id = ?",
		id,
	).Scan(&i.ID, &i.UserID, &i.Name, &i.Status, &i.ContainerID, &i.HostID, &i.Token, &i.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (d *DB) GetInstanceByToken(token string) (*Instance, error) {
	var i Instance
	err := d.QueryRow(
		"SELECT id, user_id, name, status, COALESCE(container_id,''), COALESCE(host_id,''), token, created_at FROM instances WHERE token = ?",
		token,
	).Scan(&i.ID, &i.UserID, &i.Name, &i.Status, &i.ContainerID, &i.HostID, &i.Token, &i.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (d *DB) ListInstancesByUserID(userID int64) ([]*Instance, error) {
	rows, err := d.Query(
		"SELECT id, user_id, name, status, COALESCE(container_id,''), COALESCE(host_id,''), token, created_at FROM instances WHERE user_id = ? ORDER BY created_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Instance
	for rows.Next() {
		var i Instance
		if err := rows.Scan(&i.ID, &i.UserID, &i.Name, &i.Status, &i.ContainerID, &i.HostID, &i.Token, &i.CreatedAt); err != nil {
			return nil, err
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

// ResolveToken returns instanceID and userID for a valid token, for LLM proxy.
func (d *DB) ResolveToken(token string) (instanceID, userID string, ok bool) {
	inst, err := d.GetInstanceByToken(token)
	if err != nil || inst == nil {
		return "", "", false
	}
	return strconv.FormatInt(inst.ID, 10), strconv.FormatInt(inst.UserID, 10), true
}
