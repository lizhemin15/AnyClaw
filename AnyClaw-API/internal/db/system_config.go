package db

import (
	"database/sql"
	"encoding/json"
)

const adminConfigKey = "admin_config"

// GetAdminConfigJSON 从 DB 读取管理配置 JSON，不存在返回 nil
func (d *DB) GetAdminConfigJSON() ([]byte, error) {
	var v string
	err := d.QueryRow("SELECT v FROM system_config WHERE k = ?", adminConfigKey).Scan(&v)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte(v), nil
}

// SaveAdminConfigJSON 将管理配置 JSON 写入 DB
func (d *DB) SaveAdminConfigJSON(data []byte) error {
	_, err := d.Exec(
		`INSERT INTO system_config (k, v) VALUES (?, ?) ON DUPLICATE KEY UPDATE v = VALUES(v)`,
		adminConfigKey, string(data),
	)
	return err
}

// AdminConfigData 管理配置结构（channels/smtp/payment/energy）
type AdminConfigData struct {
	Channels []any `json:"channels"`
	SMTP     any   `json:"smtp,omitempty"`
	Payment  any   `json:"payment,omitempty"`
	Energy   any   `json:"energy,omitempty"`
}

// GetAdminConfigFromDB 从 DB 读取并解析管理配置
func (d *DB) GetAdminConfigFromDB() (*AdminConfigData, error) {
	b, err := d.GetAdminConfigJSON()
	if err != nil || b == nil {
		return nil, err
	}
	var out AdminConfigData
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
