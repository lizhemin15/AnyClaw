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
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	CoinsCost        int    `json:"coins_cost"`
	CreatedAt        string `json:"created_at"`
}

func (d *DB) ListUserUsage(userID int64, limit, offset int) ([]*UsageLogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	uid := fmt.Sprintf("%d", userID)
	rows, err := d.Query(
		`SELECT id, instance_id, model, prompt_tokens, completion_tokens, COALESCE(coins_cost,0), created_at 
		 FROM usage_log WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		uid, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list user usage: %w", err)
	}
	defer rows.Close()
	var list []*UsageLogEntry
	for rows.Next() {
		var e UsageLogEntry
		if err := rows.Scan(&e.ID, &e.InstanceID, &e.Model, &e.PromptTokens, &e.CompletionTokens, &e.CoinsCost, &e.CreatedAt); err != nil {
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
