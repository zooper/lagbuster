# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Lagbuster is a BGP path optimizer that monitors latency to edge routers and automatically switches BGP paths when the current primary peer degrades beyond acceptable thresholds. Written in Go as a single-file application (`lagbuster.go`), it integrates with Bird routing daemon to dynamically adjust BGP priorities based on real-time latency measurements.

**Key Concept**: Per-peer health monitoring with static baselines. Each peer is evaluated against its own expected baseline, not compared to others. This prevents baseline drift and ensures predictable failover behavior.

## Architecture

### Core Components

The application follows a monitoring loop architecture with these main phases:

1. **Monitor**: Continuously ping all edge routers at configured intervals (default: 10s)
2. **Evaluate**: Compare each peer's current latency against its static baseline
3. **Decide**: Apply decision logic with hysteresis/damping to prevent route flapping
4. **Apply**: Update Bird BGP priorities via `lagbuster-priorities.conf` and reload with `birdc configure`

### Key Data Structures

- `AppState` (lagbuster.go:93-99): Top-level runtime state containing config, peer states, and current primary
- `PeerState` (lagbuster.go:83-91): Per-peer runtime state with latency measurements, health status, consecutive unhealthy counter, and consecutive healthy counter
- `Config` (lagbuster.go:22-31): Configuration loaded from YAML file with nested structures for thresholds, damping, failback, Bird integration, etc.

### Decision Logic Flow

The decision engine (`selectPrimary()` at lagbuster.go:376-443) follows this hierarchy:

1. **Failback check**: If failback enabled and preferred primary has been healthy for required duration, switch back to it
2. **Cooldown check**: If recently switched (within `cooldown_period`), stay on current primary (unless failback)
3. **Comfort zone**: If current primary is healthy AND within comfort threshold, stay (stability preferred)
4. **Damping**: Only switch if current primary unhealthy for N consecutive measurements (`consecutive_unhealthy_count`)
5. **Best alternative**: Find healthiest peer with lowest latency

Health evaluation (`isPeerHealthy()` at lagbuster.go:356-374):
- Unhealthy if: ping fails (latency = -1), latency > baseline + degradation_threshold, OR latency > absolute_max_latency
- Comfortable if: latency < baseline + comfort_threshold

### Bird Integration

Lagbuster manages Bird configuration through:
- Writes to `/etc/bird/lagbuster-priorities.conf` (or configured path)
- Defines Bird variables like `define core01_edge01_lagbuster_priority = 1;`
- Priority values: 1=primary (best), 2=secondary, 3=tertiary
- Triggers Bird reload with `birdc configure` command
- Verifies reconfiguration success by checking for "Reconfigured" in output

Bird configs use these variables in import filters to set `bgp_local_pref` values, controlling route preference.

## Development Commands

### Building

```bash
# Install dependencies
go mod download

# Build binary
go build -o lagbuster lagbuster.go

# Build with optimizations
go build -ldflags="-s -w" -o lagbuster lagbuster.go
```

### Testing

```bash
# Run in dry-run mode (no changes applied to Bird)
./lagbuster -dry-run -config config.yaml

# Run with custom config
./lagbuster -config /path/to/config.yaml

# Test configuration loading
go run lagbuster.go -dry-run
```

### Deployment

```bash
# Deploy to router
scp lagbuster config.yaml your-router:/tmp/
ssh your-router
sudo mkdir -p /opt/lagbuster
sudo mv /tmp/lagbuster /opt/lagbuster/
sudo mv /tmp/config.yaml /opt/lagbuster/
sudo chmod +x /opt/lagbuster/lagbuster

# Install systemd service
sudo cp lagbuster.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable lagbuster.service
sudo systemctl start lagbuster
```

### Monitoring

```bash
# Follow logs
sudo journalctl -u lagbuster -f

# View recent logs
sudo journalctl -u lagbuster -n 100

# Check current Bird priorities
cat /etc/bird/lagbuster-priorities.conf

# Verify Bird configuration
sudo birdc configure check
sudo birdc show protocols
```

## Configuration

Example configuration structure in `config.yaml`:

- **peers**: Array of edge routers with hostname, expected_baseline (ms), and bird_variable name
- **thresholds**: degradation_threshold, comfort_threshold, absolute_max_latency, timeout_latency
- **damping**: consecutive_unhealthy_count, measurement_interval, cooldown_period, measurement_window
- **startup**: grace_period (delay before first change), initial_primary, preferred_primary (optional, for failback)
- **failback**: enabled, consecutive_healthy_count, require_cooldown_before_failback
- **bird**: priorities_file path, birdc_path, birdc_timeout
- **logging**: level (debug/info/warn/error), log_measurements, log_decisions
- **mode**: dry_run flag

See `config.example.yaml` for complete reference.

## Platform-Specific Details

### Ping Command Differences

The `pingHost()` function (lagbuster.go:258-305) handles OS-specific ping syntax with context-based timeout protection:

- **macOS**: `ping -c 1 -t 3 host` (ping is IPv4 by default, -t for timeout)
- **Linux**: `ping -4 -c 1 -W 3000 host` (-4 forces IPv4, -W timeout in milliseconds)
- **Timeout protection**: Uses `context.WithTimeout()` with 5-second deadline to prevent hanging on unreachable hosts or DNS issues
- **Return value**: Returns -1 on timeout/failure, latency in milliseconds on success

Always uses IPv4 to ensure consistent measurements across platforms.

### File Operations

Configuration updates use atomic write pattern:
1. Write to `file.tmp`
2. Atomic rename to `file` (prevents partial reads by Bird)

## Important Behaviors

### Startup Behavior

1. Loads configuration from file
2. Initializes peer states with `initial_primary`
3. Waits `grace_period` seconds before making any changes
4. Runs first measurement immediately after grace period
5. **Applies initial Bird configuration** based on first measurement (even if no switch occurs)
6. Then runs on `measurement_interval` ticker

This ensures Bird is always synchronized with lagbuster's state on startup and priorities are optimized based on actual measured latencies, not just the configured `initial_primary`.

### Damping/Hysteresis

Multiple mechanisms prevent route flapping:
- **Consecutive unhealthy**: Requires N consecutive unhealthy measurements before switching away
- **Consecutive healthy**: Tracks N consecutive healthy measurements for failback logic
- **Cooldown period**: Minimum time between switches (prevents rapid oscillation)
- **Comfort threshold**: Creates hysteresis band where current primary is preferred even if another peer is slightly better
- **Measurement window**: Keeps rolling history for future enhancements (not currently used in decision logic)

### Automatic Failback

When enabled, lagbuster will automatically switch back to the `preferred_primary` peer after it recovers:
- Tracks consecutive healthy measurements for each peer
- Failback occurs when preferred primary has been healthy for `consecutive_healthy_count` measurements
- Optional cooldown protection: `require_cooldown_before_failback` ensures minimum time since last switch
- Prevents comparing latencies between geographically different peers
- Logged with reason: "failback to preferred primary (sustained health: N measurements)"

### Logging

The Logger wrapper (lagbuster.go:92-122) supports hierarchical levels:
- `debug`: All messages including per-measurement details
- `info`: Normal operations, decisions, switches
- `warn`: Potential issues
- `error`: Failures only

Key log patterns:
- Health transitions: "Peer X became UNHEALTHY/HEALTHY" (with reason for unhealthy: unreachable/timeout, exceeds max, or degradation)
- Switches: "SWITCHING PRIMARY: X -> Y | Reason: ..." (includes failback, unreachable, degraded, or comfort zone)
- Failback: "Failing back to preferred primary X (healthy for N consecutive measurements)"
- Best peer selection: Shows which peer selected and why
- Timeout warnings: "Ping to X timed out after 5 seconds (host may be unreachable or DNS hanging)"
- Decision rationale: Logged when `log_decisions: true`

## Code Style

- Single-file application for simplicity
- Struct-based configuration with YAML tags
- Pass-by-pointer for state mutations
- Clear function naming: `isPeerHealthy()`, `selectPrimary()`, `switchPrimary()`
- Comprehensive logging with rationale for decisions
- Error handling with wrapped errors (`fmt.Errorf("context: %w", err)`)

## Integration Points

### With Ansible

Lagbuster coexists with Ansible-managed Bird configurations:
- Ansible manages: Main Bird config, BGP peer definitions, filter logic templates
- Lagbuster manages: Runtime priority values in `lagbuster-priorities.conf`
- No conflicts: Ansible can safely run to update other configs
- Reboot behavior: Bird starts with Ansible defaults, lagbuster re-optimizes after grace period

See `ansible-example-changes.md` for Ansible integration details.

### With Bird

Required Bird configuration structure:
- Include lagbuster-priorities.conf at top of config
- Use `*_lagbuster_priority` variables in import filters
- Map priority values to bgp_local_pref (e.g., priority 1 → local_pref 130)
- Optional: Use priority for AS path prepending in export filters

See `MANUAL-SETUP.md` for detailed Bird configuration examples.

## Dependencies

- `gopkg.in/yaml.v3`: YAML configuration parsing
- Standard library only otherwise (no external runtime dependencies)
- Requires: Bird 2.0+, birdc command, ping command

## Files Structure

- `lagbuster.go`: Complete application (~600 lines)
- `config.yaml`: Active configuration (user creates from example)
- `config.example.yaml`: Configuration template with documentation
- `lagbuster.service`: systemd service definition
- `README.md`: Complete user documentation
- `MANUAL-SETUP.md`: Detailed setup guide for non-Ansible users
- `ansible-example-changes.md`: Ansible integration reference
- `CLAUDE.md`: This file - AI assistant guidance

## Recent Enhancements (2025-10-17)

**Critical Bug Fixes:**
1. **Context-based timeout protection**: Added `context.WithTimeout()` to `pingHost()` to prevent infinite hanging when hosts are unreachable or DNS fails. This fixes a critical issue where lagbuster would completely freeze during network outages.

2. **Improved unhealthy logging**: Better error messages showing "unreachable/timeout" instead of confusing negative latency values.

**New Features:**
3. **Automatic failback**: Added `preferred_primary` configuration and automatic failback logic. When enabled, lagbuster will switch back to the preferred peer after it has been healthy for a configurable duration (default: 30 minutes). This allows geographically-dispersed peers without comparing latencies.

## Future Enhancements

Listed in README.md but not yet implemented:
- Packet loss monitoring (currently latency only)
- Adaptive baselines (currently static only)
- Prometheus metrics export
- Web dashboard
- Alert integration (email/Slack)
- Historical data persistence
- Multi-path load balancing support
