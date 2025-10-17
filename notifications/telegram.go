package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// TelegramConfig holds Telegram notification configuration
type TelegramConfig struct {
	Enabled  bool
	BotToken string
	ChatID   string
	Events   []EventType
}

// TelegramChannel implements Telegram notifications
type TelegramChannel struct {
	config TelegramConfig
	client *http.Client
}

// NewTelegramChannel creates a new Telegram notification channel
func NewTelegramChannel(config TelegramConfig) *TelegramChannel {
	return &TelegramChannel{
		config: config,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name returns the channel name
func (t *TelegramChannel) Name() string {
	return "telegram"
}

// IsEnabled returns whether the channel is enabled
func (t *TelegramChannel) IsEnabled() bool {
	return t.config.Enabled
}

// ShouldNotify returns whether this channel should notify for the given event type
func (t *TelegramChannel) ShouldNotify(eventType EventType) bool {
	for _, et := range t.config.Events {
		if et == eventType {
			return true
		}
	}
	return false
}

// Send sends a Telegram notification
func (t *TelegramChannel) Send(event Event) error {
	message := t.formatMessage(event)

	payload := map[string]interface{}{
		"chat_id":    t.config.ChatID,
		"text":       message,
		"parse_mode": "HTML",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling telegram payload: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.config.BotToken)
	resp, err := t.client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("posting to telegram: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram returned status %d", resp.StatusCode)
	}

	return nil
}

func (t *TelegramChannel) formatMessage(event Event) string {
	timestamp := event.Timestamp.Format("2006-01-02 15:04:05")

	switch event.Type {
	case EventSwitch:
		return fmt.Sprintf(`🔄 <b>BGP Primary Switch</b>

<b>Time:</b> %s
<b>Old Primary:</b> %s
<b>New Primary:</b> %s
<b>Reason:</b> %s`, timestamp, event.OldPrimary, event.NewPrimary, event.Reason)

	case EventFailback:
		return fmt.Sprintf(`↩️ <b>Failback to Preferred Primary</b>

<b>Time:</b> %s
<b>Primary:</b> %s
<b>Reason:</b> %s`, timestamp, event.NewPrimary, event.Reason)

	case EventUnhealthy:
		return fmt.Sprintf(`⚠️ <b>Peer Unhealthy: %s</b>

<b>Time:</b> %s
<b>Peer:</b> %s
<b>Latency:</b> %.2fms (baseline: %.2fms)
<b>Reason:</b> %s`, event.PeerName, timestamp, event.PeerName, event.Latency, event.Baseline, event.Reason)

	case EventRecovery:
		return fmt.Sprintf(`✅ <b>Peer Recovered: %s</b>

<b>Time:</b> %s
<b>Peer:</b> %s
<b>Latency:</b> %.2fms (baseline: %.2fms)`, event.PeerName, timestamp, event.PeerName, event.Latency, event.Baseline)

	case EventStartup:
		return fmt.Sprintf(`🚀 <b>Lagbuster Started</b>

<b>Time:</b> %s

The BGP path optimization service has started.`, timestamp)

	case EventShutdown:
		return fmt.Sprintf(`🛑 <b>Lagbuster Stopped</b>

<b>Time:</b> %s

The BGP path optimization service has been stopped.`, timestamp)

	default:
		return fmt.Sprintf(`<b>Event: %s</b>

<b>Time:</b> %s`, event.Type, timestamp)
	}
}
