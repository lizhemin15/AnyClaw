package db

import (
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/go-sql-driver/mysql"
)

type DB struct {
	*sql.DB
}

func Open(dsn string) (*DB, error) {
	if dsn == "" {
		dsn = "anyclaw:anyclaw@tcp(localhost:3306)/anyclaw?parseTime=true&charset=utf8mb4"
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	d := &DB{db}
	if err := d.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role VARCHAR(64) NOT NULL DEFAULT 'user',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS instances (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			user_id BIGINT NOT NULL,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(64) NOT NULL DEFAULT 'creating',
			container_id VARCHAR(255),
			host_id VARCHAR(255),
			token VARCHAR(255) NOT NULL UNIQUE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS hosts (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			addr VARCHAR(255) NOT NULL,
			ssh_port INT NOT NULL DEFAULT 22,
			ssh_user VARCHAR(255) NOT NULL,
			ssh_key TEXT,
			docker_image VARCHAR(255),
			enabled TINYINT NOT NULL DEFAULT 1,
			status VARCHAR(64) NOT NULL DEFAULT 'unknown',
			last_check_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, q := range queries {
		if _, err := d.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	// Indexes (ignore duplicate key)
	for _, q := range []string{
		"CREATE INDEX idx_users_email ON users(email)",
		"CREATE INDEX idx_instances_user_id ON instances(user_id)",
		"CREATE INDEX idx_instances_token ON instances(token)",
	} {
		if _, err := d.Exec(q); err != nil && !isDuplicateKey(err) {
			log.Printf("[db] create index: %v", err)
		}
	}

	// Add host_id if missing (upgrade from older schema)
	if _, err := d.Exec("ALTER TABLE instances ADD COLUMN host_id VARCHAR(255)"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter instances: %v", err)
	}
	return nil
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1061 // Duplicate key name
	}
	return false
}

func isDuplicateColumn(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1060 // Duplicate column name
	}
	return false
}
