package notifications

import (
	"fmt"
	"net/smtp"
	"strings"
)

// EmailConfig holds email notification configuration
type EmailConfig struct {
	Enabled   bool        `yaml:"enabled"`
	SMTPHost  string      `yaml:"smtp_host"`
	SMTPPort  int         `yaml:"smtp_port"`
	Username  string      `yaml:"username"`
	Password  string      `yaml:"password"`
	From      string      `yaml:"from"`
	To        []string    `yaml:"to"`
	Events    []EventType `yaml:"events"`
}

// EmailChannel implements email notifications
type EmailChannel struct {
	config EmailConfig
}

// NewEmailChannel creates a new email notification channel
func NewEmailChannel(config EmailConfig) *EmailChannel {
	return &EmailChannel{config: config}
}

// Name returns the channel name
func (e *EmailChannel) Name() string {
	return "email"
}

// IsEnabled returns whether the channel is enabled
func (e *EmailChannel) IsEnabled() bool {
	return e.config.Enabled
}

// ShouldNotify returns whether this channel should notify for the given event type
func (e *EmailChannel) ShouldNotify(eventType EventType) bool {
	for _, et := range e.config.Events {
		if et == eventType {
			return true
		}
	}
	return false
}

// Send sends an email notification
func (e *EmailChannel) Send(event Event) error {
	subject, body := e.formatMessage(event)

	// Build email message
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"\r\n"+
		"%s\r\n",
		e.config.From,
		strings.Join(e.config.To, ","),
		subject,
		body)

	// Setup authentication
	auth := smtp.PlainAuth("", e.config.Username, e.config.Password, e.config.SMTPHost)

	// Send email
	addr := fmt.Sprintf("%s:%d", e.config.SMTPHost, e.config.SMTPPort)
	return smtp.SendMail(addr, auth, e.config.From, e.config.To, []byte(msg))
}

func (e *EmailChannel) formatMessage(event Event) (subject, body string) {
	switch event.Type {
	case EventSwitch:
		subject = fmt.Sprintf("[Lagbuster] BGP Primary Switch: %s → %s", event.OldPrimary, event.NewPrimary)
		body = fmt.Sprintf(`BGP Primary Path Switch

Time: %s
Old Primary: %s
New Primary: %s
Reason: %s

The BGP primary path has been switched. Please monitor the situation.
`, event.Timestamp.Format("2006-01-02 15:04:05"), event.OldPrimary, event.NewPrimary, event.Reason)

	case EventFailback:
		subject = fmt.Sprintf("[Lagbuster] Failback to Preferred Primary: %s", event.NewPrimary)
		body = fmt.Sprintf(`BGP Failback to Preferred Primary

Time: %s
Preferred Primary: %s
Reason: %s

The system has automatically failed back to the preferred primary peer.
`, event.Timestamp.Format("2006-01-02 15:04:05"), event.NewPrimary, event.Reason)

	case EventUnhealthy:
		subject = fmt.Sprintf("[Lagbuster] Peer Unhealthy: %s", event.PeerName)
		body = fmt.Sprintf(`BGP Peer Became Unhealthy

Time: %s
Peer: %s
Latency: %.2fms (baseline: %.2fms)
Reason: %s

Please investigate the peer health issue.
`, event.Timestamp.Format("2006-01-02 15:04:05"), event.PeerName, event.Latency, event.Baseline, event.Reason)

	case EventRecovery:
		subject = fmt.Sprintf("[Lagbuster] Peer Recovered: %s", event.PeerName)
		body = fmt.Sprintf(`BGP Peer Recovered

Time: %s
Peer: %s
Latency: %.2fms (baseline: %.2fms)

The peer has returned to healthy status.
`, event.Timestamp.Format("2006-01-02 15:04:05"), event.PeerName, event.Latency, event.Baseline)

	case EventStartup:
		subject = "[Lagbuster] Service Started"
		body = fmt.Sprintf(`Lagbuster Service Started

Time: %s

The BGP path optimization service has started.
`, event.Timestamp.Format("2006-01-02 15:04:05"))

	case EventShutdown:
		subject = "[Lagbuster] Service Stopped"
		body = fmt.Sprintf(`Lagbuster Service Stopped

Time: %s

The BGP path optimization service has been stopped.
`, event.Timestamp.Format("2006-01-02 15:04:05"))

	default:
		subject = fmt.Sprintf("[Lagbuster] Event: %s", event.Type)
		body = fmt.Sprintf("Event: %s\nTime: %s\n", event.Type, event.Timestamp.Format("2006-01-02 15:04:05"))
	}

	return subject, body
}
