// Lagbuster - BGP path optimizer based on per-peer latency health monitoring
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Configuration structures
type Config struct {
	Peers      []PeerConfig    `yaml:"peers"`
	Thresholds ThresholdConfig `yaml:"thresholds"`
	Damping    DampingConfig   `yaml:"damping"`
	Startup    StartupConfig   `yaml:"startup"`
	Bird       BirdConfig      `yaml:"bird"`
	Logging    LoggingConfig   `yaml:"logging"`
	Mode       ModeConfig      `yaml:"mode"`
}

type PeerConfig struct {
	Name             string  `yaml:"name"`
	Hostname         string  `yaml:"hostname"`
	ExpectedBaseline float64 `yaml:"expected_baseline"`
	BirdVariable     string  `yaml:"bird_variable"`
}

type ThresholdConfig struct {
	DegradationThreshold float64 `yaml:"degradation_threshold"`
	ComfortThreshold     float64 `yaml:"comfort_threshold"`
	AbsoluteMaxLatency   float64 `yaml:"absolute_max_latency"`
	TimeoutLatency       float64 `yaml:"timeout_latency"`
}

type DampingConfig struct {
	ConsecutiveUnhealthyCount int `yaml:"consecutive_unhealthy_count"`
	MeasurementInterval       int `yaml:"measurement_interval"`
	CooldownPeriod            int `yaml:"cooldown_period"`
	MeasurementWindow         int `yaml:"measurement_window"`
}

type StartupConfig struct {
	GracePeriod    int    `yaml:"grace_period"`
	InitialPrimary string `yaml:"initial_primary"`
}

type BirdConfig struct {
	PrioritiesFile string `yaml:"priorities_file"`
	BirdcPath      string `yaml:"birdc_path"`
	BirdcTimeout   int    `yaml:"birdc_timeout"`
}

type LoggingConfig struct {
	Level           string `yaml:"level"`
	LogMeasurements bool   `yaml:"log_measurements"`
	LogDecisions    bool   `yaml:"log_decisions"`
}

type ModeConfig struct {
	DryRun bool `yaml:"dry_run"`
}

// Runtime state structures
type PeerState struct {
	Config                    PeerConfig
	Measurements              []float64
	CurrentLatency            float64
	ConsecutiveUnhealthyCount int
	IsHealthy                 bool
	IsPrimary                 bool
}

type AppState struct {
	Config         Config
	Peers          map[string]*PeerState
	CurrentPrimary string
	LastSwitchTime time.Time
	StartTime      time.Time
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

	// Initialize application state
	state := initializeState(config)

	// Startup grace period
	logger.Info("Startup grace period: %d seconds", config.Startup.GracePeriod)
	time.Sleep(time.Duration(config.Startup.GracePeriod) * time.Second)

	// Main monitoring loop
	ticker := time.NewTicker(time.Duration(config.Damping.MeasurementInterval) * time.Second)
	defer ticker.Stop()

	// Run first measurement immediately
	runMonitoringCycle(state)

	// Apply initial Bird configuration based on first measurement
	logger.Info("Applying initial Bird configuration with primary: %s", state.CurrentPrimary)
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
		Config:         config,
		Peers:          make(map[string]*PeerState),
		CurrentPrimary: config.Startup.InitialPrimary,
		StartTime:      time.Now(),
	}

	// Initialize peer states
	for _, peerConfig := range config.Peers {
		state.Peers[peerConfig.Name] = &PeerState{
			Config:       peerConfig,
			Measurements: make([]float64, 0, config.Damping.MeasurementWindow),
			IsPrimary:    peerConfig.Name == config.Startup.InitialPrimary,
		}
	}

	logger.Info("Initialized with %d peers, primary: %s", len(state.Peers), state.CurrentPrimary)

	return state
}

// Run one monitoring cycle
func runMonitoringCycle(state *AppState) {
	// Measure latency for all peers
	for _, peer := range state.Peers {
		latency := pingHost(peer.Config.Hostname)
		peer.CurrentLatency = latency

		// Add to measurement window
		peer.Measurements = append(peer.Measurements, latency)
		if len(peer.Measurements) > state.Config.Damping.MeasurementWindow {
			peer.Measurements = peer.Measurements[1:]
		}

		if state.Config.Logging.LogMeasurements {
			logger.Debug("Peer %s: latency=%.2fms, baseline=%.2fms",
				peer.Config.Name, latency, peer.Config.ExpectedBaseline)
		}
	}

	// Evaluate health of all peers
	evaluatePeerHealth(state)

	// Make primary selection decision
	newPrimary := selectPrimary(state)

	// Apply changes if needed
	if newPrimary != state.CurrentPrimary {
		switchPrimary(state, newPrimary)
	}
}

// Ping a host and return latency in milliseconds
// Uses IPv4 only to ensure consistent measurements
func pingHost(host string) float64 {
	var cmd *exec.Cmd

	// Different ping syntax for different operating systems
	// macOS: ping is IPv4 by default (ping6 for IPv6), use -t for timeout
	// Linux: supports -4 flag for IPv4, -W is in milliseconds
	if runtime.GOOS == "darwin" {
		// macOS: ping is IPv4 by default, -t 3 = 3 second timeout, -W is broken on macOS
		cmd = exec.Command("ping", "-c", "1", "-t", "3", host)
	} else {
		// Linux: use -4 flag to force IPv4, -W timeout in milliseconds
		cmd = exec.Command("ping", "-4", "-c", "1", "-W", "3000", host)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Debug("Ping to %s failed: %v", host, err)
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

// Evaluate health of all peers
func evaluatePeerHealth(state *AppState) {
	for name, peer := range state.Peers {
		latency := peer.CurrentLatency
		baseline := peer.Config.ExpectedBaseline

		// Check if peer is healthy
		wasHealthy := peer.IsHealthy
		peer.IsHealthy = isPeerHealthy(latency, baseline, state.Config.Thresholds)

		// Track consecutive unhealthy count
		if !peer.IsHealthy {
			peer.ConsecutiveUnhealthyCount++
		} else {
			peer.ConsecutiveUnhealthyCount = 0
		}

		// Log health transitions
		if wasHealthy && !peer.IsHealthy {
			logger.Info("Peer %s became UNHEALTHY: latency=%.2fms, baseline=%.2fms, degradation=%.2fms",
				name, latency, baseline, latency-baseline)
		} else if !wasHealthy && peer.IsHealthy {
			logger.Info("Peer %s became HEALTHY: latency=%.2fms, baseline=%.2fms",
				name, latency, baseline)
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

// Select the best primary peer
func selectPrimary(state *AppState) string {
	currentPeer := state.Peers[state.CurrentPrimary]
	thresholds := state.Config.Thresholds
	damping := state.Config.Damping

	// Check if we're in cooldown period
	timeSinceSwitch := time.Since(state.LastSwitchTime).Seconds()
	if timeSinceSwitch < float64(damping.CooldownPeriod) && !state.LastSwitchTime.IsZero() {
		logger.Debug("In cooldown period (%.0fs remaining)",
			float64(damping.CooldownPeriod)-timeSinceSwitch)
		return state.CurrentPrimary
	}

	// Check if current primary is healthy and within comfort zone
	if currentPeer.IsHealthy {
		latency := currentPeer.CurrentLatency
		baseline := currentPeer.Config.ExpectedBaseline
		degradation := latency - baseline

		if degradation <= thresholds.ComfortThreshold {
			logger.Debug("Current primary %s is healthy and comfortable (degradation=%.2fms)",
				state.CurrentPrimary, degradation)
			return state.CurrentPrimary
		}
	}

	// Current primary is unhealthy or uncomfortable
	// Check if we have enough consecutive unhealthy measurements before switching
	if currentPeer.ConsecutiveUnhealthyCount < damping.ConsecutiveUnhealthyCount {
		logger.Debug("Current primary %s unhealthy but waiting for damping (%d/%d)",
			state.CurrentPrimary,
			currentPeer.ConsecutiveUnhealthyCount,
			damping.ConsecutiveUnhealthyCount)
		return state.CurrentPrimary
	}

	// Time to switch - find best alternative
	return findBestPeer(state)
}

// Find the best peer to use as primary
func findBestPeer(state *AppState) string {
	type peerScore struct {
		name    string
		latency float64
		healthy bool
	}

	scores := make([]peerScore, 0, len(state.Peers))

	for name, peer := range state.Peers {
		latency := peer.CurrentLatency
		if latency < 0 {
			latency = state.Config.Thresholds.TimeoutLatency
		}

		scores = append(scores, peerScore{
			name:    name,
			latency: latency,
			healthy: peer.IsHealthy,
		})
	}

	// Sort by: healthy first, then by latency
	sort.Slice(scores, func(i, j int) bool {
		if scores[i].healthy != scores[j].healthy {
			return scores[i].healthy
		}
		return scores[i].latency < scores[j].latency
	})

	bestPeer := scores[0].name

	if state.Config.Logging.LogDecisions {
		healthyCount := 0
		for _, s := range scores {
			if s.healthy {
				healthyCount++
			}
		}
		logger.Info("Best peer selection: %s (latency=%.2fms, healthy=%v, %d/%d peers healthy)",
			bestPeer, scores[0].latency, scores[0].healthy, healthyCount, len(scores))
	}

	return bestPeer
}

// Switch to a new primary peer
func switchPrimary(state *AppState, newPrimary string) {
	oldPrimary := state.CurrentPrimary
	oldPeer := state.Peers[oldPrimary]
	newPeer := state.Peers[newPrimary]

	reason := buildSwitchReason(oldPeer, newPeer, state.Config.Thresholds)

	if state.Config.Logging.LogDecisions {
		logger.Info("SWITCHING PRIMARY: %s -> %s | Reason: %s",
			oldPrimary, newPrimary, reason)
		logger.Info("  Old: %s latency=%.2fms baseline=%.2fms healthy=%v",
			oldPrimary, oldPeer.CurrentLatency, oldPeer.Config.ExpectedBaseline, oldPeer.IsHealthy)
		logger.Info("  New: %s latency=%.2fms baseline=%.2fms healthy=%v",
			newPrimary, newPeer.CurrentLatency, newPeer.Config.ExpectedBaseline, newPeer.IsHealthy)
	}

	// Update state
	oldPeer.IsPrimary = false
	newPeer.IsPrimary = true
	state.CurrentPrimary = newPrimary
	state.LastSwitchTime = time.Now()

	// Reset consecutive unhealthy counter for new primary
	newPeer.ConsecutiveUnhealthyCount = 0

	// Apply changes to Bird
	if !state.Config.Mode.DryRun {
		err := applyBirdConfiguration(state)
		if err != nil {
			logger.Error("Failed to apply Bird configuration: %v", err)
		} else {
			logger.Info("Bird configuration updated successfully")
		}
	} else {
		logger.Info("DRY-RUN: Would update Bird configuration")
	}
}

// Build human-readable switch reason
func buildSwitchReason(oldPeer, newPeer *PeerState, thresholds ThresholdConfig) string {
	if !oldPeer.IsHealthy {
		degradation := oldPeer.CurrentLatency - oldPeer.Config.ExpectedBaseline
		if oldPeer.CurrentLatency < 0 {
			return "current primary unreachable"
		}
		if oldPeer.CurrentLatency > thresholds.AbsoluteMaxLatency {
			return fmt.Sprintf("current primary exceeds absolute max (%.2fms > %.2fms)",
				oldPeer.CurrentLatency, thresholds.AbsoluteMaxLatency)
		}
		return fmt.Sprintf("current primary degraded (%.2fms above baseline)", degradation)
	}

	// Old peer is healthy but uncomfortable
	degradation := oldPeer.CurrentLatency - oldPeer.Config.ExpectedBaseline
	return fmt.Sprintf("current primary outside comfort zone (%.2fms above baseline)", degradation)
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

// Assign priority values (1=best, 2=second, 3=third) based on current primary
func assignPriorities(state *AppState) map[string]int {
	type peerLatency struct {
		name    string
		latency float64
	}

	priorities := make(map[string]int)

	// Current primary always gets priority 1
	priorities[state.CurrentPrimary] = 1

	// Collect remaining peers
	remainingPeers := make([]peerLatency, 0, len(state.Peers)-1)
	for name, peer := range state.Peers {
		if name == state.CurrentPrimary {
			continue // Skip current primary
		}
		latency := peer.CurrentLatency
		if latency < 0 {
			latency = state.Config.Thresholds.TimeoutLatency
		}
		remainingPeers = append(remainingPeers, peerLatency{name: name, latency: latency})
	}

	// Sort remaining peers by latency (lowest first)
	sort.Slice(remainingPeers, func(i, j int) bool {
		return remainingPeers[i].latency < remainingPeers[j].latency
	})

	// Assign priorities 2, 3, etc. to remaining peers
	for i, peer := range remainingPeers {
		priorities[peer.name] = i + 2
	}

	return priorities
}

// Generate Bird configuration file content
func generateBirdConfig(state *AppState, priorities map[string]int) string {
	var sb strings.Builder

	sb.WriteString("# Lagbuster dynamic priority overrides\n")
	sb.WriteString(fmt.Sprintf("# Generated at: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("# Current primary: %s\n", state.CurrentPrimary))
	sb.WriteString("#\n")
	sb.WriteString("# Priority values: 1=primary, 2=secondary, 3=tertiary\n")
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
