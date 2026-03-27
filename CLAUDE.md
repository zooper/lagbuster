# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Lagbuster is a BGP path optimizer that monitors latency to edge routers and enables asymmetric routing (ECMP) by automatically managing BGP priorities based on peer health. It integrates with Bird routing daemon to dynamically adjust BGP priorities: all healthy peers receive equal priority for ECMP routing, while unhealthy peers are automatically disabled.

**Key Concept**: Per-peer health monitoring with static baselines and asymmetric damping. Each peer is independently evaluated against its own expected baseline (not compared to others). Health state transitions use damping to prevent route flapping: peers degrade quickly (after 3 consecutive bad measurements) but recover slowly (after 12 consecutive good measurements), ensuring bad paths are removed quickly while preventing flapping.

## Architecture

### Package Structure

The application is organized into multiple packages:

```
lagbuster/
├── lagbuster.go           # Main entry point and monitoring loop
├── api/                   # REST API and WebSocket server
│   ├── server.go          # HTTP server setup and routes
│   ├── handlers.go        # API endpoint handlers
│   └── websocket.go       # Real-time WebSocket broadcasting
├── database/              # SQLite persistence layer
│   ├── db.go              # Database operations
│   └── schema.sql         # Embedded database schema
├── notifications/         # Alert notification system
│   ├── notifier.go        # Core notification logic with rate limiting
│   ├── email.go           # Email (SMTP) channel
│   ├── slack.go           # Slack webhook channel
│   └── telegram.go        # Telegram bot channel
└── webui/                 # Web dashboard
    ├── frontend/          # React TypeScript application
    └── backend/           # Node.js development proxy
```

### Core Components

The application follows a monitoring loop architecture with these main phases:

1. **Monitor**: Continuously ping all edge routers and check BGP session status at configured intervals (default: 10s)
2. **Evaluate**: Compare each peer's current latency against its static baseline with damping
3. **Apply**: Update Bird BGP priorities via `lagbuster-priorities.conf` and reload with `birdc configure` (all healthy peers get priority 1 for ECMP, unhealthy peers get priority 99)
4. **Persist**: Record measurements and events to SQLite database
5. **Notify**: Send alerts via configured channels (Email, Slack, Telegram)
6. **Broadcast**: Push real-time updates to connected WebSocket clients

### Key Data Structures

- `AppState` (lagbuster.go:~111): Top-level runtime state containing config, peer states, database connection, notifier, and API server
- `PeerState` (lagbuster.go:~99): Per-peer runtime state with latency measurements, health status, consecutive counters, and BGP session state
- `Config` (lagbuster.go:~25): Configuration loaded from YAML file with nested structures for thresholds, damping, Bird integration, API, database, and notifications

### Decision Logic Flow

The asymmetric routing model uses independent per-peer health evaluation with damping:

**Health Evaluation** (`evaluatePeerHealth()` at lagbuster.go:~532):
1. **Measure**: Check current latency vs baseline using `isPeerHealthy()`
2. **Track**: Increment consecutive healthy/unhealthy counters
3. **Dampen degradation**: Peer becomes unhealthy after `consecutive_unhealthy_count` consecutive bad measurements (default: 3)
4. **Dampen recovery**: Peer becomes healthy after `consecutive_healthy_count_for_recovery` consecutive good measurements (default: 12)
5. **Notify**: Send notifications only on actual health state transitions

**Health Criteria** (`isPeerHealthy()` at lagbuster.go:~610):
- Unhealthy if: ping fails (latency = -1), latency > baseline + degradation_threshold, OR latency > absolute_max_latency
- Healthy otherwise

**Priority Assignment** (`assignPriorities()` at lagbuster.go:~935):
- All healthy peers with established BGP sessions: priority 1 (ECMP)
- Unhealthy or BGP-down peers: priority 99 (disabled)

This simple model eliminates complex primary selection logic, failback logic, cooldown periods, and comfort zones. Every peer is independently evaluated and managed.

### BGP Session Awareness

Lagbuster monitors BGP session status for each peer via `birdc show protocols`:
- `checkBGPSession()` (lagbuster.go:~477) queries Bird for each peer's BGP state
- Peers with non-established BGP sessions receive priority 99 (disabled) regardless of health
- Only peers with "Established" BGP sessions can receive priority 1
- BGP states tracked: Established, Active, Connect, Idle, Down, Unknown

### Bird Integration

Lagbuster manages Bird configuration through:
- Writes to `/etc/bird/lagbuster-priorities.conf` (or configured path)
- Defines Bird variables like `define core01_edge01_lagbuster_priority = 1;`
- Priority values: 1=active (ECMP routing), 99=disabled
- Triggers Bird reload with `birdc configure` command
- Verifies reconfiguration success by checking for "Reconfigured" in output

Bird configs use these variables in import filters to set `bgp_local_pref` values. All peers with priority 1 get equal local_pref for ECMP, while priority 99 peers are filtered out or get very low local_pref.

### REST API

The API server (`api/` package) provides:

**Endpoints:**
- `GET /api/status` - Current system status with healthy/unhealthy peer counts, uptime, and all peer states
- `GET /api/peers` - All peer statuses with latency, health, and BGP state
- `GET /api/metrics?peer=X&range=1h|24h|7d|30d` - Historical latency measurements
- `GET /api/events?range=1h|24h|7d|30d&type=health_change` - System events (primarily health changes)
- `GET /api/settings/notifications` - Current notification configuration
- `PUT /api/settings/notifications` - Update notification settings
- `POST /api/settings/notifications/test` - Send test notification

**WebSocket:**
- `ws://host:port/ws` - Real-time status updates (broadcasts every 10 seconds)
- Event broadcasting for health changes

### Web Dashboard

The web UI (`webui/frontend/`) is a React TypeScript application with:

**Pages:**
- **Dashboard**: Real-time peer status cards with latency and BGP state
- **Metrics**: Historical latency graphs with time range selection
- **Events**: Event log with type filtering
- **Settings**: Notification channel configuration and testing

**Development:**
```bash
cd webui/frontend && npm install && npm start
```

The frontend proxies API requests to the lagbuster API server.

### Notification System

The notification system (`notifications/` package) supports:

**Channels:**
- **Email**: SMTP with TLS, configurable recipients
- **Slack**: Webhook integration with formatted messages
- **Telegram**: Bot API with chat ID targeting

**Event Types:**
- `unhealthy` - Peer became unhealthy (degraded or unreachable)
- `recovery` - Peer recovered to healthy
- `startup` - Lagbuster started
- `test` - Test notification

**Features:**
- Per-channel event type filtering
- Global rate limiting (configurable minutes between same event types)
- Runtime configuration updates via API

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

### Web UI Development

```bash
# Install frontend dependencies
cd webui/frontend && npm install

# Start development server (proxies to lagbuster API)
npm start

# Build for production
npm run build
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

# Query API status
curl http://localhost:8080/api/status | jq

# View recent events
curl "http://localhost:8080/api/events?range=24h" | jq
```

## Configuration

Example configuration structure in `config.yaml`:

- **peers**: Array of edge routers with hostname, expected_baseline (ms), and bird_variable name
- **thresholds**: degradation_threshold, absolute_max_latency, timeout_latency
- **damping**: consecutive_unhealthy_count, consecutive_healthy_count_for_recovery, measurement_interval, measurement_window
- **startup**: grace_period (delay before first configuration change)
- **bird**: priorities_file path, birdc_path, birdc_timeout
- **logging**: level (debug/info/warn/error), log_measurements, log_decisions
- **mode**: dry_run flag
- **api**: enabled, listen_address (e.g., `:8080`)
- **database**: path (SQLite file), retention_days
- **notifications**: Global notification settings
  - enabled, rate_limit_minutes
  - **email**: SMTP settings (smtp_host, smtp_port, username, password, from, to, events)
  - **slack**: Webhook settings (webhook_url, events)
  - **telegram**: Bot settings (bot_token, chat_id, events)

See `config.example.yaml` for complete reference.

## Platform-Specific Details

### Ping Command Differences

The `pingHost()` function (lagbuster.go:430-474) handles OS-specific ping syntax with context-based timeout protection:

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
2. Initializes all peer states as healthy (will be evaluated on first cycle)
3. Waits `grace_period` seconds before making any configuration changes
4. Runs first measurement immediately after grace period
5. **Applies initial Bird configuration** based on first measurement
6. Then runs on `measurement_interval` ticker

This ensures Bird is synchronized with lagbuster's state on startup and priorities are set based on actual measured health states.

### Damping/Hysteresis

Asymmetric damping prevents route flapping while quickly removing bad paths:
- **Consecutive unhealthy**: Requires N consecutive unhealthy measurements before marking peer as unhealthy (default: 3 measurements, ~30 seconds)
- **Consecutive healthy for recovery**: Requires M consecutive healthy measurements before marking unhealthy peer as healthy again (default: 12 measurements, ~2 minutes)
- **Asymmetric damping rationale**: Degrade quickly to remove bad paths immediately, recover slowly to ensure sustained health before re-adding paths
- **Measurement window**: Keeps rolling history for potential future enhancements (not currently used in decision logic)

No cooldown periods or comfort zones are needed since there's no single primary to protect - each peer is independently managed.

### Logging

The Logger wrapper (lagbuster.go:122-151) supports hierarchical levels:
- `debug`: All messages including per-measurement details
- `info`: Normal operations, decisions, switches
- `warn`: Potential issues
- `error`: Failures only

Key log patterns:
- Health transitions: "Peer X became UNHEALTHY after N consecutive unhealthy measurements: [reason]" (reason: unreachable/timeout, exceeds max, or degradation)
- Recovery: "Peer X became HEALTHY after M consecutive healthy measurements: latency=Xms, baseline=Yms"
- Timeout warnings: "Ping to X timed out after 5 seconds (host may be unreachable or DNS hanging)"
- BGP state: "BGP session X state: Established (up=true)" or "BGP session X state: Active (up=false)"
- Bird configuration: "Applying initial Bird configuration (asymmetric routing mode)"
- Measurement details: Logged when `log_measurements: true`

## Code Style

- Multi-package architecture with clear separation of concerns
- Main logic in `lagbuster.go`, supporting packages for API, database, notifications
- Struct-based configuration with YAML tags
- Pass-by-pointer for state mutations
- Interface-based abstractions (Logger, Channel) for testability
- Clear function naming: `isPeerHealthy()`, `evaluatePeerHealth()`, `assignPriorities()`, `applyBirdConfiguration()`
- Comprehensive logging with rationale for health transitions
- Error handling with wrapped errors (`fmt.Errorf("context: %w", err)`)
- Thread-safe state access with `sync.RWMutex` in API server

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
- Map priority values to bgp_local_pref for ECMP:
  - Priority 1 (healthy, BGP up): High local_pref (e.g., 130) for active ECMP routing
  - Priority 99 (unhealthy/BGP down): Low local_pref (e.g., 50) or filter out completely
- Bird will automatically balance traffic across all peers with equal local_pref (ECMP)

See `MANUAL-SETUP.md` for detailed Bird configuration examples.

## Dependencies

Go dependencies:
- `gopkg.in/yaml.v3`: YAML configuration parsing
- `github.com/gorilla/mux`: HTTP router for REST API
- `github.com/gorilla/websocket`: WebSocket support for real-time updates
- `github.com/mattn/go-sqlite3`: SQLite database driver

System requirements:
- Bird 2.0+ with birdc command
- ping command
- SQLite3 (for database features)

Web UI dependencies (optional):
- Node.js 18+ (for development)
- React 18, TypeScript, Chart.js (bundled in webui/frontend)

## Files Structure

```
lagbuster/
├── lagbuster.go              # Main application (~1000 lines)
├── api/
│   ├── server.go             # HTTP/WebSocket server
│   ├── handlers.go           # REST endpoint handlers
│   └── websocket.go          # WebSocket event broadcasting
├── database/
│   ├── db.go                 # SQLite operations
│   └── schema.sql            # Database schema (embedded)
├── notifications/
│   ├── notifier.go           # Notification dispatcher with rate limiting
│   ├── email.go              # SMTP email channel
│   ├── slack.go              # Slack webhook channel
│   └── telegram.go           # Telegram bot channel
├── webui/
│   ├── frontend/             # React TypeScript dashboard
│   └── backend/              # Node.js development proxy
├── config.yaml               # Active configuration (user creates)
├── config.example.yaml       # Configuration template
├── lagbuster.service         # systemd service definition
├── README.md                 # User documentation
├── MANUAL-SETUP.md           # Setup guide for non-Ansible users
├── ansible-example-changes.md # Ansible integration reference
└── CLAUDE.md                 # This file - AI assistant guidance
```

## Recent Enhancements

### 2026-02-28 Updates

**Asymmetric Routing / ECMP:**
- **Complete architectural change** from single-primary failover to asymmetric routing (ECMP)
- All healthy peers with established BGP sessions receive equal priority (priority 1) for multi-path routing
- Unhealthy or BGP-down peers automatically disabled (priority 99)
- Independent per-peer health evaluation with asymmetric damping (degrade fast, recover slow)
- Removed obsolete concepts: primary selection, failback, cooldown periods, comfort zones
- Simplified configuration: removed `initial_primary`, `preferred_primary`, `failback`, `cooldown_period`, `comfort_threshold`
- Added `consecutive_healthy_count_for_recovery` for recovery damping
- Web UI updated to show healthy peer count instead of current primary
- API responses updated to remove primary-related fields

**BGP Session Awareness:**
- Added BGP session monitoring via `birdc show protocols`
- Peers with non-established BGP sessions are automatically excluded (priority 99)
- BGP state displayed in API and Web UI

**Notification System Enhancements:**
- Added notification channel rebuild infrastructure for runtime config changes
- Test notification feature to verify channel configuration
- Configuration persistence from Web UI
- Event types simplified to: unhealthy, recovery, startup, test

### 2025-11 to 2026-01 Updates

**REST API Server** (`api/` package):
- Full REST API for status, peers, metrics, and events
- WebSocket support for real-time status updates
- Notification settings management endpoints
- CORS support for web UI development

**SQLite Database** (`database/` package):
- Persistent storage for latency measurements and events
- Configurable data retention with automatic cleanup
- WAL mode for better concurrency

**Notification System** (`notifications/` package):
- Email notifications via SMTP
- Slack notifications via webhook
- Telegram notifications via bot API
- Per-channel event type filtering
- Rate limiting to prevent notification spam

**Web Dashboard** (`webui/`):
- React TypeScript single-page application
- Real-time peer status with latency graphs
- Historical metrics visualization (1h, 24h, 7d, 30d)
- Event log with filtering
- Notification settings management

### 2025-10-17 Updates

**Critical Bug Fixes:**
1. **Context-based timeout protection**: Added `context.WithTimeout()` to `pingHost()` to prevent infinite hanging when hosts are unreachable or DNS fails.
2. **Improved unhealthy logging**: Better error messages showing "unreachable/timeout" instead of confusing negative latency values.

**New Features:**
3. **Automatic failback**: Added `preferred_primary` configuration and automatic failback logic.

## Future Enhancements

Not yet implemented:
- Packet loss monitoring (currently latency only)
- Adaptive baselines (currently static only)
- Prometheus metrics export

## Deployment Notes

- When ssh'ing into the router, always use `-A` for ssh-key forwarding
- When deploying new changes, commit and push them, then log on to the router and pull the changes