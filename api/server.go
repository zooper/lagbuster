package api

import (
	"context"
	"encoding/json"
	"fmt"
	"lagbuster/database"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

// Config represents the application configuration (subset needed for API)
type Config struct {
	Notifications NotificationConfig `yaml:"notifications"`
}

type NotificationConfig struct {
	Enabled          bool           `yaml:"enabled"`
	RateLimitMinutes int            `yaml:"rate_limit_minutes"`
	Email            EmailConfig    `yaml:"email"`
	Slack            SlackConfig    `yaml:"slack"`
	Telegram         TelegramConfig `yaml:"telegram"`
}

type EmailConfig struct {
	Enabled    bool     `yaml:"enabled"`
	SMTPHost   string   `yaml:"smtp_host"`
	SMTPPort   int      `yaml:"smtp_port"`
	Username   string   `yaml:"username"`
	Password   string   `yaml:"password"`
	From       string   `yaml:"from"`
	To         []string `yaml:"to"`
	EventTypes []string `yaml:"event_types"`
}

type SlackConfig struct {
	Enabled    bool     `yaml:"enabled"`
	WebhookURL string   `yaml:"webhook_url"`
	EventTypes []string `yaml:"event_types"`
}

type TelegramConfig struct {
	Enabled    bool     `yaml:"enabled"`
	BotToken   string   `yaml:"bot_token"`
	ChatID     string   `yaml:"chat_id"`
	EventTypes []string `yaml:"event_types"`
}

// AppState represents the current application state (same as lagbuster.go)
type AppState struct {
	CurrentPrimary string
	LastSwitchTime time.Time
	StartTime      time.Time
	Peers          map[string]*PeerState
	Config         *Config
	Notifier       interface{} // notifications.Notifier (avoid circular import)
	ConfigPath     string      // Path to config file for saving
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
	s.router.HandleFunc("/api/settings/notifications", s.handleGetNotificationSettings).Methods("GET")
	s.router.HandleFunc("/api/settings/notifications", s.handleUpdateNotificationSettings).Methods("PUT", "POST")
	s.router.HandleFunc("/api/settings/notifications/test", s.handleTestNotification).Methods("POST")

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

func readJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// saveConfig saves the current config to disk
func (s *Server) saveConfig() error {
	if s.state.ConfigPath == "" {
		s.logger.Warn("No config path available - settings will not persist")
		return fmt.Errorf("config path not set")
	}

	// Read the full config file as a map to preserve all sections
	data, err := os.ReadFile(s.state.ConfigPath)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	var fullConfig map[string]interface{}
	if err := yaml.Unmarshal(data, &fullConfig); err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}

	// Update just the notifications section
	s.state.mu.RLock()
	fullConfig["notifications"] = s.state.Config.Notifications
	s.state.mu.RUnlock()

	// Write back to file
	updatedData, err := yaml.Marshal(fullConfig)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Write to temp file first, then rename for atomic update
	tempPath := s.state.ConfigPath + ".tmp"
	if err := os.WriteFile(tempPath, updatedData, 0644); err != nil {
		return fmt.Errorf("writing temp config: %w", err)
	}

	if err := os.Rename(tempPath, s.state.ConfigPath); err != nil {
		return fmt.Errorf("renaming temp config: %w", err)
	}

	s.logger.Info("Notification settings saved to %s", s.state.ConfigPath)
	return nil
}

// rebuildNotificationChannels rebuilds notification channels with new settings
func (s *Server) rebuildNotificationChannels() {
	// This will trigger notification channel rebuild in the main app
	// For now, changes take effect in memory
	s.logger.Info("Rebuilding notification channels with new settings")
}

// UpdateState updates the server's state without recreating the server
// This preserves Config, Notifier, and ConfigPath while updating dynamic fields
func (s *Server) UpdateState(currentPrimary string, lastSwitchTime time.Time, startTime time.Time, peers map[string]*PeerState) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()

	s.state.CurrentPrimary = currentPrimary
	s.state.LastSwitchTime = lastSwitchTime
	s.state.StartTime = startTime
	s.state.Peers = peers
}
