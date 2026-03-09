package ws

import (
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

type Hub struct {
	mu         sync.RWMutex
	containers map[int64]*websocket.Conn // instance_id -> container conn
}

func NewHub() *Hub {
	return &Hub{containers: make(map[int64]*websocket.Conn)}
}

func (h *Hub) Register(instanceID int64, conn *websocket.Conn) {
	h.mu.Lock()
	if old, ok := h.containers[instanceID]; ok {
		old.Close()
	}
	h.containers[instanceID] = conn
	h.mu.Unlock()
	log.Printf("[ws] container registered for instance %d", instanceID)
}

func (h *Hub) Unregister(instanceID int64) {
	h.mu.Lock()
	if conn, ok := h.containers[instanceID]; ok {
		conn.Close()
		delete(h.containers, instanceID)
	}
	h.mu.Unlock()
	log.Printf("[ws] container unregistered for instance %d", instanceID)
}

func (h *Hub) Get(instanceID int64) *websocket.Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.containers[instanceID]
}
