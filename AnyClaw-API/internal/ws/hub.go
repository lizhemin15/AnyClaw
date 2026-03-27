package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// OnContainerMessage is called when a message is received from the container (before forwarding to user).
type OnContainerMessage func(instanceID int64, data []byte)

type containerEntry struct {
	conn    *websocket.Conn
	writeMu sync.Mutex // 与 bridge 并发写容器连接时串行化
	userMu  sync.RWMutex
	user    *websocket.Conn // nil when no user attached
	// 容器下行转发与 API 下行（协作推送）共用，避免并发 WriteMessage
	userOutboundMu sync.Mutex
	done           chan struct{} // closed when container disconnects
}

type Hub struct {
	mu               sync.RWMutex
	containers       map[int64]*containerEntry
	onContainerMsg   OnContainerMessage
}

func NewHub() *Hub {
	return &Hub{containers: make(map[int64]*containerEntry)}
}

func (h *Hub) SetOnContainerMessage(f OnContainerMessage) {
	h.mu.Lock()
	h.onContainerMsg = f
	h.mu.Unlock()
}

// Register stores the container conn and starts a reader. Returns a channel that
// closes when the container disconnects. Caller should block on it.
func (h *Hub) Register(instanceID int64, conn *websocket.Conn) <-chan struct{} {
	h.mu.Lock()
	if old, ok := h.containers[instanceID]; ok {
		old.conn.Close()
		<-old.done
	}
	entry := &containerEntry{conn: conn, done: make(chan struct{})}
	h.containers[instanceID] = entry
	h.mu.Unlock()
	log.Printf("[ws] container registered for instance %d", instanceID)

	go h.containerReader(instanceID, entry)
	return entry.done
}

func (h *Hub) containerReader(instanceID int64, entry *containerEntry) {
	defer func() {
		close(entry.done)
		h.Unregister(instanceID)
	}()
	for {
		mt, data, err := entry.conn.ReadMessage()
		if err != nil {
			if !websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[ws] container %d disconnected: %v", instanceID, err)
			}
			return
		}
		h.mu.RLock()
		onMsg := h.onContainerMsg
		h.mu.RUnlock()
		if onMsg != nil {
			onMsg(instanceID, data)
		}
		entry.userMu.RLock()
		user := entry.user
		entry.userMu.RUnlock()
		if user != nil {
			entry.userOutboundMu.Lock()
			err := user.WriteMessage(mt, data)
			entry.userOutboundMu.Unlock()
			if err != nil {
				log.Printf("[ws] forward to user failed (user likely disconnected): %v", err)
				entry.userMu.Lock()
				entry.user = nil
				entry.userMu.Unlock()
			}
		}
	}
}

func (h *Hub) Unregister(instanceID int64) {
	h.mu.Lock()
	if entry, ok := h.containers[instanceID]; ok {
		entry.conn.Close()
		delete(h.containers, instanceID)
	}
	h.mu.Unlock()
	log.Printf("[ws] container unregistered for instance %d", instanceID)
}

func (h *Hub) Get(instanceID int64) *websocket.Conn {
	h.mu.RLock()
	entry, ok := h.containers[instanceID]
	h.mu.RUnlock()
	if !ok || entry == nil {
		return nil
	}
	return entry.conn
}

func (h *Hub) AttachUser(instanceID int64, userConn *websocket.Conn) bool {
	h.mu.RLock()
	entry, ok := h.containers[instanceID]
	h.mu.RUnlock()
	if !ok || entry == nil {
		return false
	}
	entry.userMu.Lock()
	entry.user = userConn
	entry.userMu.Unlock()
	return true
}

func (h *Hub) DetachUser(instanceID int64) {
	h.mu.RLock()
	entry, ok := h.containers[instanceID]
	h.mu.RUnlock()
	if !ok || entry == nil {
		return
	}
	entry.userMu.Lock()
	entry.user = nil
	entry.userMu.Unlock()
}

// WriteContainerMessage 向容器 WebSocket 写入二进制帧（与 API 推送共用，避免并发写）
func (h *Hub) WriteContainerMessage(instanceID int64, messageType int, data []byte) error {
	h.mu.RLock()
	entry, ok := h.containers[instanceID]
	h.mu.RUnlock()
	if !ok || entry == nil {
		return fmt.Errorf("no container for instance %d", instanceID)
	}
	entry.writeMu.Lock()
	defer entry.writeMu.Unlock()
	return entry.conn.WriteMessage(messageType, data)
}

// WriteContainerJSON 向容器推送 JSON 文本帧（内部邮件唤醒、拓扑更新等）
func (h *Hub) WriteContainerJSON(instanceID int64, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	h.mu.RLock()
	entry, ok := h.containers[instanceID]
	h.mu.RUnlock()
	if !ok || entry == nil {
		return fmt.Errorf("no container for instance %d", instanceID)
	}
	entry.writeMu.Lock()
	defer entry.writeMu.Unlock()
	return entry.conn.WriteMessage(websocket.TextMessage, data)
}

// WriteAttachedUserJSON 向当前挂接在实例上的浏览器 WS 推送 JSON（与容器同帧结构，用于协作页与对话页联动）
func (h *Hub) WriteAttachedUserJSON(instanceID int64, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.RLock()
	entry, ok := h.containers[instanceID]
	h.mu.RUnlock()
	if !ok || entry == nil {
		return
	}
	entry.userMu.RLock()
	user := entry.user
	entry.userMu.RUnlock()
	if user == nil {
		return
	}
	entry.userOutboundMu.Lock()
	err = user.WriteMessage(websocket.TextMessage, data)
	entry.userOutboundMu.Unlock()
	if err != nil {
		log.Printf("[ws] push JSON to attached user instance %d: %v", instanceID, err)
		entry.userMu.Lock()
		entry.user = nil
		entry.userMu.Unlock()
	}
}
