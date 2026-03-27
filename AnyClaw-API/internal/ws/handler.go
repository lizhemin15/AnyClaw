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
// jsonOK：顶层 JSON 合法；persistAssistant：message.create/update 且 role 为 assistant、可解析 payload，应走入库逻辑
func parseContainerMsg(data []byte) (msgType, content, role string, jsonOK bool, persistAssistant bool) {
	var base struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return "", "", "", false, false
	}
	msgType = strings.TrimSpace(base.Type)
	if msgType != "message.create" && msgType != "message.update" {
		return msgType, "", "", true, false
	}
	if len(base.Payload) == 0 || string(base.Payload) == "null" {
		return msgType, "", "", true, false
	}
	// 标准格式：payload.content, payload.role
	var std struct {
		Content   string `json:"content"`
		MessageID string `json:"message_id"`
		Role      string `json:"role"`
	}
	if err := json.Unmarshal(base.Payload, &std); err == nil {
		content = strings.TrimSpace(std.Content)
		role = std.Role
		if role == "" {
			role = "assistant"
		}
		return msgType, content, role, true, role == "assistant"
	}
	var m map[string]any
	if err := json.Unmarshal(base.Payload, &m); err == nil {
		if c, _ := m["content"].(string); c != "" {
			content = strings.TrimSpace(c)
		}
		if r, _ := m["role"].(string); r != "" {
			role = r
		} else {
			role = "assistant"
		}
		return msgType, content, role, true, role == "assistant"
	}
	return msgType, "", "", true, false
}

func NewHandler(database *db.DB, hub *Hub) *Handler {
	h := &Handler{db: database, hub: hub}
	hub.SetOnContainerMessage(func(instanceID int64, data []byte) {
		msgType, content, role, jsonOK, persist := parseContainerMsg(data)
		if !jsonOK {
			log.Printf("[ws] instance %d: failed to parse container msg (invalid JSON) raw=%s", instanceID, string(data))
			return
		}
		if !persist {
			return
		}
		// 空内容不存（占位符 Thinking... 有内容会存）
		if content == "" {
			return
		}
		// 流式回复：首条 message.create 常为 "Thinking..."，也存为占位符，后续 message.update 会覆盖
		stored := false
		if msgType == "message.create" {
			if db.IsMediaContent(content) {
				n, _ := h.db.AppendToLastAssistantMessage(instanceID, content)
				if n > 0 {
					stored = true
				}
			}
			if !stored {
				_, err := h.db.InsertMessage(instanceID, role, content)
				log.Printf("[ws] instance %d: message.create stored len=%d err=%v", instanceID, len(content), err)
				stored = err == nil
			}
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
