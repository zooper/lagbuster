# Lagbuster Quick Start Guide

> **Note**: This guide uses example hostnames and configuration from a real deployment. Replace `router-jc01`, `*.linuxburken.se`, and `jc01_*` variables with your own environment's values.

Step-by-step guide to get lagbuster running on your router.

## Prerequisites

- [ ] Go 1.20+ installed locally (for building)
- [ ] SSH access to router-jc01
- [ ] Root/sudo access on router-jc01
- [ ] Bird 2.0+ running on router-jc01
- [ ] Ansible playbook access for Bird configuration

## Step 1: Build Lagbuster

On your local machine:

```bash
cd /Users/jonsson/dev/private/lagbuster

# Download dependencies
go mod tidy

# Build binary
go build -o lagbuster lagbuster.go

# Verify build
./lagbuster -help
```

## Step 2: Configure Baselines

First, determine expected baselines by measuring current latency:

```bash
# Test from your local machine or from router-jc01
for host in router-nyc01.linuxburken.se router-nyc02.linuxburken.se router-ash01.linuxburken.se; do
  echo "=== $host ==="
  ping -c 10 $host | grep -E 'time=|avg'
done
```

Edit `config.yaml` and set `expected_baseline` values based on your observations:

```yaml
peers:
  - name: nyc01
    expected_baseline: 45.0  # Use your measured average
  - name: nyc02
    expected_baseline: 55.0  # Use your measured average
  - name: ash01
    expected_baseline: 52.0  # Use your measured average
```

## Step 3: Test Locally in Dry-Run Mode

```bash
# Test configuration is valid
./lagbuster -dry-run -config config.yaml

# Let it run for a minute and observe the logs
# Press Ctrl+C to stop

# Verify decision logic makes sense
```

Expected output:
```
[INFO] Lagbuster starting (version 1.0)
[INFO] Running in DRY-RUN mode - no changes will be applied
[INFO] Initialized with 3 peers, primary: nyc01
[INFO] Startup grace period: 60 seconds
[DEBUG] Current primary nyc01 is healthy and comfortable (degradation=2.34ms)
[INFO] DRY-RUN: Would update Bird configuration
```

## Step 4: Update Ansible Configuration

See `ansible-example-changes.md` for detailed instructions.

**Summary:**
1. Create `templates/lagbuster-priorities.conf.j2`
2. Modify `templates/ibgp_edge.conf.j2` to use `*_lagbuster_priority` variables
3. Update `deploy-bird.yml` to deploy lagbuster-priorities.conf
4. Run Ansible playbook

```bash
cd /Users/jonsson/dev/private/bird/ansible

# Test deployment (dry-run)
ansible-playbook -i inventory.yml deploy-bird.yml --check --diff -l router-jc01

# Deploy for real
ansible-playbook -i inventory.yml deploy-bird.yml -l router-jc01
```

## Step 5: Verify Ansible Changes

SSH to router-jc01 and verify:

```bash
# Check lagbuster-priorities.conf was created
cat /etc/bird/lagbuster-priorities.conf

# Should show something like:
# define jc01_nyc01_lagbuster_priority = 1;
# define jc01_nyc02_lagbuster_priority = 3;
# define jc01_ash01_lagbuster_priority = 2;

# Verify Bird accepts the config
sudo birdc configure check

# Should show: Configuration OK

# Check variables are defined
echo 'show symbols' | sudo birdc | grep lagbuster_priority

# Should show all three variables as constants
```

## Step 6: Deploy Lagbuster to Router

```bash
# From your local machine
cd /Users/jonsson/dev/private/lagbuster

# Copy files to router
scp lagbuster config.yaml router-jc01:/tmp/

# SSH to router
ssh router-jc01

# On router-jc01:
sudo mkdir -p /opt/lagbuster
sudo mv /tmp/lagbuster /opt/lagbuster/
sudo mv /tmp/config.yaml /opt/lagbuster/
sudo chmod +x /opt/lagbuster/lagbuster

# Test on router in dry-run mode
sudo /opt/lagbuster/lagbuster -dry-run

# Let it run for 2-3 minutes to see decision-making
# Press Ctrl+C when satisfied
```

## Step 7: Install Systemd Service

```bash
# On your local machine, copy service file
scp lagbuster.service router-jc01:/tmp/

# On router-jc01:
sudo mv /tmp/lagbuster.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable lagbuster.service
```

## Step 8: Start Lagbuster

```bash
# On router-jc01:
sudo systemctl start lagbuster

# Check status
sudo systemctl status lagbuster

# Follow logs
sudo journalctl -u lagbuster -f
```

Expected log output:
```
[INFO] Lagbuster starting (version 1.0)
[INFO] Initialized with 3 peers, primary: nyc01
[INFO] Startup grace period: 60 seconds
[INFO] Current primary nyc01 is healthy and comfortable
```

## Step 9: Verify Integration

### Check Generated Config

```bash
# View lagbuster-generated priorities
cat /etc/bird/lagbuster-priorities.conf

# Should show current priorities and health status in comments
```

### Check Bird Status

```bash
# Show BGP protocols
echo 'show protocols' | sudo birdc

# Show routes with local_pref
echo 'show route all' | sudo birdc | grep -B2 -A2 local_pref

# Verify priority values
echo 'eval jc01_nyc01_lagbuster_priority' | sudo birdc
```

### Monitor for Switches

```bash
# Watch logs for any switches
sudo journalctl -u lagbuster -f --grep="SWITCHING"
```

## Step 10: Simulate Degradation (Optional Test)

To test lagbuster's response to degradation:

### Option A: Firewall Rules (Temporary Delay)

```bash
# On router-jc01: Add artificial latency to NYC01
sudo tc qdisc add dev NYC01_TO_JC01 root netem delay 100ms

# Watch lagbuster logs
sudo journalctl -u lagbuster -f

# Should see:
# [INFO] Peer nyc01 became UNHEALTHY: latency=145.00ms, baseline=45.00ms
# [INFO] SWITCHING PRIMARY: nyc01 -> ash01

# Remove delay
sudo tc qdisc del dev NYC01_TO_JC01 root
```

### Option B: Manual Priority Override

```bash
# Manually set ASH01 as primary to simulate lagbuster action
sudo tee /etc/bird/lagbuster-priorities.conf > /dev/null <<'EOF'
# Manual test
define jc01_nyc01_lagbuster_priority = 2;
define jc01_nyc02_lagbuster_priority = 3;
define jc01_ash01_lagbuster_priority = 1;
EOF

sudo birdc configure

# Check BGP routes reflect new priority
echo 'show route all' | sudo birdc | grep -A3 local_pref

# Lagbuster will overwrite this file on next cycle based on actual latency
```

## Troubleshooting

### Lagbuster Not Starting

```bash
# Check service status
sudo systemctl status lagbuster

# View full logs
sudo journalctl -u lagbuster -n 100

# Common issues:
# - Config file not found: Check /opt/lagbuster/config.yaml exists
# - Permission denied: Ensure binary is executable
# - Bird not running: Check `systemctl status bird`
```

### No Switches Happening

```bash
# Check if in cooldown
sudo journalctl -u lagbuster | grep cooldown

# Check if damping is preventing switch
sudo journalctl -u lagbuster | grep damping

# Verify thresholds are appropriate
cat /opt/lagbuster/config.yaml | grep -A5 thresholds
```

### Bird Config Errors

```bash
# Test Bird config
sudo birdc configure check

# Check for syntax errors in lagbuster-priorities.conf
cat /etc/bird/lagbuster-priorities.conf

# Restart Bird if needed
sudo systemctl restart bird
```

## Monitoring Checklist

After deployment, monitor these:

- [ ] Lagbuster service is running: `systemctl status lagbuster`
- [ ] No errors in logs: `journalctl -u lagbuster -p err`
- [ ] Bird accepting configs: Check for "Reconfigured" in logs
- [ ] Peer health status: All peers should be healthy under normal conditions
- [ ] Switch frequency: Should be rare (only when actual degradation occurs)

## Daily Operations

### View Current Status

```bash
# Quick status check
sudo systemctl status lagbuster
cat /etc/bird/lagbuster-priorities.conf

# See which peer is primary (priority=1)
grep 'priority = 1' /etc/bird/lagbuster-priorities.conf
```

### Review Recent Switches

```bash
# Show all switches in last 24 hours
sudo journalctl -u lagbuster --since "24 hours ago" | grep SWITCHING

# See reasons for switches
sudo journalctl -u lagbuster --since "24 hours ago" | grep "Reason:"
```

### Adjust Configuration

```bash
# Edit config on router
sudo nano /opt/lagbuster/config.yaml

# Restart lagbuster to apply changes
sudo systemctl restart lagbuster

# Monitor logs to verify new config
sudo journalctl -u lagbuster -f
```

## Rollback Plan

If you need to disable lagbuster:

```bash
# Stop lagbuster
sudo systemctl stop lagbuster
sudo systemctl disable lagbuster

# Manually set priorities back to defaults
sudo tee /etc/bird/lagbuster-priorities.conf > /dev/null <<'EOF'
define jc01_nyc01_lagbuster_priority = 1;
define jc01_nyc02_lagbuster_priority = 3;
define jc01_ash01_lagbuster_priority = 2;
EOF

sudo birdc configure
```

## Next Steps

Once lagbuster is stable:

1. Monitor for a week to observe switching behavior
2. Adjust thresholds if switches are too frequent or too rare
3. Consider adding monitoring/alerting for lagbuster service health
4. Document any peer-specific issues you discover

## Support

For issues or questions:
- Check logs: `journalctl -u lagbuster -n 100`
- Review configuration: `cat /opt/lagbuster/config.yaml`
- Test in dry-run: `sudo /opt/lagbuster/lagbuster -dry-run`
- See README.md for detailed documentation
