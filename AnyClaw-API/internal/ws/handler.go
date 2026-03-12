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

// parseContainerMsg 解析容器消息，兼容多种 payload 格式（Pico、网页、飞书等）
func parseContainerMsg(data []byte) (msgType, content, role string, ok bool) {
	var base struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(data, &base) != nil {
		return "", "", "", false
	}
	msgType = base.Type
	if base.Payload == nil {
		return msgType, "", "", false
	}
	// 标准格式：payload.content, payload.role
	var std struct {
		Content   string `json:"content"`
		MessageID string `json:"message_id"`
		Role      string `json:"role"`
	}
	if json.Unmarshal(base.Payload, &std) == nil {
		content = strings.TrimSpace(std.Content)
		role = std.Role
		if role == "" {
			role = "assistant"
		}
		return msgType, content, role, content != ""
	}
	// 兜底：尝试从 map 提取
	var m map[string]any
	if json.Unmarshal(base.Payload, &m) == nil {
		if c, _ := m["content"].(string); c != "" {
			content = strings.TrimSpace(c)
		}
		if r, _ := m["role"].(string); r != "" {
			role = r
		} else {
			role = "assistant"
		}
		return msgType, content, role, content != ""
	}
	return msgType, "", "", false
}

func NewHandler(db *db.DB, hub *Hub) *Handler {
	h := &Handler{db: db, hub: hub}
	hub.SetOnContainerMessage(func(instanceID int64, data []byte) {
		msgType, content, role, ok := parseContainerMsg(data)
		if !ok {
			log.Printf("[ws] instance %d: failed to parse container msg raw=%s", instanceID, string(data))
			return
		}
		// 只存储 assistant 消息，忽略 user
		if role != "assistant" {
			return
		}
		if strings.HasPrefix(content, "Thinking") {
			return
		}
		stored := false
		if msgType == "message.create" {
			_, err := h.db.InsertMessage(instanceID, role, content)
			log.Printf("[ws] instance %d: message.create stored len=%d err=%v", instanceID, len(content), err)
			stored = err == nil
		}
		if msgType == "message.update" {
			n, _ := h.db.UpdateLastAssistantMessage(instanceID, content)
			if n == 0 {
				_, err := h.db.InsertMessage(instanceID, role, content)
				log.Printf("[ws] instance %d: message.update->insert len=%d err=%v", instanceID, len(content), err)
				stored = err == nil
			} else {
				stored = true
			}
		}
		if !stored && content != "" {
			_, err := h.db.InsertMessage(instanceID, role, content)
			log.Printf("[ws] instance %d: fallback store type=%s len=%d err=%v", instanceID, msgType, len(content), err)
		}
	})
	return h
}
