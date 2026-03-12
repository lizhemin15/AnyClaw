package db

import (
	"database/sql"
	"fmt"
	"time"
)

func (d *DB) SaveVerificationCode(email, code string, expiresAt time.Time) error {
	_, err := d.Exec(
		"INSERT INTO verification_codes (email, code, expires_at) VALUES (?, ?, ?)",
		email, code, expiresAt,
	)
	return err
}

func (d *DB) VerifyAndConsumeCode(email, code string) (bool, error) {
	var n int
	err := d.QueryRow(
		"SELECT 1 FROM verification_codes WHERE email = ? AND code = ? AND expires_at > UTC_TIMESTAMP() LIMIT 1",
		email, code,
	).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("verify code: %w", err)
	}
	_, _ = d.Exec("DELETE FROM verification_codes WHERE email = ? AND code = ?", email, code)
	return true, nil
}
