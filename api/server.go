package api

import (
	"context"
	"encoding/json"
	"lagbuster/database"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// AppState represents the current application state (same as lagbuster.go)
type AppState struct {
	CurrentPrimary string
	LastSwitchTime time.Time
	StartTime      time.Time
	Peers          map[string]*PeerState
	mu             sync.RWMutex
}

// PeerState represents a peer's current state
type PeerState struct {
	Name                      string
	Hostname                  string
	Baseline                  float64
	CurrentLatency            float64
	IsHealthy                 bool
	IsPrimary                 bool
	ConsecutiveHealthyCount   int
	ConsecutiveUnhealthyCount int
	Measurements              []float64
}

// Server is the HTTP API server
type Server struct {
	state    *AppState
	db       *database.DB
	router   *mux.Router
	upgrader websocket.Upgrader
	clients  map[*websocket.Conn]bool
	mu       sync.RWMutex
	logger   Logger
}

// Logger interface for logging
type Logger interface {
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

// NewServer creates a new API server
func NewServer(state *AppState, db *database.DB, logger Logger) *Server {
	s := &Server{
		state: state,
		db:    db,
		router: mux.NewRouter(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins for development
		},
		clients: make(map[*websocket.Conn]bool),
		logger:  logger,
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// API routes
	s.router.HandleFunc("/api/status", s.handleStatus).Methods("GET")
	s.router.HandleFunc("/api/peers", s.handlePeers).Methods("GET")
	s.router.HandleFunc("/api/metrics", s.handleMetrics).Methods("GET")
	s.router.HandleFunc("/api/events", s.handleEvents).Methods("GET")

	// WebSocket
	s.router.HandleFunc("/ws", s.handleWebSocket)

	// Enable CORS for development
	s.router.Use(corsMiddleware)
}

// Start starts the API server
func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	// Start broadcast goroutine
	go s.broadcastLoop(ctx)

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		s.logger.Info("Shutting down API server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	s.logger.Info("API server listening on %s", addr)
	return srv.ListenAndServe()
}

// Broadcast sends an update to all connected WebSocket clients
func (s *Server) Broadcast(data interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msg, err := json.Marshal(data)
	if err != nil {
		s.logger.Error("Failed to marshal broadcast data: %v", err)
		return
	}

	for client := range s.clients {
		if err := client.WriteMessage(websocket.TextMessage, msg); err != nil {
			s.logger.Warn("Failed to write to websocket client: %v", err)
			client.Close()
			delete(s.clients, client)
		}
	}
}

func (s *Server) broadcastLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Broadcast current status every 10 seconds
			status := s.getCurrentStatus()
			s.Broadcast(map[string]interface{}{
				"type": "status_update",
				"data": status,
			})
		}
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
