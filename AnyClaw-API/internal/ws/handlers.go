package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/request"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleContainerConnect: container connects with ?instance_id=&token=
func (h *Handler) HandleContainerConnect(w http.ResponseWriter, r *http.Request) {
	instanceIDStr := r.URL.Query().Get("instance_id")
	token := r.URL.Query().Get("token")
	if instanceIDStr == "" || token == "" {
		http.Error(w, "instance_id and token required", http.StatusBadRequest)
		return
	}
	instanceID, err := strconv.ParseInt(instanceIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid instance_id", http.StatusBadRequest)
		return
	}
	inst, err := h.db.GetInstanceByToken(token)
	if err != nil || inst == nil || inst.ID != instanceID {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() {
		h.hub.Unregister(instanceID)
		conn.Close()
	}()
	done := h.hub.Register(instanceID, conn)
	<-done
}

// HandleUserWS: user connects with Bearer JWT to /instances/:id/ws
func (h *Handler) HandleUserWS(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	instanceIDStr := chi.URLParam(r, "id")
	if instanceIDStr == "" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	instanceID, err := strconv.ParseInt(instanceIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid instance id", http.StatusBadRequest)
		return
	}
	inst, err := h.db.GetInstanceByID(instanceID)
	if err != nil || inst == nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if inst.UserID != claims.UserID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	containerConn := h.hub.Get(instanceID)
	if containerConn == nil {
		http.Error(w, "container not connected", http.StatusServiceUnavailable)
		return
	}
	userConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() {
		h.hub.DetachUser(instanceID)
		userConn.Close()
	}()
	h.hub.AttachUser(instanceID, userConn)
	// user->container: read from user, write to container (container->user is handled by Hub's single reader)
	h.bridgeTo(containerConn, userConn, instanceID, false)
}

func (h *Handler) bridgeTo(dst, src *websocket.Conn, instanceID int64, closeDstOnDone bool) {
	if closeDstOnDone {
		defer dst.Close()
	}
	for {
		mt, data, err := src.ReadMessage()
		if err != nil {
			log.Printf("[ws] bridge read error: %v", err)
			return
		}
		if mt == websocket.TextMessage {
			var msg struct {
				Type    string `json:"type"`
				Payload struct {
					Content string `json:"content"`
				} `json:"payload"`
			}
			if json.Unmarshal(data, &msg) == nil && msg.Type == "message.send" && strings.TrimSpace(msg.Payload.Content) != "" {
				_, _ = h.db.InsertMessage(instanceID, "user", msg.Payload.Content)
			}
		}
		if err := dst.WriteMessage(mt, data); err != nil {
			log.Printf("[ws] bridge write error: %v", err)
			return
		}
	}
}
