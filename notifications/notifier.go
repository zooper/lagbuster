package notifications

import (
	"fmt"
	"sync"
	"time"
)

// EventType represents the type of event that triggered a notification
type EventType string

const (
	EventSwitch     EventType = "switch"
	EventUnhealthy  EventType = "unhealthy"
	EventRecovery   EventType = "recovery"
	EventFailback   EventType = "failback"
	EventStartup    EventType = "startup"
	EventShutdown   EventType = "shutdown"
)

// Event represents a notification event
type Event struct {
	Type       EventType
	PeerName   string
	OldPrimary string
	NewPrimary string
	Reason     string
	Latency    float64
	Baseline   float64
	Timestamp  time.Time
}

// Channel represents a notification channel (email, slack, etc.)
type Channel interface {
	Name() string
	Send(event Event) error
	IsEnabled() bool
	ShouldNotify(eventType EventType) bool
}

// Notifier manages multiple notification channels with rate limiting
type Notifier struct {
	channels      []Channel
	rateLimitMins int
	lastSent      map[string]time.Time // key: "channelName:eventType"
	mu            sync.RWMutex
	logger        Logger
}

// Logger interface for logging (matches lagbuster's logger)
type Logger interface {
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

// NewNotifier creates a new notifier with the given channels
func NewNotifier(channels []Channel, rateLimitMins int, logger Logger) *Notifier {
	return &Notifier{
		channels:      channels,
		rateLimitMins: rateLimitMins,
		lastSent:      make(map[string]time.Time),
		logger:        logger,
	}
}

// Notify sends an event to all enabled channels that should receive it
func (n *Notifier) Notify(event Event) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, channel := range n.channels {
		if !channel.IsEnabled() {
			continue
		}

		if !channel.ShouldNotify(event.Type) {
			continue
		}

		// Check rate limiting
		key := fmt.Sprintf("%s:%s", channel.Name(), event.Type)
		if lastSent, exists := n.lastSent[key]; exists {
			if time.Since(lastSent) < time.Duration(n.rateLimitMins)*time.Minute {
				n.logger.Debug("Rate limited: %s for %s", channel.Name(), event.Type)
				continue
			}
		}

		// Send notification
		if err := channel.Send(event); err != nil {
			n.logger.Error("Failed to send %s notification via %s: %v", event.Type, channel.Name(), err)
		} else {
			n.logger.Info("Sent %s notification via %s", event.Type, channel.Name())
			n.lastSent[key] = time.Now()
		}
	}
}

// AddChannel adds a new notification channel
func (n *Notifier) AddChannel(channel Channel) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.channels = append(n.channels, channel)
}

// SendTest sends a test notification to a specific channel or all channels
func (n *Notifier) SendTest(channelName string) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// Create test event
	testEvent := Event{
		Type:      EventType("test"),
		Reason:    "This is a test notification from Lagbuster",
		Timestamp: time.Now(),
	}

	var sentCount int
	var lastError error

	for _, channel := range n.channels {
		// Skip if channel doesn't match (unless "all" is specified)
		if channelName != "all" && channel.Name() != channelName {
			continue
		}

		if !channel.IsEnabled() {
			n.logger.Warn("Channel %s is not enabled, skipping test", channel.Name())
			continue
		}

		// Send test notification (bypass rate limiting and event type filtering for tests)
		if err := channel.Send(testEvent); err != nil {
			n.logger.Error("Failed to send test notification via %s: %v", channel.Name(), err)
			lastError = err
		} else {
			n.logger.Info("Sent test notification via %s", channel.Name())
			sentCount++
		}
	}

	if sentCount == 0 {
		if lastError != nil {
			return fmt.Errorf("test notification failed: %w", lastError)
		}
		return fmt.Errorf("no enabled channels found for: %s", channelName)
	}

	return nil
}

// MainConfig holds the top-level notifications configuration
type MainConfig struct {
	Enabled          bool           `yaml:"enabled"`
	RateLimitMinutes int            `yaml:"rate_limit_minutes"`
	Email            EmailConfig    `yaml:"email"`
	Slack            SlackConfig    `yaml:"slack"`
	Telegram         TelegramConfig `yaml:"telegram"`
}
