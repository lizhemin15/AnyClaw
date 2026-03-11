package ws

import (
	"encoding/json"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/db"
)

type Handler struct {
	db  *db.DB
	hub *Hub
}

func NewHandler(db *db.DB, hub *Hub) *Handler {
	h := &Handler{db: db, hub: hub}
		hub.SetOnContainerMessage(func(instanceID int64, data []byte) {
		var msg struct {
			Type    string `json:"type"`
			Payload struct {
				Content   string `json:"content"`
				MessageID string `json:"message_id"`
				Role     string `json:"role"`
			} `json:"payload"`
		}
		if json.Unmarshal(data, &msg) != nil {
			return
		}
		content := strings.TrimSpace(msg.Payload.Content)
		if content == "" {
			return
		}
		role := msg.Payload.Role
		if role == "" {
			role = "assistant"
		}
		if msg.Type == "message.create" && !strings.HasPrefix(content, "Thinking") {
			_, _ = h.db.InsertMessage(instanceID, role, content)
		}
		if msg.Type == "message.update" {
			_, _ = h.db.InsertMessage(instanceID, role, content)
		}
	})
	return h
}
