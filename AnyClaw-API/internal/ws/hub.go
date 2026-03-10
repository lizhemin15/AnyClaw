package ws

import (
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// OnContainerMessage is called when a message is received from the container (before forwarding to user).
type OnContainerMessage func(instanceID int64, data []byte)

type containerEntry struct {
	conn   *websocket.Conn
	userMu sync.RWMutex
	user   *websocket.Conn // nil when no user attached
	done   chan struct{}  // closed when container disconnects
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
			if err := user.WriteMessage(mt, data); err != nil {
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
