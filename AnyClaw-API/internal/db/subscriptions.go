package db

import (
	"database/sql"
	"fmt"
	"time"
)

// IsInstanceSubscribed 检查实例在指定月份是否已包月
func (d *DB) IsInstanceSubscribed(instanceID int64, monthYear string) (bool, error) {
	if monthYear == "" {
		monthYear = time.Now().Format("2006-01")
	}
	var n int
	err := d.QueryRow(
		"SELECT 1 FROM instance_subscriptions WHERE instance_id = ? AND month_year = ? LIMIT 1",
		instanceID, monthYear,
	).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check subscription: %w", err)
	}
	return true, nil
}

// SubscribeInstance 为实例包月指定月份，扣除用户金币
func (d *DB) SubscribeInstance(instanceID int64, userID int64, monthYear string) error {
	if monthYear == "" {
		monthYear = time.Now().Format("2006-01")
	}
	_, err := d.Exec(
		"INSERT INTO instance_subscriptions (instance_id, month_year) VALUES (?, ?)",
		instanceID, monthYear,
	)
	return err
}

// GetInstanceSubscribedMonth 返回实例当前已包月的月份，空表示未包月
func (d *DB) GetInstanceSubscribedMonth(instanceID int64) (string, error) {
	monthYear := time.Now().Format("2006-01")
	var m string
	err := d.QueryRow(
		"SELECT month_year FROM instance_subscriptions WHERE instance_id = ? AND month_year = ? LIMIT 1",
		instanceID, monthYear,
	).Scan(&m)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return m, nil
}

// GetSubscribedMonthsByInstanceIDs 批量查询多个实例的当前月包月状态，返回 instance_id -> month_year 映射
func (d *DB) GetSubscribedMonthsByInstanceIDs(instanceIDs []int64) (map[int64]string, error) {
	if len(instanceIDs) == 0 {
		return nil, nil
	}
	monthYear := time.Now().Format("2006-01")
	// 构建 IN 子句
	placeholders := ""
	args := make([]any, 0, len(instanceIDs)+1)
	for i, id := range instanceIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, id)
	}
	args = append(args, monthYear)
	rows, err := d.Query(
		"SELECT instance_id, month_year FROM instance_subscriptions WHERE instance_id IN ("+placeholders+") AND month_year = ?",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64]string)
	for rows.Next() {
		var id int64
		var m string
		if err := rows.Scan(&id, &m); err != nil {
			return nil, err
		}
		out[id] = m
	}
	return out, nil
}
