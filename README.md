# Lagbuster

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

BGP path optimizer based on per-peer latency health monitoring. Lagbuster monitors latency to your edge routers and automatically switches BGP paths when the current primary peer degrades beyond acceptable thresholds.

**GitHub**: [https://github.com/zooper/lagbuster](https://github.com/zooper/lagbuster)

## Key Features

- **Per-peer health monitoring**: Each peer is evaluated against its own baseline, not compared to others
- **Static baselines**: Configure expected latency for each peer to prevent baseline drift
- **Smart decision logic**: Only switches when current primary degrades significantly or is unhealthy
- **Hysteresis/damping**: Prevents route flapping with configurable thresholds
- **Dry-run mode**: Test configuration and decisions without applying changes
- **Comprehensive logging**: All decisions and measurements are logged with rationale

## Architecture

### How It Works

1. **Monitor**: Continuously ping all edge routers at configured intervals
2. **Evaluate**: Compare each peer's current latency against its static baseline
3. **Decide**: If current primary is unhealthy (degraded beyond threshold), select best healthy alternative
4. **Apply**: Update Bird BGP priorities via `lagbuster-priorities.conf` and reload with `birdc configure`

### Health Evaluation

A peer is considered **unhealthy** if:
- Ping fails or times out
- Current latency > expected baseline + degradation threshold (e.g., 45ms + 20ms = 65ms)
- Current latency > absolute maximum threshold (e.g., 150ms)

A peer is considered **comfortable** if:
- Current latency < expected baseline + comfort threshold (e.g., 45ms + 10ms = 55ms)

### Decision Logic

```
IF current primary is healthy AND within comfort zone:
    → Stay on current primary (stability preferred)

IF current primary unhealthy for N consecutive measurements:
    → Switch to best healthy peer (lowest current latency)
    → If all unhealthy, pick least-bad option

IF recently switched (within cooldown period):
    → Stay on current primary (prevent flapping)
```

## Configuration

### Example config.yaml

```yaml
peers:
  - name: edge01
    hostname: edge01.example.com
    expected_baseline: 45.0  # Your expected "good" latency (ms)
    bird_variable: core01_edge01_lagbuster_priority

  - name: edge02
    hostname: edge02.example.com
    expected_baseline: 55.0
    bird_variable: core01_edge02_lagbuster_priority

  - name: edge03
    hostname: edge03.example.com
    expected_baseline: 52.0
    bird_variable: core01_edge03_lagbuster_priority

thresholds:
  degradation_threshold: 20.0    # Switch if peer > baseline + 20ms
  comfort_threshold: 10.0        # Stay if peer < baseline + 10ms
  absolute_max_latency: 150.0    # Hard limit for unhealthy
  timeout_latency: 3000.0        # Treat timeouts as this value

damping:
  consecutive_unhealthy_count: 3  # Require 3 unhealthy before switching
  measurement_interval: 10        # Measure every 10 seconds
  cooldown_period: 180            # Wait 3min after switch
  measurement_window: 20          # Keep last 20 measurements

startup:
  grace_period: 60                # Wait 60s before first change
  initial_primary: edge01         # Start with this peer

bird:
  priorities_file: /etc/bird/lagbuster-priorities.conf
  birdc_path: /usr/sbin/birdc
  birdc_timeout: 5

logging:
  level: info                     # debug, info, warn, error
  log_measurements: false         # Log every ping (verbose)
  log_decisions: true             # Log decision rationale

mode:
  dry_run: false                  # Set true to test without applying
```

### Determining Baseline Values

To find your expected baselines, run lagbuster manually and observe:

```bash
# Run a few pings manually
for i in {1..10}; do
  ping -c 1 edge01.example.com | grep time=
  sleep 1
done

# Or use lagbuster in dry-run mode to observe current latencies
./lagbuster -dry-run
```

Use the typical "good" latency you observe as your `expected_baseline`.

## Installation

> **Note**: This section assumes you're using Ansible to manage Bird configuration. If you're managing Bird manually, see [MANUAL-SETUP.md](MANUAL-SETUP.md) for detailed instructions.

### 1. Build the binary

```bash
cd /path/to/lagbuster
go mod tidy
go build -o lagbuster lagbuster.go
```

### 2. Deploy to router

```bash
# On your core router
sudo mkdir -p /opt/lagbuster
sudo cp lagbuster /opt/lagbuster/
sudo cp config.yaml /opt/lagbuster/
sudo chmod +x /opt/lagbuster/lagbuster
```

### 3. Create initial Bird lagbuster-priorities.conf

This file must exist before Bird starts. Create it with initial default values:

```bash
# On your core router
sudo tee /etc/bird/lagbuster-priorities.conf > /dev/null <<'EOF'
# Lagbuster dynamic priority overrides
# Initial values (will be updated by lagbuster)

define core01_edge01_lagbuster_priority = 1;
define core01_edge02_lagbuster_priority = 3;
define core01_edge03_lagbuster_priority = 2;
EOF
```

### 4. Update Bird configuration (Ansible)

Modify your Ansible `ibgp_edge.conf.j2` template to:
- Include `lagbuster-priorities.conf` at the top
- Use `*_lagbuster_priority` variables in filters instead of `*_priority`

Example changes:

```jinja2
# At the top of ibgp_edge.conf.j2
include "/etc/bird/lagbuster-priorities.conf";

# In the filter logic, change from:
if core01_edge01_priority = 1 then {

# To:
if core01_edge01_lagbuster_priority = 1 then {
```

Run your Ansible playbook to apply these changes.

### 5. Install systemd service

```bash
sudo cp lagbuster.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable lagbuster.service
```

## Usage

### Test in Dry-Run Mode

```bash
# Test locally first
./lagbuster -dry-run -config config.yaml

# Or on the router
sudo /opt/lagbuster/lagbuster -dry-run
```

Watch the logs to verify decision logic is working as expected.

### Start the Service

```bash
sudo systemctl start lagbuster
sudo systemctl status lagbuster
```

### Monitor Logs

```bash
# Follow logs
sudo journalctl -u lagbuster -f

# View recent logs
sudo journalctl -u lagbuster -n 100
```

### Check Current State

```bash
# View generated Bird config
cat /etc/bird/lagbuster-priorities.conf

# Check Bird status
sudo birdc show status
sudo birdc show protocols
```

## Example Scenarios

### Scenario 1: Regional Degradation

```
Initial state:
EDGE01: 45ms (baseline: 45ms) → Primary, healthy
EDGE02: 52ms (baseline: 52ms) → Secondary, healthy
EDGE03: 55ms (baseline: 55ms) → Tertiary, healthy

Region experiences network issues:
EDGE01: 95ms (baseline: 45ms, +50ms degradation) → UNHEALTHY
EDGE02: 53ms (baseline: 52ms, +1ms) → HEALTHY
EDGE03: 100ms (baseline: 55ms, +45ms degradation) → UNHEALTHY

Decision: After 3 consecutive unhealthy measurements
→ Switch from EDGE01 to EDGE02
→ Reason: "current primary degraded (50.00ms above baseline)"
```

### Scenario 2: Minor Fluctuations

```
EDGE01: 52ms (baseline: 45ms, +7ms) → HEALTHY (within comfort threshold)
EDGE02: 51ms (baseline: 52ms)
EDGE03: 54ms (baseline: 55ms)

Decision: Stay on EDGE01
→ Reason: Current primary healthy and within comfort zone
→ No unnecessary switching despite EDGE02 being slightly faster
```

### Scenario 3: All Peers Degraded

```
EDGE01: 120ms → UNHEALTHY
EDGE02: 115ms → UNHEALTHY
EDGE03: 125ms → UNHEALTHY

Decision: Switch to EDGE02 (least-bad option)
→ Reason: All unhealthy, pick lowest current latency
```

## Troubleshooting

### Lagbuster not switching when expected

Check:
1. Is it in cooldown period? (3 minutes after last switch)
2. Has it met consecutive unhealthy count? (default: 3 measurements)
3. Are thresholds configured correctly?
4. Check logs: `journalctl -u lagbuster -n 50`

### Bird not accepting configuration

```bash
# Test Bird config manually
sudo birdc configure check

# View Bird logs
sudo journalctl -u bird -n 50

# Verify lagbuster-priorities.conf syntax
cat /etc/bird/lagbuster-priorities.conf
```

### Baseline seems wrong

Run manual pings to observe typical latency:

```bash
# 100 pings with 1 second interval
ping -c 100 -i 1 edge01.example.com | grep time= | \
  grep -oP 'time=\K[0-9.]+' | \
  awk '{s+=$1; n++} END {print "Avg:", s/n, "ms"}'
```

Update `expected_baseline` in config.yaml accordingly.

### Dry-run mode for testing

Always test configuration changes in dry-run mode first:

```bash
sudo /opt/lagbuster/lagbuster -dry-run
```

This will log all decisions without applying changes to Bird.

## Monitoring

### Key Metrics to Watch

1. **Switch frequency**: How often is lagbuster switching primaries?
   - Too frequent = adjust thresholds or damping
   - Never switching = check if it's working

2. **Peer health**: Are peers consistently healthy or frequently degraded?
   - Frequent degradation = investigate network issues

3. **Decision rationale**: Why is lagbuster making switches?
   - Check logs for decision reasons

### Log Examples

```
[INFO] Lagbuster starting (version 1.0)
[INFO] Initialized with 3 peers, primary: edge01
[INFO] Startup grace period: 60 seconds
[INFO] Peer edge01 became UNHEALTHY: latency=95.23ms, baseline=45.00ms, degradation=50.23ms
[INFO] Best peer selection: edge03 (latency=52.11ms, healthy=true, 2/3 peers healthy)
[INFO] SWITCHING PRIMARY: edge01 -> edge03 | Reason: current primary degraded (50.23ms above baseline)
[INFO]   Old: edge01 latency=95.23ms baseline=45.00ms healthy=false
[INFO]   New: edge03 latency=52.11ms baseline=52.00ms healthy=true
[INFO] Bird configuration updated successfully
```

## Integration with Ansible

Lagbuster is designed to co-exist with Ansible-managed Bird configurations:

### What Ansible Manages
- Main Bird configuration (`bird.conf`)
- BGP peer definitions (`ibgp_edge.conf`)
- Filter logic (uses `*_lagbuster_priority` variables)
- Initial `lagbuster-priorities.conf` template with defaults

### What Lagbuster Manages
- Runtime updates to `/etc/bird/lagbuster-priorities.conf`
- Dynamic priority values based on health monitoring

### Reboot Behavior
1. Bird starts with Ansible-managed defaults (from lagbuster-priorities.conf template)
2. Lagbuster starts via systemd
3. After grace period, lagbuster re-optimizes priorities based on current latency
4. No conflicts with Ansible - it can still safely run to update other Bird configs

## Future Enhancements

Possible improvements (not currently implemented):

- [ ] Packet loss monitoring in addition to latency
- [ ] Adaptive baselines (bounded improvement only)
- [ ] Prometheus metrics export
- [ ] Web dashboard for monitoring
- [ ] Alert integration (email/Slack on primary degradation)
- [ ] Historical data persistence
- [ ] Multi-path load balancing support

## License

MIT License - see LICENSE file for details.

## Contributing

Contributions welcome! Please:
1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

Report bugs or request features by opening an issue on [GitHub](https://github.com/zooper/lagbuster/issues).

## Author

Created by [@zooper](https://github.com/zooper)
