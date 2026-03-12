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
			log.Printf("[ws] instance %d: failed to parse container msg", instanceID)
			return
		}
		log.Printf("[ws] instance %d: recv type=%s role=%s contentLen=%d", instanceID, msg.Type, msg.Payload.Role, len(msg.Payload.Content))
		content := strings.TrimSpace(msg.Payload.Content)
		if content == "" {
			return
		}
		role := msg.Payload.Role
		if role == "" {
			role = "assistant"
		}
		if msg.Type == "message.create" && !strings.HasPrefix(content, "Thinking") {
			_, err := h.db.InsertMessage(instanceID, role, content)
			log.Printf("[ws] instance %d: message.create stored role=%s len=%d err=%v", instanceID, role, len(content), err)
		}
		if msg.Type == "message.update" {
			// message.update is for streaming; update the last assistant message, do not insert
			n, _ := h.db.UpdateLastAssistantMessage(instanceID, content)
			if n == 0 {
				// No existing assistant message (e.g. agent sent update without create), insert
				_, err := h.db.InsertMessage(instanceID, role, content)
				log.Printf("[ws] instance %d: message.update->insert role=%s len=%d err=%v", instanceID, role, len(content), err)
			}
		}
	})
	return h
}
