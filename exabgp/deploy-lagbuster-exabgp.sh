#!/bin/bash
# Deploy lagbuster with ExaBGP mode enabled

set -e

ROUTER="router-jc01.tailed6e7.ts.net"
LAGBUSTER_DIR="/opt/lagbuster"

echo "=========================================="
echo "Deploying Lagbuster with ExaBGP Support"
echo "=========================================="
echo

# Step 1: Create ExaBGP-enabled config
echo "[1/4] Creating ExaBGP-enabled config..."
cat > /tmp/config-exabgp.yaml << 'EOF'
# Lagbuster Configuration - ExaBGP Mode

# Edge router peers to monitor
peers:
  - name: edgenyc01
    hostname: edge-nyc01.linuxburken.se
    expected_baseline: 15.0
    bird_variable: jc01_edgenyc01_lagbuster_priority  # Not used in ExaBGP mode
    nexthop: "2a0e:97c0:e61:ff80::d"  # IPv6 BGP next-hop

  - name: ash01
    hostname: router-ash01.linuxburken.se
    expected_baseline: 19.0
    bird_variable: jc01_ash01_lagbuster_priority
    nexthop: "2a0e:97c0:e61:ff80::9"

# Health check thresholds
thresholds:
  degradation_threshold: 20.0
  absolute_max_latency: 100.0
  timeout_latency: 3000.0

# Damping settings
damping:
  consecutive_unhealthy_count: 3
  consecutive_healthy_count_for_recovery: 12
  measurement_interval: 10
  measurement_window: 20

# Startup behavior
startup:
  grace_period: 30

# Bird integration (still needed for BGP session status checks)
bird:
  priorities_file: /etc/bird/lagbuster-priorities.conf
  birdc_path: /usr/sbin/birdc
  birdc_timeout: 5

# ExaBGP integration - ENABLED
exabgp:
  enabled: true

# Prefixes to announce via ExaBGP
announced_prefixes:
  - "2a0e:97c0:e61::/48"

# Logging
logging:
  level: info
  log_measurements: false
  log_decisions: true

# Operational mode
mode:
  dry_run: false

# Web API
api:
  enabled: true
  listen_address: "0.0.0.0:8080"

# Database (disabled - no SQLite in static binary)
database:
  path: ""
  retention_days: 0

# Notifications
notifications:
  enabled: false
EOF

echo "✅ Config created"
echo

# Step 2: Deploy lagbuster binary
echo "[2/4] Deploying lagbuster binary..."
cd "$(dirname "$0")/.."
scp lagbuster-linux "$ROUTER:/tmp/lagbuster-exabgp"
scp /tmp/config-exabgp.yaml "$ROUTER:/tmp/config-exabgp.yaml"

ssh "$ROUTER" 'bash -s' << 'ENDSSH'
    # Backup existing
    sudo cp /opt/lagbuster/lagbuster /opt/lagbuster/lagbuster.backup-bird 2>/dev/null || true
    sudo cp /opt/lagbuster/config.yaml /opt/lagbuster/config.yaml.backup-bird 2>/dev/null || true

    # Install new binary and config
    sudo cp /tmp/lagbuster-exabgp /opt/lagbuster/lagbuster
    sudo cp /tmp/config-exabgp.yaml /opt/lagbuster/config.yaml
    sudo chmod +x /opt/lagbuster/lagbuster

    echo "Lagbuster binary and config deployed"
ENDSSH
echo "✅ Binary deployed"
echo

# Step 3: Restart lagbuster
echo "[3/4] Restarting lagbuster..."
ssh "$ROUTER" 'bash -s' << 'ENDSSH'
    sudo systemctl restart lagbuster

    # Wait for startup
    sleep 3

    echo "Lagbuster status:"
    sudo systemctl status lagbuster --no-pager -l || true
ENDSSH
echo "✅ Lagbuster restarted"
echo

# Step 4: Verify operation
echo "[4/4] Verifying ExaBGP integration..."
echo
echo "Checking lagbuster logs..."
ssh "$ROUTER" 'sudo journalctl -u lagbuster -n 30 --no-pager' || true
echo
echo "Checking ExaBGP logs..."
ssh "$ROUTER" 'sudo journalctl -u exabgp -n 30 --no-pager' || true
echo

echo "=========================================="
echo "Deployment Complete!"
echo "=========================================="
echo
echo "Monitor lagbuster: ssh $ROUTER 'sudo journalctl -u lagbuster -f'"
echo "Monitor ExaBGP: ssh $ROUTER 'sudo journalctl -u exabgp -f'"
echo "Check status: ssh $ROUTER 'sudo systemctl status lagbuster exabgp'"
echo
echo "To rollback to Bird mode:"
echo "  ssh $ROUTER 'sudo systemctl stop lagbuster'"
echo "  ssh $ROUTER 'sudo cp /opt/lagbuster/lagbuster.backup-bird /opt/lagbuster/lagbuster'"
echo "  ssh $ROUTER 'sudo cp /opt/lagbuster/config.yaml.backup-bird /opt/lagbuster/config.yaml'"
echo "  ssh $ROUTER 'sudo systemctl start lagbuster'"
echo
