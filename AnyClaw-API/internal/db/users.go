package db

import (
	"database/sql"
	"fmt"
)

type User struct {
	ID            int64  `json:"id"`
	Email         string `json:"email"`
	PasswordHash  string `json:"-"`
	Role          string `json:"role"`
	Energy        int    `json:"energy"`
	EmailVerified bool   `json:"email_verified"`
	CreatedAt     string `json:"created_at"`
}

func (d *DB) CreateUser(email, passwordHash, role string, emailVerified bool, initialEnergy int) (*User, error) {
	if role == "" {
		role = "user"
	}
	verified := 0
	if emailVerified {
		verified = 1
	}
	res, err := d.Exec(
		"INSERT INTO users (email, password_hash, role, email_verified, energy) VALUES (?, ?, ?, ?, ?)",
		email, passwordHash, role, verified, initialEnergy,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	id, _ := res.LastInsertId()
	return d.GetUserByID(id)
}

func (d *DB) GetUserByID(id int64) (*User, error) {
	var u User
	err := d.QueryRow(
		"SELECT id, email, password_hash, role, COALESCE(energy, 0), COALESCE(email_verified, 1), created_at FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Energy, &u.EmailVerified, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) ListUsers() ([]*User, error) {
	rows, err := d.Query(
		"SELECT id, email, role, COALESCE(energy, 0), created_at FROM users ORDER BY id ASC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.Energy, &u.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &u)
	}
	return list, nil
}

func (d *DB) GetUserByEmail(email string) (*User, error) {
	var u User
	err := d.QueryRow(
		"SELECT id, email, password_hash, role, COALESCE(energy, 0), COALESCE(email_verified, 1), created_at FROM users WHERE email = ?",
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Energy, &u.EmailVerified, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) AddUserEnergy(userID int64, amount int) error {
	if amount <= 0 {
		return nil
	}
	_, err := d.Exec("UPDATE users SET energy = energy + ? WHERE id = ?", amount, userID)
	return err
}

func (d *DB) DeductUserEnergy(userID int64, amount int) (ok bool, err error) {
	if amount <= 0 {
		return true, nil
	}
	res, err := d.Exec("UPDATE users SET energy = energy - ? WHERE id = ? AND energy >= ?", amount, userID, amount)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetUserInviterID 返回用户的邀请人 ID，无则返回 0, false
func (d *DB) GetUserInviterID(userID int64) (int64, bool) {
	var inviterID int64
	err := d.QueryRow("SELECT inviter_id FROM users WHERE id = ? AND inviter_id IS NOT NULL", userID).Scan(&inviterID)
	if err == sql.ErrNoRows || inviterID == 0 {
		return 0, false
	}
	if err != nil {
		return 0, false
	}
	return inviterID, true
}

// SetUserInviter 设置用户的邀请人（仅当尚未绑定且 inviterID != userID 时）
func (d *DB) SetUserInviter(userID, inviterID int64) error {
	if userID == inviterID {
		return nil
	}
	_, err := d.Exec("UPDATE users SET inviter_id = ? WHERE id = ? AND inviter_id IS NULL", inviterID, userID)
	return err
}

// GrantDailyLoginBonus 若今日首次登录则发放金币并更新 last_login_at，返回是否发放
func (d *DB) GrantDailyLoginBonus(userID int64, bonus int) (granted bool, err error) {
	if bonus <= 0 {
		return false, nil
	}
	// 原子：仅当 last_login_at 为 NULL 或日期早于今天时更新并加金币
	res, err := d.Exec(`
		UPDATE users SET
			energy = energy + ?,
			last_login_at = NOW()
		WHERE id = ? AND (last_login_at IS NULL OR DATE(last_login_at) < CURDATE())
	`, bonus, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
