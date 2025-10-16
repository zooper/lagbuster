# Manual Setup Guide (Without Ansible)

This guide shows how to set up lagbuster if you're managing your Bird configuration manually (without Ansible).

## Overview

If you're not using Ansible, you'll need to:
1. Manually create the Bird configuration files
2. Edit your Bird config to include lagbuster priority variables
3. Deploy and configure lagbuster

## Prerequisites

- Bird 2.0+ installed and running
- Root/sudo access to your router
- SSH access to your router
- Go 1.20+ (for building lagbuster)

## Step 1: Build Lagbuster

On your local machine:

```bash
git clone https://github.com/zooper/lagbuster.git
cd lagbuster

# Install dependencies
go mod download

# Build
go build -o lagbuster lagbuster.go
```

## Step 2: Create Lagbuster Configuration

Create your `config.yaml` based on the example:

```bash
cp config.example.yaml config.yaml
nano config.yaml
```

Update with your actual values:

```yaml
peers:
  - name: edge01
    hostname: edge01.yourdomain.com  # Your actual edge router hostname
    expected_baseline: 45.0           # Measure and set appropriate baseline
    bird_variable: core_edge01_lagbuster_priority  # Match Bird config

  - name: edge02
    hostname: edge02.yourdomain.com
    expected_baseline: 52.0
    bird_variable: core_edge02_lagbuster_priority

  - name: edge03
    hostname: edge03.yourdomain.com
    expected_baseline: 55.0
    bird_variable: core_edge03_lagbuster_priority

# ... configure thresholds, damping, etc.
```

## Step 3: Create Bird Priority Configuration File

On your router, create the lagbuster priorities file:

```bash
# SSH to your router
ssh your-router

# Create the priorities file
sudo nano /etc/bird/lagbuster-priorities.conf
```

Add the following content (adjust variable names to match your config.yaml):

```bird
# Lagbuster dynamic priority overrides
# This file is managed by lagbuster at runtime
# Initial values set here, updated dynamically based on latency

# Priority values: 1=primary, 2=secondary, 3=tertiary
# Lower number = higher preference (better route)

define core_edge01_lagbuster_priority = 1;  # edge01 is primary
define core_edge02_lagbuster_priority = 2;  # edge02 is secondary
define core_edge03_lagbuster_priority = 3;  # edge03 is tertiary
```

Set proper permissions:

```bash
sudo chown root:root /etc/bird/lagbuster-priorities.conf
sudo chmod 644 /etc/bird/lagbuster-priorities.conf
```

## Step 4: Update Your Bird Configuration

### Option A: iBGP Edge Peers (Core ↔ Edge Setup)

If you have a core router with multiple edge routers (like the reference setup):

Edit your Bird configuration file (e.g., `/etc/bird/bird.conf` or `/etc/bird/ibgp_edge.conf`):

```bird
# Include lagbuster priorities at the top of your config
include "lagbuster-priorities.conf";

# Define your BGP peers using the lagbuster priority variables
protocol bgp EDGE01 {
    local as YOUR_ASN;
    neighbor EDGE01_IP as YOUR_ASN;
    source address YOUR_SOURCE_IP;
    
    ipv6 {
        import filter {
            # Use lagbuster priority variable for local preference
            if core_edge01_lagbuster_priority = 1 then {
                bgp_local_pref = 130;  # Primary
            } else if core_edge01_lagbuster_priority = 2 then {
                bgp_local_pref = 120;  # Secondary
            } else if core_edge01_lagbuster_priority = 3 then {
                bgp_local_pref = 110;  # Tertiary
            } else {
                bgp_local_pref = 90;   # Default/disabled
            }
            accept;
        };
        
        export filter {
            # Optionally add prepending for backup paths
            if core_edge01_lagbuster_priority = 2 then {
                bgp_path.prepend(YOUR_ASN);
                bgp_path.prepend(YOUR_ASN);
            } else if core_edge01_lagbuster_priority = 3 then {
                bgp_path.prepend(YOUR_ASN);
                bgp_path.prepend(YOUR_ASN);
                bgp_path.prepend(YOUR_ASN);
            }
            accept;
        };
    };
}

# Repeat for EDGE02, EDGE03, etc.
protocol bgp EDGE02 {
    # ... similar config using core_edge02_lagbuster_priority
}

protocol bgp EDGE03 {
    # ... similar config using core_edge03_lagbuster_priority
}
```

### Option B: eBGP Transit Providers

If you're using lagbuster to select between multiple transit providers:

```bird
# Include lagbuster priorities
include "lagbuster-priorities.conf";

protocol bgp TRANSIT_A {
    local as YOUR_ASN;
    neighbor TRANSIT_A_IP as TRANSIT_A_ASN;
    
    ipv6 {
        import filter {
            # Apply lagbuster priority via local_pref
            if transit_a_lagbuster_priority = 1 then {
                bgp_local_pref = 200;  # Primary
            } else if transit_a_lagbuster_priority = 2 then {
                bgp_local_pref = 150;  # Secondary
            } else {
                bgp_local_pref = 100;  # Tertiary
            }
            accept;
        };
        export all;
    };
}

protocol bgp TRANSIT_B {
    # ... similar config using transit_b_lagbuster_priority
}
```

## Step 5: Validate and Reload Bird Configuration

Test the configuration before applying:

```bash
# Parse check
sudo bird -p -c /etc/bird/bird.conf

# If successful, reload Bird
sudo birdc configure
```

Verify the variables are loaded:

```bash
# Check symbols are defined
echo 'show symbols' | sudo birdc | grep lagbuster_priority

# Check values
echo 'eval core_edge01_lagbuster_priority' | sudo birdc
echo 'eval core_edge02_lagbuster_priority' | sudo birdc
echo 'eval core_edge03_lagbuster_priority' | sudo birdc
```

## Step 6: Deploy Lagbuster

Copy files to your router:

```bash
# From your local machine
scp lagbuster config.yaml your-router:/tmp/
scp lagbuster.service your-router:/tmp/
```

On the router, install lagbuster:

```bash
# SSH to router
ssh your-router

# Create directory
sudo mkdir -p /opt/lagbuster

# Move files
sudo mv /tmp/lagbuster /opt/lagbuster/
sudo mv /tmp/config.yaml /opt/lagbuster/
sudo chmod +x /opt/lagbuster/lagbuster

# Install systemd service
sudo mv /tmp/lagbuster.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable lagbuster.service
```

## Step 7: Test in Dry-Run Mode

Before starting the service, test lagbuster:

```bash
sudo /opt/lagbuster/lagbuster -dry-run
```

Watch the output for 2-3 minutes. You should see:
- Latency measurements for each peer
- Health evaluations
- Decision-making logic
- "DRY-RUN: Would update Bird configuration" messages

Example output:
```
[INFO] Lagbuster starting (version 1.0)
[INFO] Running in DRY-RUN mode - no changes will be applied
[INFO] Initialized with 3 peers, primary: edge01
[INFO] Startup grace period: 60 seconds
[DEBUG] Peer edge01: latency=45.23ms, baseline=45.00ms
[DEBUG] Peer edge02: latency=52.11ms, baseline=52.00ms
[DEBUG] Peer edge03: latency=55.67ms, baseline=55.00ms
[DEBUG] Current primary edge01 is healthy and comfortable (degradation=0.23ms)
```

## Step 8: Start Lagbuster Service

Once satisfied with dry-run testing:

```bash
# Start the service
sudo systemctl start lagbuster

# Check status
sudo systemctl status lagbuster

# Follow logs
sudo journalctl -u lagbuster -f
```

## Step 9: Verify It's Working

### Check Lagbuster Status

```bash
# View recent logs
sudo journalctl -u lagbuster -n 50

# Check for any errors
sudo journalctl -u lagbuster -p err
```

### Check Bird Configuration

```bash
# View current priorities
cat /etc/bird/lagbuster-priorities.conf

# Check Bird loaded the values
echo 'eval core_edge01_lagbuster_priority' | sudo birdc
```

### Check BGP Routes

```bash
# Show routes with local_pref
echo 'show route all' | sudo birdc | grep -A5 local_pref

# Show BGP protocols
echo 'show protocols' | sudo birdc
```

## Maintenance

### Update Configuration

To change lagbuster settings:

```bash
# Edit config
sudo nano /opt/lagbuster/config.yaml

# Restart service
sudo systemctl restart lagbuster
```

### Manual Priority Override

To temporarily override lagbuster:

```bash
# Stop lagbuster
sudo systemctl stop lagbuster

# Edit priorities manually
sudo nano /etc/bird/lagbuster-priorities.conf

# Reload Bird
sudo birdc configure

# Start lagbuster again when ready
sudo systemctl start lagbuster
```

### Reset to Defaults

To reset to original priorities:

```bash
# Edit lagbuster-priorities.conf back to defaults
sudo nano /etc/bird/lagbuster-priorities.conf

# Reload Bird
sudo birdc configure

# Restart lagbuster
sudo systemctl restart lagbuster
```

## Troubleshooting

### Lagbuster Not Starting

```bash
# Check logs for errors
sudo journalctl -u lagbuster -n 100

# Common issues:
# - Config file not found: Check /opt/lagbuster/config.yaml exists
# - Permission denied: Check file ownership and execute permissions
# - Bird not running: Check sudo systemctl status bird
```

### No Priority Changes Happening

```bash
# Check if in cooldown
sudo journalctl -u lagbuster | grep cooldown

# Check thresholds in config
cat /opt/lagbuster/config.yaml | grep -A5 thresholds

# Manually test ping
ping -c 5 edge01.yourdomain.com
```

### Bird Configuration Errors

```bash
# Test Bird config
sudo bird -p -c /etc/bird/bird.conf

# Check include path
grep "include.*lagbuster" /etc/bird/*.conf

# Verify file exists
ls -la /etc/bird/lagbuster-priorities.conf
```

## Complete Example Configuration

Here's a complete example for a simple setup with 3 edge routers:

### /opt/lagbuster/config.yaml

```yaml
peers:
  - name: edge01
    hostname: edge01.example.com
    expected_baseline: 45.0
    bird_variable: core_edge01_lagbuster_priority

  - name: edge02
    hostname: edge02.example.com
    expected_baseline: 52.0
    bird_variable: core_edge02_lagbuster_priority

  - name: edge03
    hostname: edge03.example.com
    expected_baseline: 55.0
    bird_variable: core_edge03_lagbuster_priority

thresholds:
  degradation_threshold: 20.0
  comfort_threshold: 10.0
  absolute_max_latency: 150.0
  timeout_latency: 3000.0

damping:
  consecutive_unhealthy_count: 3
  measurement_interval: 10
  cooldown_period: 180
  measurement_window: 20

startup:
  grace_period: 60
  initial_primary: edge01

bird:
  priorities_file: /etc/bird/lagbuster-priorities.conf
  birdc_path: /usr/sbin/birdc
  birdc_timeout: 5

logging:
  level: info
  log_measurements: false
  log_decisions: true

mode:
  dry_run: false
```

### /etc/bird/lagbuster-priorities.conf

```bird
# Lagbuster dynamic priorities
define core_edge01_lagbuster_priority = 1;
define core_edge02_lagbuster_priority = 2;
define core_edge03_lagbuster_priority = 3;
```

### /etc/bird/bird.conf (excerpt)

```bird
# Router ID and ASN
router id 10.0.0.1;
define OWNASN = 65000;

# Include lagbuster priorities
include "lagbuster-priorities.conf";

# BGP protocol to edge01
protocol bgp EDGE01 {
    local as OWNASN;
    neighbor 10.0.1.1 as OWNASN;
    source address 10.0.0.1;
    
    ipv6 {
        import filter {
            if core_edge01_lagbuster_priority = 1 then {
                bgp_local_pref = 130;
            } else if core_edge01_lagbuster_priority = 2 then {
                bgp_local_pref = 120;
            } else {
                bgp_local_pref = 110;
            }
            accept;
        };
        export all;
    };
}

# Similar configs for EDGE02 and EDGE03...
```

## Next Steps

- Monitor lagbuster logs for a few days to ensure stable operation
- Adjust thresholds based on your network characteristics
- Set up monitoring/alerting for lagbuster service health

## Support

For issues or questions:
- Check logs: `journalctl -u lagbuster -n 100`
- Review configuration: `cat /opt/lagbuster/config.yaml`
- Test in dry-run: `sudo /opt/lagbuster/lagbuster -dry-run`
- Open an issue on [GitHub](https://github.com/zooper/lagbuster/issues)
