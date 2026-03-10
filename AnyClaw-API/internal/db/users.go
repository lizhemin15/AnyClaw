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

func (d *DB) CreateUser(email, passwordHash, role string, emailVerified bool) (*User, error) {
	if role == "" {
		role = "user"
	}
	verified := 0
	if emailVerified {
		verified = 1
	}
	res, err := d.Exec(
		"INSERT INTO users (email, password_hash, role, email_verified) VALUES (?, ?, ?, ?)",
		email, passwordHash, role, verified,
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
