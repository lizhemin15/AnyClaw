package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Message struct {
	ID         int64     `json:"id"`
	InstanceID int64     `json:"instance_id"`
	Role       string    `json:"role"` // user, assistant
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

func (d *DB) InsertMessage(instanceID int64, role, content string) (int64, error) {
	res, err := d.Exec(
		"INSERT INTO messages (instance_id, role, content) VALUES (?, ?, ?)",
		instanceID, role, content,
	)
	if err != nil {
		return 0, fmt.Errorf("insert message: %w", err)
	}
	return res.LastInsertId()
}

func (d *DB) ListMessages(instanceID int64, limit int, beforeID int64) ([]*Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows *sql.Rows
	var err error
	if beforeID > 0 {
		rows, err = d.Query(
			`SELECT id, instance_id, role, content, created_at FROM messages
			 WHERE instance_id = ? AND id < ?
			 ORDER BY id DESC LIMIT ?`,
			instanceID, beforeID, limit,
		)
	} else {
		rows, err = d.Query(
			`SELECT id, instance_id, role, content, created_at FROM messages
			 WHERE instance_id = ?
			 ORDER BY id DESC LIMIT ?`,
			instanceID, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	var list []*Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.InstanceID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, &m)
	}
	return list, nil
}

func (d *DB) DeleteMessagesByInstance(instanceID int64) error {
	_, err := d.Exec("DELETE FROM messages WHERE instance_id = ?", instanceID)
	return err
}

// IsMediaContent 判断 content 是否为媒体消息（含图片/文件/音视频链接），此类消息不应被后续文本覆盖
func IsMediaContent(content string) bool {
	s := content
	return strings.Contains(s, "![") || strings.Contains(s, "[📎") ||
		strings.Contains(s, "[📹") || strings.Contains(s, "[🔊")
}

// UpdateLastAssistantMessage updates the content of the most recent assistant message.
// 若最后一条是媒体消息（含文件链接），则不覆盖，返回 0 让调用方 Insert 新消息。
func (d *DB) UpdateLastAssistantMessage(instanceID int64, content string) (int64, error) {
	var id int64
	var lastContent string
	err := d.QueryRow(
		"SELECT id, content FROM messages WHERE instance_id = ? AND role = 'assistant' ORDER BY id DESC LIMIT 1",
		instanceID,
	).Scan(&id, &lastContent)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("update last assistant message: %w", err)
	}
	if IsMediaContent(lastContent) {
		return 0, nil
	}
	res, err := d.Exec("UPDATE messages SET content = ? WHERE id = ?", content, id)
	if err != nil {
		return 0, fmt.Errorf("update last assistant message: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
