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
	// Add ssh_password for password-based auth
	if _, err := d.Exec("ALTER TABLE hosts ADD COLUMN ssh_password TEXT"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter hosts: %v", err)
	}
	if _, err := d.Exec("ALTER TABLE hosts ADD COLUMN instance_capacity INT NOT NULL DEFAULT 0"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter hosts instance_capacity: %v", err)
	}
	// Energy system
	if _, err := d.Exec("ALTER TABLE users ADD COLUMN energy INT NOT NULL DEFAULT 100"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter users: %v", err)
	}
	if _, err := d.Exec("ALTER TABLE users ADD COLUMN inviter_id BIGINT"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter users inviter_id: %v", err)
	}
	if _, err := d.Exec("ALTER TABLE instances ADD COLUMN energy INT NOT NULL DEFAULT 100"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter instances: %v", err)
	}
	if _, err := d.Exec("ALTER TABLE instances ADD COLUMN daily_consume INT NOT NULL DEFAULT 10"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter instances: %v", err)
	}
	if _, err := d.Exec("ALTER TABLE instances ADD COLUMN zero_energy_since DATETIME"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter instances: %v", err)
	}
	if _, err := d.Exec("ALTER TABLE instances ADD COLUMN last_read_at DATETIME"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter instances last_read_at: %v", err)
	}
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS invitations (
		code VARCHAR(32) PRIMARY KEY,
		inviter_id BIGINT NOT NULL,
		invitee_id BIGINT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		log.Printf("[db] create invitations: %v", err)
	}
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS messages (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		instance_id BIGINT NOT NULL,
		role VARCHAR(32) NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_messages_instance (instance_id),
		INDEX idx_messages_instance_id (instance_id, id)
	)`); err != nil {
		log.Printf("[db] create messages: %v", err)
	}
	// Email verification
	if _, err := d.Exec("ALTER TABLE users ADD COLUMN email_verified TINYINT NOT NULL DEFAULT 1"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter users email_verified: %v", err)
	}
	if _, err := d.Exec("ALTER TABLE users ADD COLUMN last_login_at DATETIME"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter users last_login_at: %v", err)
	}
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS verification_codes (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		code VARCHAR(16) NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_vc_email (email),
		INDEX idx_vc_expires (expires_at)
	)`); err != nil {
		log.Printf("[db] create verification_codes: %v", err)
	}
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS orders (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		user_id BIGINT NOT NULL,
		plan_id VARCHAR(64) NOT NULL,
		energy INT NOT NULL,
		price_cny INT NOT NULL,
		channel VARCHAR(32) NOT NULL,
		status VARCHAR(32) NOT NULL DEFAULT 'pending',
		out_trade_no VARCHAR(64) NOT NULL UNIQUE,
		external_id VARCHAR(128),
		paid_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_orders_user (user_id),
		INDEX idx_orders_status (status),
		INDEX idx_orders_out_trade_no (out_trade_no)
	)`); err != nil {
		log.Printf("[db] create orders: %v", err)
	}
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS usage_log (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		instance_id VARCHAR(64) NOT NULL,
		user_id VARCHAR(64),
		model VARCHAR(128) NOT NULL,
		provider VARCHAR(128),
		prompt_tokens INT NOT NULL DEFAULT 0,
		completion_tokens INT NOT NULL DEFAULT 0,
		coins_cost INT NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_usage_instance (instance_id),
		INDEX idx_usage_user (user_id),
		INDEX idx_usage_created (created_at)
	)`); err != nil {
		log.Printf("[db] create usage_log: %v", err)
	}
	if _, err := d.Exec("ALTER TABLE usage_log ADD COLUMN coins_cost INT NOT NULL DEFAULT 0"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter usage_log coins_cost: %v", err)
	}
	if _, err := d.Exec("ALTER TABLE usage_log ADD COLUMN provider VARCHAR(128)"); err != nil && !isDuplicateColumn(err) {
		log.Printf("[db] alter usage_log provider: %v", err)
	}
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS usage_corrections (
		provider VARCHAR(128) NOT NULL,
		correction_date DATE NOT NULL,
		tokens_delta BIGINT NOT NULL DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (provider, correction_date)
	)`); err != nil {
		log.Printf("[db] create usage_corrections: %v", err)
	}
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS activation_codes (
		code VARCHAR(32) PRIMARY KEY,
		energy INT NOT NULL,
		used_by BIGINT,
		used_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		created_by BIGINT,
		memo VARCHAR(255),
		INDEX idx_ac_used (used_by),
		INDEX idx_ac_created (created_at)
	)`); err != nil {
		log.Printf("[db] create activation_codes: %v", err)
	}
	// 管理配置存 DB，解决 Sealos/K8s 等 /data 不持久化导致配置丢失
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS system_config (
		k VARCHAR(64) PRIMARY KEY,
		v LONGTEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	)`); err != nil {
		log.Printf("[db] create system_config: %v", err)
	}
	// 包月订阅：订阅日起 30 天内对话不消耗金币
	var hasOldSchema int
	_ = d.QueryRow("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'instance_subscriptions' AND column_name = 'month_year'").Scan(&hasOldSchema)
	if hasOldSchema > 0 {
		if _, err := d.Exec("DROP TABLE IF EXISTS instance_subscriptions"); err != nil {
			log.Printf("[db] drop old instance_subscriptions: %v", err)
		}
	}
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS instance_subscriptions (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		instance_id BIGINT NOT NULL UNIQUE,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_expires (expires_at)
	)`); err != nil {
		log.Printf("[db] create instance_subscriptions: %v", err)
	}
	return nil
}

// CheckAndMigrate 重新执行迁移逻辑，补齐缺失的表和列，用于迭代更新后结构不一致时修复。
// 幂等：已存在的表/列不会重复创建。
func (d *DB) CheckAndMigrate() error {
	return d.migrate()
}

// Reset 清空所有业务表并重新迁移，用于解决数据冲突。重置后需前往 /setup 创建管理员。
func (d *DB) Reset() error {
	tables := []string{
		"instance_subscriptions", "usage_corrections", "usage_log", "activation_codes", "orders", "verification_codes", "messages", "invitations",
		"instances", "hosts", "users", "system_config",
	}
	if _, err := d.Exec("SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		return fmt.Errorf("disable fk: %w", err)
	}
	defer d.Exec("SET FOREIGN_KEY_CHECKS = 1")
	for _, t := range tables {
		if _, err := d.Exec("DROP TABLE IF EXISTS " + t); err != nil {
			return fmt.Errorf("drop %s: %w", t, err)
		}
	}
	return d.migrate()
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
