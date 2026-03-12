package ws

import (
	"encoding/json"
	"log"
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
			log.Printf("[ws] instance %d: failed to parse container msg raw=%s", instanceID, string(data))
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
		// 只存储 assistant 消息，忽略 user
		if role != "assistant" {
			return
		}
		stored := false
		if msg.Type == "message.create" && !strings.HasPrefix(content, "Thinking") {
			_, err := h.db.InsertMessage(instanceID, role, content)
			log.Printf("[ws] instance %d: message.create stored len=%d err=%v", instanceID, len(content), err)
			stored = err == nil
		}
		if msg.Type == "message.update" {
			n, _ := h.db.UpdateLastAssistantMessage(instanceID, content)
			if n == 0 {
				_, err := h.db.InsertMessage(instanceID, role, content)
				log.Printf("[ws] instance %d: message.update->insert len=%d err=%v", instanceID, len(content), err)
				stored = err == nil
			} else {
				stored = true
			}
		}
		if !stored && msg.Type == "message.create" && !strings.HasPrefix(content, "Thinking") {
			log.Printf("[ws] instance %d: WARNING message.create not stored contentLen=%d", instanceID, len(content))
		}
	})
	return h
}
