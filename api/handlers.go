package api

import (
	"net/http"
	"time"
)

// StatusResponse represents the current system status
type StatusResponse struct {
	CurrentPrimary string                `json:"current_primary"`
	Uptime         int64                 `json:"uptime_seconds"`
	LastSwitch     *time.Time            `json:"last_switch,omitempty"`
	Peers          map[string]PeerStatus `json:"peers"`
}

// PeerStatus represents a peer's current status
type PeerStatus struct {
	Name                      string  `json:"name"`
	Hostname                  string  `json:"hostname"`
	Latency                   float64 `json:"latency"`
	Baseline                  float64 `json:"baseline"`
	Degradation               float64 `json:"degradation"`
	IsHealthy                 bool    `json:"is_healthy"`
	IsPrimary                 bool    `json:"is_primary"`
	ConsecutiveHealthyCount   int     `json:"consecutive_healthy_count"`
	ConsecutiveUnhealthyCount int     `json:"consecutive_unhealthy_count"`
}

// handleStatus returns the current system status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := s.getCurrentStatus()
	writeJSON(w, status)
}

func (s *Server) getCurrentStatus() StatusResponse {
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()

	peers := make(map[string]PeerStatus)
	for name, peer := range s.state.Peers {
		peers[name] = PeerStatus{
			Name:                      peer.Name,
			Hostname:                  peer.Hostname,
			Latency:                   peer.CurrentLatency,
			Baseline:                  peer.Baseline,
			Degradation:               peer.CurrentLatency - peer.Baseline,
			IsHealthy:                 peer.IsHealthy,
			IsPrimary:                 peer.IsPrimary,
			ConsecutiveHealthyCount:   peer.ConsecutiveHealthyCount,
			ConsecutiveUnhealthyCount: peer.ConsecutiveUnhealthyCount,
		}
	}

	resp := StatusResponse{
		CurrentPrimary: s.state.CurrentPrimary,
		Uptime:         int64(time.Since(s.state.StartTime).Seconds()),
		Peers:          peers,
	}

	if !s.state.LastSwitchTime.IsZero() {
		resp.LastSwitch = &s.state.LastSwitchTime
	}

	return resp
}

// handlePeers returns all peer statuses
func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()

	peers := make([]PeerStatus, 0, len(s.state.Peers))
	for _, peer := range s.state.Peers {
		peers = append(peers, PeerStatus{
			Name:                      peer.Name,
			Hostname:                  peer.Hostname,
			Latency:                   peer.CurrentLatency,
			Baseline:                  peer.Baseline,
			Degradation:               peer.CurrentLatency - peer.Baseline,
			IsHealthy:                 peer.IsHealthy,
			IsPrimary:                 peer.IsPrimary,
			ConsecutiveHealthyCount:   peer.ConsecutiveHealthyCount,
			ConsecutiveUnhealthyCount: peer.ConsecutiveUnhealthyCount,
		})
	}

	writeJSON(w, peers)
}

// handleMetrics returns historical metrics for a peer
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	peerName := r.URL.Query().Get("peer")
	rangeStr := r.URL.Query().Get("range")

	if peerName == "" {
		writeError(w, "peer parameter required", http.StatusBadRequest)
		return
	}

	// Parse range (1h, 24h, 7d, 30d)
	var since time.Time
	switch rangeStr {
	case "1h":
		since = time.Now().Add(-1 * time.Hour)
	case "24h":
		since = time.Now().Add(-24 * time.Hour)
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	default:
		since = time.Now().Add(-1 * time.Hour) // Default to 1 hour
	}

	if s.db == nil {
		writeError(w, "database not configured", http.StatusServiceUnavailable)
		return
	}

	measurements, err := s.db.GetMeasurements(peerName, since)
	if err != nil {
		s.logger.Error("Failed to get measurements: %v", err)
		writeError(w, "failed to fetch measurements", http.StatusInternalServerError)
		return
	}

	// Convert to API response format
	type MetricPoint struct {
		Timestamp time.Time `json:"timestamp"`
		Latency   float64   `json:"latency"`
		IsHealthy bool      `json:"is_healthy"`
	}

	points := make([]MetricPoint, len(measurements))
	for i, m := range measurements {
		points[i] = MetricPoint{
			Timestamp: m.Timestamp,
			Latency:   m.Latency,
			IsHealthy: m.IsHealthy,
		}
	}

	writeJSON(w, map[string]interface{}{
		"peer":   peerName,
		"range":  rangeStr,
		"points": points,
	})
}

// handleEvents returns system events
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	rangeStr := r.URL.Query().Get("range")
	eventTypeStr := r.URL.Query().Get("type")

	// Parse range
	var since time.Time
	switch rangeStr {
	case "1h":
		since = time.Now().Add(-1 * time.Hour)
	case "24h":
		since = time.Now().Add(-24 * time.Hour)
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	default:
		since = time.Now().Add(-24 * time.Hour) // Default to 24 hours
	}

	if s.db == nil {
		writeError(w, "database not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse event types filter
	var eventTypes []string
	if eventTypeStr != "" {
		eventTypes = []string{eventTypeStr}
	}

	events, err := s.db.GetEvents(since, eventTypes)
	if err != nil {
		s.logger.Error("Failed to get events: %v", err)
		writeError(w, "failed to fetch events", http.StatusInternalServerError)
		return
	}

	// Convert to API response format
	type EventResponse struct {
		ID         int64      `json:"id"`
		Timestamp  time.Time  `json:"timestamp"`
		EventType  string     `json:"event_type"`
		PeerName   *string    `json:"peer_name,omitempty"`
		OldPrimary *string    `json:"old_primary,omitempty"`
		NewPrimary *string    `json:"new_primary,omitempty"`
		OldHealth  *bool      `json:"old_health,omitempty"`
		NewHealth  *bool      `json:"new_health,omitempty"`
		Reason     string     `json:"reason"`
	}

	eventResponses := make([]EventResponse, len(events))
	for i, e := range events {
		eventResponses[i] = EventResponse{
			ID:         e.ID,
			Timestamp:  e.Timestamp,
			EventType:  e.EventType,
			PeerName:   e.PeerName,
			OldPrimary: e.OldPrimary,
			NewPrimary: e.NewPrimary,
			OldHealth:  e.OldHealth,
			NewHealth:  e.NewHealth,
			Reason:     e.Reason,
		}
	}

	writeJSON(w, map[string]interface{}{
		"range":  rangeStr,
		"events": eventResponses,
	})
}
