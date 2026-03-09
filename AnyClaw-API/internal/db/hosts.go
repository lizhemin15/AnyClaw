package db

import (
	"database/sql"
)

type Host struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Addr        string  `json:"addr"`
	SSHPort     int     `json:"ssh_port"`
	SSHUser     string  `json:"ssh_user"`
	SSHKey      string  `json:"-"` // never expose
	SSHPassword string  `json:"-"` // never expose
	DockerImage string  `json:"docker_image,omitempty"`
	Enabled     bool    `json:"enabled"`
	Status      string  `json:"status"`
	LastCheckAt *string `json:"last_check_at,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

func (d *DB) CreateHost(h *Host) error {
	_, err := d.Exec(
		`INSERT INTO hosts (id, name, addr, ssh_port, ssh_user, ssh_key, ssh_password, docker_image, enabled, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		h.ID, h.Name, h.Addr, h.SSHPort, h.SSHUser, h.SSHKey, h.SSHPassword, h.DockerImage,
		enabledInt(h.Enabled), h.Status,
	)
	return err
}

func (d *DB) GetHost(id string) (*Host, error) {
	var h Host
	var sshKey, sshPass sql.NullString
	var dockerImage sql.NullString
	var lastCheck sql.NullString
	var enabled int
	err := d.QueryRow(
		`SELECT id, name, addr, ssh_port, ssh_user, ssh_key, ssh_password, docker_image, enabled, status, last_check_at, created_at
		 FROM hosts WHERE id = ?`,
		id,
	).Scan(&h.ID, &h.Name, &h.Addr, &h.SSHPort, &h.SSHUser, &sshKey, &sshPass, &dockerImage,
		&enabled, &h.Status, &lastCheck, &h.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if sshKey.Valid {
		h.SSHKey = sshKey.String
	}
	if sshPass.Valid {
		h.SSHPassword = sshPass.String
	}
	if dockerImage.Valid {
		h.DockerImage = dockerImage.String
	}
	if lastCheck.Valid {
		h.LastCheckAt = &lastCheck.String
	}
	h.Enabled = enabled != 0
	return &h, nil
}

func (d *DB) ListHosts() ([]*Host, error) {
	rows, err := d.Query(
		`SELECT id, name, addr, ssh_port, ssh_user, docker_image, enabled, status, last_check_at, created_at
		 FROM hosts ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Host
	for rows.Next() {
		var h Host
		var dockerImage sql.NullString
		var lastCheck sql.NullString
		var enabled int
		if err := rows.Scan(&h.ID, &h.Name, &h.Addr, &h.SSHPort, &h.SSHUser, &dockerImage, &enabled, &h.Status, &lastCheck, &h.CreatedAt); err != nil {
			return nil, err
		}
		h.Enabled = enabled != 0
		if dockerImage.Valid {
			h.DockerImage = dockerImage.String
		}
		if lastCheck.Valid {
			h.LastCheckAt = &lastCheck.String
		}
		list = append(list, &h)
	}
	return list, nil
}

func (d *DB) ListEnabledHosts() ([]*Host, error) {
	rows, err := d.Query(
		`SELECT id, name, addr, ssh_port, ssh_user, ssh_key, ssh_password, docker_image, enabled, status
		 FROM hosts WHERE enabled = 1 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*Host
	for rows.Next() {
		var h Host
		var sshKey, sshPass sql.NullString
		var dockerImage sql.NullString
		var enabled int
		if err := rows.Scan(&h.ID, &h.Name, &h.Addr, &h.SSHPort, &h.SSHUser, &sshKey, &sshPass, &dockerImage, &enabled, &h.Status); err != nil {
			return nil, err
		}
		h.Enabled = true
		if sshKey.Valid {
			h.SSHKey = sshKey.String
		}
		if sshPass.Valid {
			h.SSHPassword = sshPass.String
		}
		if dockerImage.Valid {
			h.DockerImage = dockerImage.String
		}
		list = append(list, &h)
	}
	return list, nil
}

func (d *DB) UpdateHost(h *Host) error {
	_, err := d.Exec(
		`UPDATE hosts SET name=?, addr=?, ssh_port=?, ssh_user=?, ssh_key=?, ssh_password=?, docker_image=?, enabled=?
		 WHERE id = ?`,
		h.Name, h.Addr, h.SSHPort, h.SSHUser, h.SSHKey, h.SSHPassword, h.DockerImage, enabledInt(h.Enabled), h.ID,
	)
	return err
}

func (d *DB) UpdateHostStatus(id, status string) error {
	_, err := d.Exec(
		`UPDATE hosts SET status=?, last_check_at=CURRENT_TIMESTAMP WHERE id=?`,
		status, id,
	)
	return err
}

func (d *DB) UpdateHostNoKey(h *Host) error {
	_, err := d.Exec(
		`UPDATE hosts SET name=?, addr=?, ssh_port=?, ssh_user=?, docker_image=?, enabled=?
		 WHERE id = ?`,
		h.Name, h.Addr, h.SSHPort, h.SSHUser, h.DockerImage, enabledInt(h.Enabled), h.ID,
	)
	return err
}

func (d *DB) DeleteHost(id string) error {
	_, err := d.Exec("DELETE FROM hosts WHERE id = ?", id)
	return err
}

func enabledInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
