package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SlackConfig holds Slack notification configuration
type SlackConfig struct {
	Enabled    bool        `yaml:"enabled"`
	WebhookURL string      `yaml:"webhook_url"`
	Events     []EventType `yaml:"events"`
}

// SlackChannel implements Slack notifications
type SlackChannel struct {
	config SlackConfig
	client *http.Client
}

// NewSlackChannel creates a new Slack notification channel
func NewSlackChannel(config SlackConfig) *SlackChannel {
	return &SlackChannel{
		config: config,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name returns the channel name
func (s *SlackChannel) Name() string {
	return "slack"
}

// IsEnabled returns whether the channel is enabled
func (s *SlackChannel) IsEnabled() bool {
	return s.config.Enabled
}

// ShouldNotify returns whether this channel should notify for the given event type
func (s *SlackChannel) ShouldNotify(eventType EventType) bool {
	for _, et := range s.config.Events {
		if et == eventType {
			return true
		}
	}
	return false
}

// Send sends a Slack notification
func (s *SlackChannel) Send(event Event) error {
	payload := s.formatMessage(event)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling slack payload: %w", err)
	}

	resp, err := s.client.Post(s.config.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("posting to slack: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	return nil
}

type slackPayload struct {
	Text        string            `json:"text,omitempty"`
	Attachments []slackAttachment `json:"attachments,omitempty"`
}

type slackAttachment struct {
	Color  string              `json:"color,omitempty"`
	Title  string              `json:"title,omitempty"`
	Text   string              `json:"text,omitempty"`
	Fields []slackAttachmentField `json:"fields,omitempty"`
	Footer string              `json:"footer,omitempty"`
	Ts     int64               `json:"ts,omitempty"`
}

type slackAttachmentField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

func (s *SlackChannel) formatMessage(event Event) slackPayload {
	var color string
	var title string
	var fields []slackAttachmentField

	switch event.Type {
	case EventSwitch:
		color = "warning"
		title = "🔄 BGP Primary Switch"
		fields = []slackAttachmentField{
			{Title: "Old Primary", Value: event.OldPrimary, Short: true},
			{Title: "New Primary", Value: event.NewPrimary, Short: true},
			{Title: "Reason", Value: event.Reason, Short: false},
		}

	case EventFailback:
		color = "good"
		title = "↩️ Failback to Preferred Primary"
		fields = []slackAttachmentField{
			{Title: "Primary", Value: event.NewPrimary, Short: true},
			{Title: "Reason", Value: event.Reason, Short: false},
		}

	case EventUnhealthy:
		color = "danger"
		title = fmt.Sprintf("⚠️ Peer Unhealthy: %s", event.PeerName)
		fields = []slackAttachmentField{
			{Title: "Peer", Value: event.PeerName, Short: true},
			{Title: "Latency", Value: fmt.Sprintf("%.2fms (baseline: %.2fms)", event.Latency, event.Baseline), Short: true},
			{Title: "Reason", Value: event.Reason, Short: false},
		}

	case EventRecovery:
		color = "good"
		title = fmt.Sprintf("✅ Peer Recovered: %s", event.PeerName)
		fields = []slackAttachmentField{
			{Title: "Peer", Value: event.PeerName, Short: true},
			{Title: "Latency", Value: fmt.Sprintf("%.2fms (baseline: %.2fms)", event.Latency, event.Baseline), Short: true},
		}

	case EventStartup:
		color = "good"
		title = "🚀 Lagbuster Started"

	case EventShutdown:
		color = "#808080"
		title = "🛑 Lagbuster Stopped"

	default:
		color = "#808080"
		title = fmt.Sprintf("Event: %s", event.Type)
	}

	return slackPayload{
		Attachments: []slackAttachment{
			{
				Color:  color,
				Title:  title,
				Fields: fields,
				Footer: "Lagbuster BGP Optimizer",
				Ts:     event.Timestamp.Unix(),
			},
		},
	}
}
