package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"cluster-monitor-backend/internal/alerts"
	"cluster-monitor-backend/internal/push"

	"github.com/gorilla/websocket"
)

type Server struct {
	store  *alerts.Store
	hub    *Hub
	pusher push.AlertNotifier // nil (true nil interface) when no push backend is configured
}

func NewServer(store *alerts.Store, hub *Hub, pusher push.AlertNotifier) *Server {
	return &Server{store: store, hub: hub, pusher: pusher}
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/api/v1/pods", s.handlePods)
	mux.HandleFunc("/api/v1/alerts", s.handleAlerts)
	mux.HandleFunc("/api/v1/devices", s.handleDevices)
	mux.HandleFunc("/api/v1/ws", s.handleWS)
	return withCORS(mux)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handlePods(w http.ResponseWriter, r *http.Request) {
	snapshot := s.store.Snapshot()
	writeJSON(w, snapshot)
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	writeJSON(w, s.store.Recent(limit))
}

type deviceRequest struct {
	DeviceToken string `json:"deviceToken"`
	Action      string `json:"action"` // "register" | "unregister"
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req deviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if s.pusher == nil {
		http.Error(w, "push not configured on server", http.StatusServiceUnavailable)
		return
	}
	if req.Action == "unregister" {
		s.pusher.UnregisterDevice(req.DeviceToken)
	} else {
		s.pusher.RegisterDevice(req.DeviceToken)
	}
	w.WriteHeader(http.StatusNoContent)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.hub.Register(conn)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
