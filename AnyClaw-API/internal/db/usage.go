package db

import (
	"fmt"
	"time"
)

func (d *DB) InsertUsage(instanceID, userID, model, provider string, promptTokens, completionTokens, coinsCost int) error {
	_, err := d.Exec(
		"INSERT INTO usage_log (instance_id, user_id, model, provider, prompt_tokens, completion_tokens, coins_cost) VALUES (?, ?, ?, ?, ?, ?, ?)",
		instanceID, userID, model, provider, promptTokens, completionTokens, coinsCost,
	)
	return err
}

// UsageLogEntry 用户消耗记录单条
type UsageLogEntry struct {
	ID               int64  `json:"id"`
	InstanceID       string `json:"instance_id"`
	InstanceName     string `json:"instance_name,omitempty"` // 宠物名称，用于用户友好展示
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	CoinsCost        int    `json:"coins_cost"`
	CreatedAt        string `json:"created_at"`
}

// UsageLogEntryAdmin 管理员消耗记录，含用户、模型等
type UsageLogEntryAdmin struct {
	UsageLogEntry
	UserEmail string `json:"user_email,omitempty"`
}

func (d *DB) ListUserUsage(userID int64, limit, offset int) ([]*UsageLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	uid := fmt.Sprintf("%d", userID)
	// 优先尝试带 JOIN 的查询（含宠物名称）；失败时回退到简单查询，避免 MySQL 版本/CAST 兼容性问题
	rows, err := d.Query(
		`SELECT u.id, u.instance_id, COALESCE(i.name, ''), u.model, u.prompt_tokens, u.completion_tokens, COALESCE(u.coins_cost,0), u.created_at
		 FROM usage_log u LEFT JOIN instances i ON u.instance_id = CAST(i.id AS CHAR(64)) AND i.user_id = ?
		 WHERE u.user_id = ? ORDER BY u.created_at DESC LIMIT ? OFFSET ?`,
		userID, uid, limit, offset,
	)
	if err != nil {
		// 回退1：子查询获取宠物名称
		rows, err = d.Query(
			`SELECT u.id, u.instance_id,
			 COALESCE((SELECT i.name FROM instances i WHERE CONVERT(i.id, CHAR(64)) = u.instance_id AND i.user_id = ? LIMIT 1), '') as instance_name,
			 u.model, u.prompt_tokens, u.completion_tokens, COALESCE(u.coins_cost,0), u.created_at
			 FROM usage_log u WHERE u.user_id = ? ORDER BY u.created_at DESC LIMIT ? OFFSET ?`,
			userID, uid, limit, offset,
		)
		if err != nil {
			// 回退2：最简查询，确保能返回数据（instance_name 为空）
			rows, err = d.Query(
				`SELECT id, instance_id, '' as instance_name, model, prompt_tokens, completion_tokens, COALESCE(coins_cost,0), created_at
				 FROM usage_log WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
				uid, limit, offset,
			)
			if err != nil {
				return nil, fmt.Errorf("list user usage: %w", err)
			}
		}
	}
	defer rows.Close()
	var list []*UsageLogEntry
	for rows.Next() {
		var e UsageLogEntry
		if err := rows.Scan(&e.ID, &e.InstanceID, &e.InstanceName, &e.Model, &e.PromptTokens, &e.CompletionTokens, &e.CoinsCost, &e.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &e)
	}
	return list, nil
}

func (d *DB) ListAdminUsage(limit, offset int) ([]*UsageLogEntryAdmin, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := d.Query(
		`SELECT u.id, u.instance_id, COALESCE(i.name, ''), u.model, u.prompt_tokens, u.completion_tokens, COALESCE(u.coins_cost,0), u.created_at, COALESCE(us.email, u.user_id)
		 FROM usage_log u
		 LEFT JOIN instances i ON u.instance_id = CAST(i.id AS CHAR(64))
		 LEFT JOIN users us ON u.user_id = CAST(us.id AS CHAR(64))
		 ORDER BY u.created_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		// 回退1：子查询获取宠物名称和用户邮箱
		rows, err = d.Query(
			`SELECT u.id, u.instance_id,
			 COALESCE((SELECT i.name FROM instances i WHERE CONVERT(i.id, CHAR(64)) = u.instance_id LIMIT 1), '') as instance_name,
			 u.model, u.prompt_tokens, u.completion_tokens, COALESCE(u.coins_cost,0), u.created_at,
			 COALESCE((SELECT us.email FROM users us WHERE CONVERT(us.id, CHAR(64)) = u.user_id LIMIT 1), u.user_id) as user_email
			 FROM usage_log u ORDER BY u.created_at DESC LIMIT ? OFFSET ?`,
			limit, offset,
		)
		if err != nil {
			// 回退2：最简查询，确保能返回数据
			rows, err = d.Query(
				`SELECT id, instance_id, '' as instance_name, model, prompt_tokens, completion_tokens, COALESCE(coins_cost,0), created_at, user_id as user_email
				 FROM usage_log ORDER BY created_at DESC LIMIT ? OFFSET ?`,
				limit, offset,
			)
			if err != nil {
				return nil, fmt.Errorf("list admin usage: %w", err)
			}
		}
	}
	defer rows.Close()
	var list []*UsageLogEntryAdmin
	for rows.Next() {
		var e UsageLogEntryAdmin
		if err := rows.Scan(&e.ID, &e.InstanceID, &e.InstanceName, &e.Model, &e.PromptTokens, &e.CompletionTokens, &e.CoinsCost, &e.CreatedAt, &e.UserEmail); err != nil {
			return nil, err
		}
		list = append(list, &e)
	}
	return list, nil
}

type UsageStats struct {
	TotalCalls         int64 `json:"total_calls"`
	TotalPromptTokens  int64 `json:"total_prompt_tokens"`
	TotalCompletionTokens int64 `json:"total_completion_tokens"`
	ByModel            []ModelUsage `json:"by_model"`
	ByUser             []UserUsage  `json:"by_user"`
}

type ModelUsage struct {
	Model              string `json:"model"`
	Calls              int64  `json:"calls"`
	PromptTokens       int64  `json:"prompt_tokens"`
	CompletionTokens   int64  `json:"completion_tokens"`
}

type UserUsage struct {
	UserID             string `json:"user_id"`
	Email              string `json:"email,omitempty"`
	Calls              int64  `json:"calls"`
	PromptTokens       int64  `json:"prompt_tokens"`
	CompletionTokens   int64  `json:"completion_tokens"`
}

func (d *DB) GetUsageStats(since time.Time) (*UsageStats, error) {
	s := &UsageStats{
		ByModel: []ModelUsage{},
		ByUser:  []UserUsage{},
	}
	err := d.QueryRow(
		"SELECT COUNT(*), COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0) FROM usage_log WHERE created_at >= ?",
		since,
	).Scan(&s.TotalCalls, &s.TotalPromptTokens, &s.TotalCompletionTokens)
	if err != nil {
		return nil, fmt.Errorf("usage stats: %w", err)
	}

	rows, err := d.Query(
		"SELECT model, COUNT(*), COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0) FROM usage_log WHERE created_at >= ? GROUP BY model ORDER BY COUNT(*) DESC",
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("usage by model: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m ModelUsage
		if err := rows.Scan(&m.Model, &m.Calls, &m.PromptTokens, &m.CompletionTokens); err != nil {
			return nil, err
		}
		s.ByModel = append(s.ByModel, m)
	}

	rows2, err := d.Query(
		"SELECT user_id, COUNT(*), COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0) FROM usage_log WHERE created_at >= ? AND user_id != '' AND user_id IS NOT NULL GROUP BY user_id ORDER BY COUNT(*) DESC",
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("usage by user: %w", err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var u UserUsage
		if err := rows2.Scan(&u.UserID, &u.Calls, &u.PromptTokens, &u.CompletionTokens); err != nil {
			return nil, err
		}
		s.ByUser = append(s.ByUser, u)
	}

	return s, nil
}
