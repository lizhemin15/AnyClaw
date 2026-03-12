package db

import (
	"crypto/rand"
	"database/sql"
	"fmt"
)

const activationCodeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func generateActivationCode() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = activationCodeChars[int(b[i])%len(activationCodeChars)]
	}
	return string(b)
}

type ActivationCode struct {
	Code      string  `json:"code"`
	Energy    int     `json:"energy"`
	UsedBy    *int64  `json:"used_by,omitempty"`
	UsedAt    *string `json:"used_at,omitempty"`
	CreatedAt string  `json:"created_at"`
	CreatedBy *int64  `json:"created_by,omitempty"`
	Memo      string  `json:"memo,omitempty"`
}

func (d *DB) CreateActivationCodes(energy int, count int, createdBy int64, memo string) ([]string, error) {
	codes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		code := generateActivationCode()
		var cb interface{}
		if createdBy > 0 {
			cb = createdBy
		} else {
			cb = nil
		}
		_, err := d.Exec(
			"INSERT INTO activation_codes (code, energy, created_by, memo) VALUES (?, ?, ?, ?)",
			code, energy, cb, memo,
		)
		if err != nil {
			return nil, fmt.Errorf("create activation code: %w", err)
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func (d *DB) GetActivationCode(code string) (*ActivationCode, error) {
	var ac ActivationCode
	var usedBy sql.NullInt64
	var usedAt sql.NullString
	var createdBy sql.NullInt64
	err := d.QueryRow(
		"SELECT code, energy, used_by, used_at, created_at, created_by, COALESCE(memo,'') FROM activation_codes WHERE code = ?",
		code,
	).Scan(&ac.Code, &ac.Energy, &usedBy, &usedAt, &ac.CreatedAt, &createdBy, &ac.Memo)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if usedBy.Valid {
		ac.UsedBy = &usedBy.Int64
	}
	if usedAt.Valid {
		ac.UsedAt = &usedAt.String
	}
	if createdBy.Valid {
		ac.CreatedBy = &createdBy.Int64
	}
	return &ac, nil
}

func (d *DB) RedeemActivationCode(code string, userID int64) (energy int, err error) {
	ac, err := d.GetActivationCode(code)
	if err != nil {
		return 0, err
	}
	if ac == nil {
		return 0, fmt.Errorf("激活码不存在")
	}
	if ac.UsedBy != nil {
		return 0, fmt.Errorf("激活码已使用")
	}
	res, err := d.Exec(
		"UPDATE activation_codes SET used_by = ?, used_at = NOW() WHERE code = ? AND used_by IS NULL",
		userID, code,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return 0, fmt.Errorf("激活码已使用或不存在")
	}
	if err := d.AddUserEnergy(userID, ac.Energy); err != nil {
		return 0, err
	}
	return ac.Energy, nil
}

func (d *DB) ListActivationCodes(status string, limit, offset int) ([]*ActivationCode, error) {
	var query string
	args := []interface{}{}
	switch status {
	case "unused":
		query = "SELECT code, energy, used_by, used_at, created_at, created_by, COALESCE(memo,'') FROM activation_codes WHERE used_by IS NULL ORDER BY created_at DESC LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	case "used":
		query = "SELECT code, energy, used_by, used_at, created_at, created_by, COALESCE(memo,'') FROM activation_codes WHERE used_by IS NOT NULL ORDER BY used_at DESC LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	default:
		query = "SELECT code, energy, used_by, used_at, created_at, created_by, COALESCE(memo,'') FROM activation_codes ORDER BY created_at DESC LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*ActivationCode
	for rows.Next() {
		var ac ActivationCode
		var usedBy sql.NullInt64
		var usedAt sql.NullString
		var createdBy sql.NullInt64
		if err := rows.Scan(&ac.Code, &ac.Energy, &usedBy, &usedAt, &ac.CreatedAt, &createdBy, &ac.Memo); err != nil {
			return nil, err
		}
		if usedBy.Valid {
			ac.UsedBy = &usedBy.Int64
		}
		if usedAt.Valid {
			ac.UsedAt = &usedAt.String
		}
		if createdBy.Valid {
			ac.CreatedBy = &createdBy.Int64
		}
		list = append(list, &ac)
	}
	return list, nil
}
