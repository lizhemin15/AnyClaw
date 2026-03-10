package db

import (
	"fmt"
	"time"
)

func (d *DB) InsertUsage(instanceID, userID, model, provider string, promptTokens, completionTokens int) error {
	_, err := d.Exec(
		"INSERT INTO usage_log (instance_id, user_id, model, provider, prompt_tokens, completion_tokens) VALUES (?, ?, ?, ?, ?, ?)",
		instanceID, userID, model, provider, promptTokens, completionTokens,
	)
	return err
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
	s := &UsageStats{}
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
