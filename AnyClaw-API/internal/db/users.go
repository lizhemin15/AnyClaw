package db

import (
	"database/sql"
	"fmt"
)

type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	Energy       int    `json:"energy"`
	CreatedAt    string `json:"created_at"`
}

func (d *DB) CreateUser(email, passwordHash, role string) (*User, error) {
	if role == "" {
		role = "user"
	}
	res, err := d.Exec(
		"INSERT INTO users (email, password_hash, role) VALUES (?, ?, ?)",
		email, passwordHash, role,
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
		"SELECT id, email, password_hash, role, COALESCE(energy, 0), created_at FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Energy, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) GetUserByEmail(email string) (*User, error) {
	var u User
	err := d.QueryRow(
		"SELECT id, email, password_hash, role, COALESCE(energy, 0), created_at FROM users WHERE email = ?",
		email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Energy, &u.CreatedAt)
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
