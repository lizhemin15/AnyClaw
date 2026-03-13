package db

import (
	"database/sql"
	"fmt"
	"time"
)

// IsInstanceSubscribed 检查实例是否在包月有效期内（expires_at > now）
func (d *DB) IsInstanceSubscribed(instanceID int64) (bool, error) {
	var expiresAt string
	err := d.QueryRow(
		"SELECT expires_at FROM instance_subscriptions WHERE instance_id = ? AND expires_at > NOW() LIMIT 1",
		instanceID,
	).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check subscription: %w", err)
	}
	return true, nil
}

// SubscribeInstance 为实例包月 30 天。若已包月则从当前到期日顺延 30 天
func (d *DB) SubscribeInstance(instanceID int64, userID int64) (expiresAt time.Time, err error) {
	now := time.Now()
	expires := now.AddDate(0, 0, 30) // 30 天后
	var curExpires sql.NullString
	_ = d.QueryRow("SELECT expires_at FROM instance_subscriptions WHERE instance_id = ?", instanceID).Scan(&curExpires)
	if curExpires.Valid && curExpires.String != "" {
		var t time.Time
		if t, err = time.Parse("2006-01-02 15:04:05", curExpires.String); err == nil && t.After(now) {
			expires = t.AddDate(0, 0, 30) // 从当前到期日顺延
		}
	}
	expStr := expires.Format("2006-01-02 15:04:05")
	_, err = d.Exec(
		"INSERT INTO instance_subscriptions (instance_id, expires_at) VALUES (?, ?) ON DUPLICATE KEY UPDATE expires_at = ?",
		instanceID, expStr, expStr,
	)
	if err != nil {
		return time.Time{}, err
	}
	return expires, nil
}

// GetInstanceExpiresAt 返回实例包月到期时间，空表示未包月或已过期
func (d *DB) GetInstanceExpiresAt(instanceID int64) (string, error) {
	var expiresAt string
	err := d.QueryRow(
		"SELECT expires_at FROM instance_subscriptions WHERE instance_id = ? AND expires_at > NOW() LIMIT 1",
		instanceID,
	).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return expiresAt, nil
}

// GetSubscribedExpiresByInstanceIDs 批量查询多个实例的包月到期时间，返回 instance_id -> expires_at 映射
func (d *DB) GetSubscribedExpiresByInstanceIDs(instanceIDs []int64) (map[int64]string, error) {
	if len(instanceIDs) == 0 {
		return nil, nil
	}
	placeholders := ""
	args := make([]any, 0, len(instanceIDs))
	for i, id := range instanceIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, id)
	}
	rows, err := d.Query(
		"SELECT instance_id, expires_at FROM instance_subscriptions WHERE instance_id IN ("+placeholders+") AND expires_at > NOW()",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]string)
	for rows.Next() {
		var id int64
		var exp string
		if err := rows.Scan(&id, &exp); err != nil {
			return nil, err
		}
		out[id] = exp
	}
	return out, nil
}
