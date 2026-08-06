package api

import (
	"sync"

	"cluster-monitor-backend/internal/alerts"

	"github.com/gorilla/websocket"
)

// Hub fans out alert Events to any connected WebSocket clients (e.g. a
// future web dashboard). The iOS app doesn't need this — it gets push
// notifications directly from APNs — but it's here so a browser tab can
// show live state without polling.
type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]bool
	incoming chan alerts.Event
}

func NewHub() *Hub {
	return &Hub{
		clients:  make(map[*websocket.Conn]bool),
		incoming: make(chan alerts.Event, 64),
	}
}

func (h *Hub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

func (h *Hub) Broadcast(evt alerts.Event) {
	h.incoming <- evt
}

func (h *Hub) Run() {
	for evt := range h.incoming {
		h.mu.Lock()
		for conn := range h.clients {
			if err := conn.WriteJSON(evt); err != nil {
				conn.Close()
				delete(h.clients, conn)
			}
		}
		h.mu.Unlock()
	}
}
