package db

import (
	"database/sql"
	"time"
)

// HostPullTask 宿主机「拉取镜像并重启实例」后台任务
type HostPullTask struct {
	ID            int64     `json:"id"`
	HostID        string    `json:"host_id"`
	Status        string    `json:"status"` // pending, running, succeeded, failed
	Phase         string    `json:"phase"`  // pulling, restarting, pruning, done
	Message       string    `json:"message"`
	InstanceTotal int       `json:"instance_total"`
	InstanceDone  int       `json:"instance_done"`
	FailedJSON    string    `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (d *DB) InsertHostPullTask(hostID string) (int64, error) {
	res, err := d.Exec(
		`INSERT INTO host_pull_tasks (host_id, status, phase, message, instance_total, instance_done)
		 VALUES (?, 'pending', '', '', 0, 0)`,
		hostID,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return id, err
}

func (d *DB) GetHostPullTask(id int64) (*HostPullTask, error) {
	var t HostPullTask
	var failed sql.NullString
	err := d.QueryRow(
		`SELECT id, host_id, status, phase, message, instance_total, instance_done, failed_json, created_at, updated_at
		 FROM host_pull_tasks WHERE id = ?`,
		id,
	).Scan(&t.ID, &t.HostID, &t.Status, &t.Phase, &t.Message, &t.InstanceTotal, &t.InstanceDone, &failed, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if failed.Valid {
		t.FailedJSON = failed.String
	}
	return &t, nil
}

// ActiveHostPullTaskID 若该宿主机存在未完成的拉取任务则返回任务 id
func (d *DB) ActiveHostPullTaskID(hostID string) (int64, bool, error) {
	var id int64
	err := d.QueryRow(
		`SELECT id FROM host_pull_tasks WHERE host_id = ? AND status IN ('pending', 'running') ORDER BY id DESC LIMIT 1`,
		hostID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (d *DB) UpdateHostPullTask(id int64, status, phase, message string, total, done int) error {
	_, err := d.Exec(
		`UPDATE host_pull_tasks SET status = ?, phase = ?, message = ?, instance_total = ?, instance_done = ? WHERE id = ?`,
		status, phase, message, total, done, id,
	)
	return err
}

func (d *DB) FinishHostPullTask(id int64, status string, phase, message string, total, done int, failedJSON string) error {
	_, err := d.Exec(
		`UPDATE host_pull_tasks SET status = ?, phase = ?, message = ?, instance_total = ?, instance_done = ?, failed_json = ? WHERE id = ?`,
		status, phase, message, total, done, nullIfEmpty(failedJSON), id,
	)
	return err
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
