package ws

import (
	"github.com/anyclaw/anyclaw-api/internal/db"
)

type Handler struct {
	db  *db.DB
	hub *Hub
}

func NewHandler(db *db.DB, hub *Hub) *Handler {
	return &Handler{db: db, hub: hub}
}
