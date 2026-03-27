package main

// This file shows the key changes needed to integrate ExaBGP into lagbuster.go
// The main changes are:
// 1. Add ExaBGP client to AppState
// 2. Replace applyBirdConfiguration() with applyExaBGPConfiguration()
// 3. Remove Bird config file generation code

import (
	"lagbuster/exabgp"
	"log"
)

// Modified AppState - add ExaBGP client
type AppState struct {
	Config       Config
	Peers        map[string]*PeerState
	StartTime    time.Time
	db           *database.DB
	notifier     *notifications.Notifier
	apiServer    *api.Server
	exabgp       *exabgp.Client  // NEW: ExaBGP client
}

// Modified Config - ExaBGP instead of Bird
type ExaBGPConfig struct {
	Enabled bool `yaml:"enabled"` // Enable ExaBGP integration
	// Bird fields removed - no longer needed!
}

type Config struct {
	Peers         []PeerConfig
	Thresholds    ThresholdConfig
	Damping       DampingConfig
	Startup       StartupConfig
	ExaBGP        ExaBGPConfig  // NEW: ExaBGP config
	Logging       LoggingConfig
	Mode          ModeConfig
	API           APIConfig
	Database      DatabaseConfig
	Notifications notifications.MainConfig
}

// Modified PeerConfig - add nexthop for ExaBGP
type PeerConfig struct {
	Name             string  `yaml:"name"`
	Hostname         string  `yaml:"hostname"`
	ExpectedBaseline float64 `yaml:"expected_baseline"`
	NextHop          string  `yaml:"nexthop"`  // NEW: Next-hop IPv6 address for BGP announcements
}

// NEW: Announced prefixes (could be in config or hardcoded)
var AnnouncedPrefixes = []string{
	"2a0e:97c0:e61::/48",
	"2602:f9ba:a10::/48",
	"2a0e:97c0:e62::/48",
}

// Replace applyBirdConfiguration() with applyExaBGPConfiguration()
func applyExaBGPConfiguration(state *AppState) error {
	if state.exabgp == nil {
		return fmt.Errorf("ExaBGP client not initialized")
	}

	logger := NewLogger(state.Config.Logging.Level)

	// Get priorities for all peers
	priorities := assignPriorities(state)

	// Update route announcements for each peer
	for name, peer := range state.Peers {
		priority := priorities[name]

		if state.Config.Mode.DryRun {
			logger.Info("DRY-RUN: Would set %s to priority %d via ExaBGP", name, priority)
			continue
		}

		// Announce all prefixes via this peer with the calculated priority
		err := state.exabgp.AnnounceRoutes(
			name,
			peer.Config.NextHop,
			AnnouncedPrefixes,
			priority,
		)

		if err != nil {
			logger.Error("Failed to update ExaBGP routes for %s: %v", name, err)
			return fmt.Errorf("ExaBGP update failed for %s: %w", name, err)
		}

		logger.Debug("Updated ExaBGP: %s priority=%d nexthop=%s", name, priority, peer.Config.NextHop)
	}

	logger.Info("ExaBGP configuration updated successfully")
	return nil
}

// Modified runMonitoringCycle - use ExaBGP instead of Bird
func runMonitoringCycle(state *AppState) {
	// ... measurement code unchanged ...

	// Evaluate health of all peers (with damping)
	evaluatePeerHealth(state)

	// Apply ExaBGP configuration based on current health states
	err := applyExaBGPConfiguration(state)  // CHANGED: ExaBGP instead of Bird
	if err != nil {
		logger := NewLogger(state.Config.Logging.Level)
		logger.Error("Failed to apply ExaBGP configuration: %v", err)
	}

	// Update API server state
	updateAPIServerState(state)
}

// Modified initialization in main()
func main() {
	// ... config loading unchanged ...

	// Initialize ExaBGP client (replaces Bird file operations)
	state.exabgp = exabgp.NewClient()

	logger.Info("Initialized ExaBGP client")

	// ... rest of main() unchanged ...
}

// assignPriorities() stays exactly the same!
// It still returns map[string]int with priorities 1 or 99
// We just send those priorities to ExaBGP instead of writing a config file

// Example config.yaml changes:
/*
# OLD Bird config (REMOVE):
bird:
  priorities_file: /etc/bird/lagbuster-priorities.conf
  birdc_path: /usr/sbin/birdc
  birdc_timeout: 5

# NEW ExaBGP config:
exabgp:
  enabled: true

# Updated peer config (ADD nexthop):
peers:
  - name: edgenyc01
    hostname: edge-nyc01.linuxburken.se
    expected_baseline: 15.0
    nexthop: 2a0e:97c0:e61:ff80::d  # NEW: BGP next-hop

  - name: ash01
    hostname: router-ash01.linuxburken.se
    expected_baseline: 19.0
    nexthop: 2a0e:97c0:e61:ff80::9  # NEW: BGP next-hop
*/
