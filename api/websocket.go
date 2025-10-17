package api

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// handleWebSocket handles WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("WebSocket upgrade failed: %v", err)
		return
	}

	s.logger.Info("New WebSocket client connected from %s", r.RemoteAddr)

	// Register client
	s.mu.Lock()
	s.clients[conn] = true
	s.mu.Unlock()

	// Send initial status immediately
	status := s.getCurrentStatus()
	conn.WriteJSON(map[string]interface{}{
		"type": "status_update",
		"data": status,
	})

	// Setup ping/pong for connection health
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Cleanup on disconnect
	defer func() {
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
		s.logger.Info("WebSocket client disconnected from %s", r.RemoteAddr)
	}()

	// Keep connection alive and handle incoming messages
	go func() {
		for range ticker.C {
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	// Read messages (mostly to detect disconnection)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.logger.Warn("WebSocket error: %v", err)
			}
			break
		}
	}
}

// BroadcastEvent sends an event notification to all WebSocket clients
func (s *Server) BroadcastEvent(eventType, peerName, reason string) {
	s.Broadcast(map[string]interface{}{
		"type": "event",
		"data": map[string]interface{}{
			"event_type": eventType,
			"peer_name":  peerName,
			"reason":     reason,
			"timestamp":  time.Now(),
		},
	})
}
