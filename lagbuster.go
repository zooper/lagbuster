// Lagbuster - BGP path optimizer based on per-peer latency health monitoring
package main

import (
	"context"
	"flag"
	"fmt"
	"lagbuster/api"
	"lagbuster/database"
	"lagbuster/exabgp"
	"lagbuster/notifications"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Configuration structures
type Config struct {
	Peers            []PeerConfig              `yaml:"peers"`
	Thresholds       ThresholdConfig           `yaml:"thresholds"`
	Damping          DampingConfig             `yaml:"damping"`
	Startup          StartupConfig             `yaml:"startup"`
	Bird             BirdConfig                `yaml:"bird"`
	ExaBGP           ExaBGPConfig              `yaml:"exabgp"`
	AnnouncedPrefixes []string                 `yaml:"announced_prefixes"`
	Logging          LoggingConfig             `yaml:"logging"`
	Mode             ModeConfig                `yaml:"mode"`
	API              APIConfig                 `yaml:"api"`
	Database         DatabaseConfig            `yaml:"database"`
	Notifications    notifications.MainConfig  `yaml:"notifications"`
}

type PeerConfig struct {
	Name             string  `yaml:"name"`
	Hostname         string  `yaml:"hostname"`
	ExpectedBaseline float64 `yaml:"expected_baseline"`
	BirdVariable     string  `yaml:"bird_variable"`  // For Bird mode: define variable name in lagbuster-priorities.conf
	BirdProtocol     string  `yaml:"bird_protocol"`  // For Bird mode: Bird protocol name (e.g. EDGE_NYC_01)
	NextHop          string  `yaml:"nexthop"`        // For ExaBGP mode - BGP next-hop IPv6 address
}

type ThresholdConfig struct {
	DegradationThreshold float64 `yaml:"degradation_threshold"`
	AbsoluteMaxLatency   float64 `yaml:"absolute_max_latency"`
	TimeoutLatency       float64 `yaml:"timeout_latency"`
}

type DampingConfig struct {
	ConsecutiveUnhealthyCount          int `yaml:"consecutive_unhealthy_count"`
	ConsecutiveHealthyCountForRecovery int `yaml:"consecutive_healthy_count_for_recovery"`
	MeasurementInterval                int `yaml:"measurement_interval"`
	MeasurementWindow                  int `yaml:"measurement_window"`
}

type StartupConfig struct {
	GracePeriod int `yaml:"grace_period"`
}

type BirdConfig struct {
	PrioritiesFile string `yaml:"priorities_file"`
	BirdcPath      string `yaml:"birdc_path"`
	BirdcTimeout   int    `yaml:"birdc_timeout"`
}

type ExaBGPConfig struct {
	Enabled bool `yaml:"enabled"` // Use ExaBGP instead of Bird
}

type LoggingConfig struct {
	Level           string `yaml:"level"`
	LogMeasurements bool   `yaml:"log_measurements"`
	LogDecisions    bool   `yaml:"log_decisions"`
}

type ModeConfig struct {
	DryRun bool `yaml:"dry_run"`
}

type APIConfig struct {
	Enabled       bool   `yaml:"enabled"`
	ListenAddress string `yaml:"listen_address"`
}

type DatabaseConfig struct {
	Path          string `yaml:"path"`
	RetentionDays int    `yaml:"retention_days"`
}

// Runtime state structures
type PeerState struct {
	Config                    PeerConfig
	Measurements              []float64
	CurrentLatency            float64
	ConsecutiveUnhealthyCount int
	ConsecutiveHealthyCount   int
	IsHealthy                 bool
	BGPSessionUp              bool   // Whether BGP session is established in Bird
	BGPSessionState           string // Current BGP session state from Bird
}

type AppState struct {
	Config     Config
	Peers      map[string]*PeerState
	StartTime  time.Time
	db         *database.DB
	notifier   *notifications.Notifier
	apiServer  *api.Server
	exabgp     *exabgp.Client // ExaBGP API client (when ExaBGP mode enabled)
}

// Logger wrapper for structured logging
type Logger struct {
	level string
}

func NewLogger(level string) *Logger {
	return &Logger{level: level}
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level == "debug" {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func (l *Logger) Info(format string, args ...interface{}) {
	if l.level == "debug" || l.level == "info" {
		log.Printf("[INFO] "+format, args...)
	}
}

func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level != "error" {
		log.Printf("[WARN] "+format, args...)
	}
}

func (l *Logger) Error(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

var logger *Logger

// Main function
func main() {
	configFile := flag.String("config", "config.yaml", "Path to configuration file")
	dryRun := flag.Bool("dry-run", false, "Dry run mode - log decisions without applying changes")
	flag.Parse()

	// Load configuration
	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override dry-run if specified on command line
	if *dryRun {
		config.Mode.DryRun = true
	}

	// Initialize logger
	logger = NewLogger(config.Logging.Level)

	logger.Info("Lagbuster starting (version 1.0)")
	if config.Mode.DryRun {
		logger.Info("Running in DRY-RUN mode - no changes will be applied")
	}

	// Initialize database if configured
	var db *database.DB
	if config.Database.Path != "" {
		db, err = database.Open(config.Database.Path)
		if err != nil {
			log.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()
		logger.Info("Database initialized: %s", config.Database.Path)

		// Start cleanup goroutine if retention is configured
		if config.Database.RetentionDays > 0 {
			go func() {
				for {
					time.Sleep(24 * time.Hour)
					if err := db.CleanupOldData(config.Database.RetentionDays); err != nil {
						logger.Error("Database cleanup failed: %v", err)
					} else {
						logger.Debug("Database cleanup completed (retention: %d days)", config.Database.RetentionDays)
					}
				}
			}()
		}
	}

	// Initialize notifications if configured
	var notifier *notifications.Notifier
	if config.Notifications.Enabled {
		channels := notifications.BuildChannels(config.Notifications, logger)
		notifier = notifications.NewNotifier(channels, config.Notifications.RateLimitMinutes, logger)
		logger.Info("Notifications initialized with %d channels", len(channels))

		// Send startup notification
		notifier.Notify(notifications.Event{
			Type:      notifications.EventStartup,
			Timestamp: time.Now(),
		})
	}

	// Initialize application state
	state := initializeState(config)
	state.db = db
	state.notifier = notifier

	// Initialize API server if configured
	var apiServer *api.Server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if config.API.Enabled {
		// Convert notification event types from EventType to string
		emailEvents := make([]string, len(config.Notifications.Email.Events))
		for i, e := range config.Notifications.Email.Events {
			emailEvents[i] = string(e)
		}
		slackEvents := make([]string, len(config.Notifications.Slack.Events))
		for i, e := range config.Notifications.Slack.Events {
			slackEvents[i] = string(e)
		}
		telegramEvents := make([]string, len(config.Notifications.Telegram.Events))
		for i, e := range config.Notifications.Telegram.Events {
			telegramEvents[i] = string(e)
		}

		// Create API state wrapper from AppState
		apiState := &api.AppState{
			StartTime: state.StartTime,
			Peers:     make(map[string]*api.PeerState),
			Config: &api.Config{
				MeasurementInterval: config.Damping.MeasurementInterval,
				Notifications: api.NotificationConfig{
					Enabled:          config.Notifications.Enabled,
					RateLimitMinutes: config.Notifications.RateLimitMinutes,
					Email: api.EmailConfig{
						Enabled:    config.Notifications.Email.Enabled,
						SMTPHost:   config.Notifications.Email.SMTPHost,
						SMTPPort:   config.Notifications.Email.SMTPPort,
						Username:   config.Notifications.Email.Username,
						Password:   config.Notifications.Email.Password,
						From:       config.Notifications.Email.From,
						To:         config.Notifications.Email.To,
						EventTypes: emailEvents,
					},
					Slack: api.SlackConfig{
						Enabled:    config.Notifications.Slack.Enabled,
						WebhookURL: config.Notifications.Slack.WebhookURL,
						EventTypes: slackEvents,
					},
					Telegram: api.TelegramConfig{
						Enabled:    config.Notifications.Telegram.Enabled,
						BotToken:   config.Notifications.Telegram.BotToken,
						ChatID:     config.Notifications.Telegram.ChatID,
						EventTypes: telegramEvents,
					},
				},
			},
			Notifier:   notifier,
			ConfigPath: *configFile,
		}

		// Convert peer states
		for name, peer := range state.Peers {
			apiState.Peers[name] = &api.PeerState{
				Name:                      peer.Config.Name,
				Hostname:                  peer.Config.Hostname,
				Baseline:                  peer.Config.ExpectedBaseline,
				CurrentLatency:            peer.CurrentLatency,
				IsHealthy:                 peer.IsHealthy,
				ConsecutiveHealthyCount:   peer.ConsecutiveHealthyCount,
				ConsecutiveUnhealthyCount: peer.ConsecutiveUnhealthyCount,
				BGPSessionUp:              peer.BGPSessionUp,
				BGPSessionState:           peer.BGPSessionState,
			}
		}

		apiServer = api.NewServer(apiState, db, logger)
		state.apiServer = apiServer

		go func() {
			logger.Info("Starting API server on %s", config.API.ListenAddress)
			if err := apiServer.Start(ctx, config.API.ListenAddress); err != nil {
				logger.Error("API server error: %v", err)
			}
		}()
	}

	// Startup grace period
	logger.Info("Startup grace period: %d seconds", config.Startup.GracePeriod)
	time.Sleep(time.Duration(config.Startup.GracePeriod) * time.Second)

	// Main monitoring loop
	ticker := time.NewTicker(time.Duration(config.Damping.MeasurementInterval) * time.Second)
	defer ticker.Stop()

	// Run first measurement immediately
	runMonitoringCycle(state)

	// Apply initial Bird configuration based on first measurement
	logger.Info("Applying initial Bird configuration (asymmetric routing mode)")
	if !config.Mode.DryRun {
		err := applyBirdConfiguration(state)
		if err != nil {
			logger.Error("Failed to apply initial Bird configuration: %v", err)
		} else {
			logger.Info("Initial Bird configuration applied successfully")
		}
	} else {
		logger.Info("DRY-RUN: Would apply initial Bird configuration")
	}

	for range ticker.C {
		runMonitoringCycle(state)
	}
}

// Load configuration from YAML file
func loadConfig(filename string) (Config, error) {
	var config Config

	data, err := os.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("reading config file: %w", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("parsing config file: %w", err)
	}

	// Validate configuration
	if len(config.Peers) == 0 {
		return config, fmt.Errorf("no peers defined in configuration")
	}

	return config, nil
}

// Initialize application state
func initializeState(config Config) *AppState {
	state := &AppState{
		Config:    config,
		Peers:     make(map[string]*PeerState),
		StartTime: time.Now(),
	}

	// Initialize peer states (all start as healthy by default, will be evaluated on first cycle)
	for _, peerConfig := range config.Peers {
		state.Peers[peerConfig.Name] = &PeerState{
			Config:       peerConfig,
			Measurements: make([]float64, 0, config.Damping.MeasurementWindow),
			IsHealthy:    true, // Assume healthy until first measurement
		}
	}

	logger.Info("Initialized with %d peers in asymmetric routing mode (ECMP)", len(state.Peers))

	// Initialize ExaBGP client if enabled
	if config.ExaBGP.Enabled {
		state.exabgp = exabgp.NewClient()
		logger.Info("ExaBGP API client initialized (pipe: %s)", exabgp.PipePath)
	}

	return state
}

// Run one monitoring cycle
func runMonitoringCycle(state *AppState) {
	// Measure latency and BGP session status for all peers
	for _, peer := range state.Peers {
		latency := pingHost(peer.Config.Hostname)
		peer.CurrentLatency = latency

		// Check BGP session status
		// In ExaBGP mode, assume sessions are up (ExaBGP manages them directly)
		// In Bird mode, check session status via birdc
		if state.Config.ExaBGP.Enabled {
			peer.BGPSessionUp = true
			peer.BGPSessionState = "Established"
		} else {
			bgpUp, bgpState := checkBGPSession(peer.Config, state.Config.Bird)
			peer.BGPSessionUp = bgpUp
			peer.BGPSessionState = bgpState
		}

		// Add to measurement window
		peer.Measurements = append(peer.Measurements, latency)
		if len(peer.Measurements) > state.Config.Damping.MeasurementWindow {
			peer.Measurements = peer.Measurements[1:]
		}

		if state.Config.Logging.LogMeasurements {
			logger.Debug("Peer %s: latency=%.2fms, baseline=%.2fms, BGP=%s",
				peer.Config.Name, latency, peer.Config.ExpectedBaseline, peer.BGPSessionState)
		}

		// Record measurement to database
		if state.db != nil {
			if err := state.db.RecordMeasurement(peer.Config.Name, latency, peer.IsHealthy, false); err != nil {
				logger.Error("Failed to record measurement for %s: %v", peer.Config.Name, err)
			}
		}
	}

	// Evaluate health of all peers (with damping)
	evaluatePeerHealth(state)

	// Apply routing configuration based on mode
	if state.Config.ExaBGP.Enabled {
		// ExaBGP mode: API-driven route announcements
		if err := applyExaBGPConfiguration(state); err != nil {
			logger.Error("Failed to apply ExaBGP configuration: %v", err)
		}
	} else {
		// Bird mode: Config file approach
		if err := applyBirdConfiguration(state); err != nil {
			logger.Error("Failed to apply Bird configuration: %v", err)
		}
	}

	// Update API server state
	updateAPIServerState(state)
}

// Ping a host and return latency in milliseconds
// Supports both IPv4 and IPv6 addresses
// Uses context-based timeout to prevent hanging on unreachable hosts
func pingHost(host string) float64 {
	// Create context with 5-second timeout (safety margin above ping's 3s timeout)
	// This ensures the command will be killed even if DNS hangs or ping doesn't timeout properly
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var cmd *exec.Cmd

	// Different ping syntax for different operating systems
	// Let ping auto-detect IPv4 vs IPv6 based on hostname resolution
	if runtime.GOOS == "darwin" {
		// macOS: -t 3 = 3 second timeout
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-t", "3", host)
	} else {
		// Linux: -W timeout in milliseconds
		// No -4 or -6 flag - let ping auto-detect based on DNS resolution
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", "3000", host)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it was a timeout
		if ctx.Err() == context.DeadlineExceeded {
			logger.Warn("Ping to %s timed out after 5 seconds (host may be unreachable or DNS hanging)", host)
		} else {
			logger.Debug("Ping to %s failed: %v", host, err)
		}
		return -1
	}

	re := regexp.MustCompile(`time[=<](\d+\.?\d*)\s*ms`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		logger.Debug("Failed to parse ping output for %s", host)
		return -1
	}

	latency, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		logger.Debug("Failed to convert latency for %s: %v", host, err)
		return -1
	}

	return latency
}

// Check BGP session status for a peer via birdc
func checkBGPSession(peerConfig PeerConfig, config BirdConfig) (bool, string) {
	protocolName := peerConfig.BirdProtocol
	if protocolName == "" {
		logger.Debug("No bird_protocol configured for peer %s, skipping BGP session check", peerConfig.Name)
		return false, "Unknown"
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.BirdcTimeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, config.BirdcPath, "show", "protocols", protocolName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Debug("Failed to check BGP session for %s: %v", peerConfig.Name, err)
		return false, "Unknown"
	}

	outputStr := string(output)

	// Parse birdc output for BGP state
	// Expected format: "PROTOCOL_NAME  BGP   ---   up/start   TIMESTAMP   Established/Active/..."
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		// Skip header and empty lines
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "BIRD") || strings.HasPrefix(line, "Name") {
			continue
		}

		// Look for state information - format varies but "Established" means BGP is up
		if strings.Contains(line, "BGP state:") {
			// Detailed state line like: "  BGP state:          Established"
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				state := strings.TrimSpace(parts[1])
				isUp := state == "Established"
				logger.Debug("BGP session %s state: %s (up=%v)", peerConfig.Name, state, isUp)
				return isUp, state
			}
		} else if strings.Contains(line, "Established") {
			// Summary line contains "Established"
			logger.Debug("BGP session %s: Established", peerConfig.Name)
			return true, "Established"
		} else if strings.Contains(line, "Active") || strings.Contains(line, "Connect") ||
		           strings.Contains(line, "Idle") || strings.Contains(line, "start") {
			// Other BGP states indicate session is down
			state := "Down"
			if strings.Contains(line, "Active") {
				state = "Active"
			} else if strings.Contains(line, "Connect") {
				state = "Connect"
			} else if strings.Contains(line, "Idle") {
				state = "Idle"
			}
			logger.Debug("BGP session %s state: %s (up=false)", peerConfig.Name, state)
			return false, state
		}
	}

	logger.Debug("Could not determine BGP state for %s from birdc output", peerConfig.Name)
	return false, "Unknown"
}

// Evaluate health of all peers with damping
func evaluatePeerHealth(state *AppState) {
	for name, peer := range state.Peers {
		latency := peer.CurrentLatency
		baseline := peer.Config.ExpectedBaseline

		// Check current health (without damping)
		currentlyHealthy := isPeerHealthy(latency, baseline, state.Config.Thresholds)

		// Track consecutive unhealthy/healthy counts
		if !currentlyHealthy {
			peer.ConsecutiveUnhealthyCount++
			peer.ConsecutiveHealthyCount = 0
		} else {
			peer.ConsecutiveUnhealthyCount = 0
			peer.ConsecutiveHealthyCount++
		}

		// Apply damping: only change state after consecutive threshold
		wasHealthy := peer.IsHealthy
		if peer.IsHealthy && peer.ConsecutiveUnhealthyCount >= state.Config.Damping.ConsecutiveUnhealthyCount {
			// Degrade: healthy → unhealthy after N consecutive bad measurements
			peer.IsHealthy = false
		} else if !peer.IsHealthy && peer.ConsecutiveHealthyCount >= state.Config.Damping.ConsecutiveHealthyCountForRecovery {
			// Recover: unhealthy → healthy after M consecutive good measurements
			peer.IsHealthy = true
		}

		// Handle health transitions (only log/notify on actual state changes)
		if wasHealthy != peer.IsHealthy {
			// Determine reason for health change
			var reason string
			if !peer.IsHealthy {
				if latency < 0 {
					reason = "unreachable/timeout"
					logger.Info("Peer %s became UNHEALTHY after %d consecutive unhealthy measurements: unreachable/timeout, baseline=%.2fms",
						name, peer.ConsecutiveUnhealthyCount, baseline)
				} else if latency > state.Config.Thresholds.AbsoluteMaxLatency {
					reason = fmt.Sprintf("latency %.2fms exceeds absolute max %.2fms", latency, state.Config.Thresholds.AbsoluteMaxLatency)
					logger.Info("Peer %s became UNHEALTHY after %d consecutive unhealthy measurements: latency=%.2fms exceeds absolute max (%.2fms), baseline=%.2fms",
						name, peer.ConsecutiveUnhealthyCount, latency, state.Config.Thresholds.AbsoluteMaxLatency, baseline)
				} else {
					degradation := latency - baseline
					reason = fmt.Sprintf("degradation %.2fms above baseline", degradation)
					logger.Info("Peer %s became UNHEALTHY after %d consecutive unhealthy measurements: latency=%.2fms, baseline=%.2fms, degradation=%.2fms",
						name, peer.ConsecutiveUnhealthyCount, latency, baseline, degradation)
				}
			} else {
				reason = "latency returned to acceptable levels"
				logger.Info("Peer %s became HEALTHY after %d consecutive healthy measurements: latency=%.2fms, baseline=%.2fms",
					name, peer.ConsecutiveHealthyCount, latency, baseline)
			}

			// Record health change event to database
			if state.db != nil {
				if _, err := state.db.RecordEvent("health_change", &name, nil, nil, &wasHealthy, &peer.IsHealthy, reason, nil); err != nil {
					logger.Error("Failed to record health change event for %s: %v", name, err)
				}
			}

			// Send notifications for significant health changes
			if state.notifier != nil {
				if !peer.IsHealthy {
					// Became unhealthy
					state.notifier.Notify(notifications.Event{
						Type:      notifications.EventUnhealthy,
						PeerName:  name,
						Latency:   latency,
						Baseline:  baseline,
						Reason:    reason,
						Timestamp: time.Now(),
					})
				} else {
					// Recovered to healthy
					state.notifier.Notify(notifications.Event{
						Type:      notifications.EventRecovery,
						PeerName:  name,
						Latency:   latency,
						Baseline:  baseline,
						Timestamp: time.Now(),
					})
				}
			}
		}
	}
}

// Check if a peer is healthy based on current latency vs baseline
func isPeerHealthy(latency float64, baseline float64, thresholds ThresholdConfig) bool {
	// Timeout or failed ping
	if latency < 0 {
		return false
	}

	// Exceeds absolute maximum
	if latency > thresholds.AbsoluteMaxLatency {
		return false
	}

	// Degraded beyond threshold from baseline
	degradation := latency - baseline
	if degradation > thresholds.DegradationThreshold {
		return false
	}

	return true
}


// Apply Bird configuration changes
func applyBirdConfiguration(state *AppState) error {
	// Assign priorities: 1 for primary, 2 for second-best, 3 for third
	priorities := assignPriorities(state)

	// Generate configuration file content
	content := generateBirdConfig(state, priorities)

	// Write to temporary file first
	tempFile := state.Config.Bird.PrioritiesFile + ".tmp"
	err := os.WriteFile(tempFile, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}

	// Atomic rename
	err = os.Rename(tempFile, state.Config.Bird.PrioritiesFile)
	if err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}

	logger.Debug("Wrote Bird config to %s", state.Config.Bird.PrioritiesFile)

	// Reload Bird configuration
	cmd := exec.Command(state.Config.Bird.BirdcPath, "configure")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("birdc configure failed: %w, output: %s", err, string(output))
	}

	// Check if reconfiguration was successful
	if !strings.Contains(string(output), "Reconfigured") {
		return fmt.Errorf("birdc configure did not confirm success: %s", string(output))
	}

	logger.Debug("birdc output: %s", strings.TrimSpace(string(output)))

	return nil
}

// Apply ExaBGP configuration changes via API
func applyExaBGPConfiguration(state *AppState) error {
	// Assign priorities: 1 for healthy, 99 for unhealthy
	priorities := assignPriorities(state)

	// Log what we're about to do
	logger.Debug("Applying ExaBGP configuration for %d peers", len(state.Config.Peers))

	// For each peer, announce routes with appropriate priority
	for _, peerConfig := range state.Config.Peers {
		peer := state.Peers[peerConfig.Name]
		priority := priorities[peerConfig.Name]

		// Log the announcement
		healthStatus := "HEALTHY"
		if !peer.IsHealthy {
			healthStatus = "UNHEALTHY"
		}
		bgpStatus := "BGP-UP"
		if !peer.BGPSessionUp {
			bgpStatus = "BGP-DOWN"
		}

		logger.Debug("ExaBGP: Peer %s: priority=%d, latency=%.2fms, %s, %s",
			peerConfig.Name, priority, peer.CurrentLatency, healthStatus, bgpStatus)

		// Dry-run mode: log but don't actually send
		if state.Config.Mode.DryRun {
			logger.Info("[DRY-RUN] Would announce %d prefixes for peer %s via %s with priority %d",
				len(state.Config.AnnouncedPrefixes), peerConfig.Name, peerConfig.NextHop, priority)
			continue
		}

		// Send announcement to ExaBGP
		err := state.exabgp.AnnounceRoutes(
			peerConfig.Name,
			peerConfig.NextHop,
			state.Config.AnnouncedPrefixes,
			priority,
		)
		if err != nil {
			return fmt.Errorf("announcing routes for peer %s: %w", peerConfig.Name, err)
		}

		logger.Debug("Successfully announced %d prefixes for peer %s with priority %d",
			len(state.Config.AnnouncedPrefixes), peerConfig.Name, priority)
	}

	logger.Info("ExaBGP configuration applied: %d healthy peers, %d unhealthy peers",
		countHealthyPeers(state), len(state.Config.Peers)-countHealthyPeers(state))

	return nil
}

// countHealthyPeers returns the number of healthy peers with BGP sessions up
func countHealthyPeers(state *AppState) int {
	count := 0
	for _, peer := range state.Peers {
		if peer.IsHealthy && peer.BGPSessionUp {
			count++
		}
	}
	return count
}

// Assign priority values (1=best, 2=second, 3=third) based on current primary
func assignPriorities(state *AppState) map[string]int {
	priorities := make(map[string]int)

	// Asymmetric routing (ECMP): All healthy peers with established BGP get priority 1
	// Unhealthy or BGP-down peers get priority 99 (effectively disabled)
	for name, peer := range state.Peers {
		if peer.IsHealthy && peer.BGPSessionUp {
			// Healthy peer with established BGP session - use for routing
			priorities[name] = 1
		} else {
			// Unhealthy or BGP session down - disable
			priorities[name] = 99
		}
	}

	return priorities
}

// Generate Bird configuration file content
func generateBirdConfig(state *AppState, priorities map[string]int) string {
	var sb strings.Builder

	// Count healthy and unhealthy peers
	healthyPeers := make([]string, 0)
	unhealthyPeers := make([]string, 0)
	for name, peer := range state.Peers {
		if peer.IsHealthy && peer.BGPSessionUp {
			healthyPeers = append(healthyPeers, name)
		} else {
			unhealthyPeers = append(unhealthyPeers, name)
		}
	}

	sb.WriteString("# Lagbuster dynamic priority overrides - Asymmetric Routing (ECMP)\n")
	sb.WriteString(fmt.Sprintf("# Generated at: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString("#\n")
	sb.WriteString("# Mode: All healthy peers get priority 1 (ECMP)\n")
	sb.WriteString(fmt.Sprintf("# Healthy peers (%d): %v\n", len(healthyPeers), healthyPeers))
	sb.WriteString(fmt.Sprintf("# Unhealthy/BGP-down peers (%d): %v\n", len(unhealthyPeers), unhealthyPeers))
	sb.WriteString("#\n")
	sb.WriteString("# Priority values: 1=active (ECMP), 99=disabled\n")
	sb.WriteString("#\n\n")

	// Write peer status as comments
	for _, peerConfig := range state.Config.Peers {
		peer := state.Peers[peerConfig.Name]
		priority := priorities[peerConfig.Name]

		healthStatus := "HEALTHY"
		if !peer.IsHealthy {
			healthStatus = "UNHEALTHY"
		}

		sb.WriteString(fmt.Sprintf("# %s: priority=%d, latency=%.2fms, baseline=%.2fms, %s\n",
			peerConfig.Name, priority, peer.CurrentLatency, peer.Config.ExpectedBaseline, healthStatus))
	}

	sb.WriteString("\n")

	// Write priority definitions
	for _, peerConfig := range state.Config.Peers {
		priority := priorities[peerConfig.Name]
		sb.WriteString(fmt.Sprintf("define %s = %d;\n", peerConfig.BirdVariable, priority))
	}

	return sb.String()
}
// updateAPIServerState synchronizes AppState to API server state
func updateAPIServerState(state *AppState) {
	if state.apiServer == nil {
		return
	}

	// Convert peers to API format
	apiPeers := make(map[string]*api.PeerState)
	for name, peer := range state.Peers {
		apiPeers[name] = &api.PeerState{
			Name:                      peer.Config.Name,
			Hostname:                  peer.Config.Hostname,
			Baseline:                  peer.Config.ExpectedBaseline,
			CurrentLatency:            peer.CurrentLatency,
			IsHealthy:                 peer.IsHealthy,
			ConsecutiveHealthyCount:   peer.ConsecutiveHealthyCount,
			ConsecutiveUnhealthyCount: peer.ConsecutiveUnhealthyCount,
			BGPSessionUp:              peer.BGPSessionUp,
			BGPSessionState:           peer.BGPSessionState,
		}
	}

	// Update API server state without recreating the entire server
	// This preserves Config, Notifier, and ConfigPath
	state.apiServer.UpdateState(state.StartTime, apiPeers)
}
