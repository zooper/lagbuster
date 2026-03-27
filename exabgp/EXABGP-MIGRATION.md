# ExaBGP Integration Migration Guide

This guide explains how to migrate from Bird config-file approach to ExaBGP API-driven approach for lagbuster.

## Architecture Change

### Before (Bird Config File)
```
Lagbuster → Write /etc/bird/lagbuster-priorities.conf → birdc configure → Bird
           (config file)                                 (reload)
```

### After (ExaBGP API)
```
Lagbuster → Named Pipe → ExaBGP Control Script → ExaBGP → BGP Sessions
           (JSON API)    (Python)                (direct)
```

## What Changes

### router-jc01 (Core Router)
- **REMOVE:** Bird iBGP sessions
- **ADD:** ExaBGP with API control
- **KEEP:** Nothing changes about routing logic - just the implementation

### Edges (edge-nyc01, router-ash01)
- **NO CHANGES!** Edges keep Bird for transit BGP
- They still receive iBGP routes with communities
- They still apply prepending based on communities

## Benefits of ExaBGP

✅ **Direct API control** - No config file writes
✅ **Instant updates** - No `birdc configure` delays
✅ **More flexible** - Can do flowspec, blackholing, dynamic routes
✅ **Programmatic** - Native Python API for automation
✅ **Cleaner** - Lagbuster speaks directly to routing daemon

## Installation

### 1. Install ExaBGP on router-jc01

```bash
ssh router-jc01.tailed6e7.ts.net

# Install Python 3 (if not already installed)
sudo apt update
sudo apt install python3 python3-pip

# Install ExaBGP
sudo pip3 install exabgp

# Verify installation
exabgp --version
# Should show: ExaBGP 4.2.x or newer
```

### 2. Deploy ExaBGP Configuration

```bash
# On your local machine
cd ~/dev/private/lagbuster

# Copy ExaBGP config files to router
scp exabgp/exabgp.conf router-jc01.tailed6e7.ts.net:/tmp/
scp exabgp/exabgp-control.py router-jc01.tailed6e7.ts.net:/tmp/

# SSH to router and install
ssh router-jc01.tailed6e7.ts.net

# Create ExaBGP config directory
sudo mkdir -p /etc/exabgp
sudo cp /tmp/exabgp.conf /etc/exabgp/
sudo chmod 644 /etc/exabgp/exabgp.conf

# Install control script
sudo cp /tmp/exabgp-control.py /opt/lagbuster/
sudo chmod +x /opt/lagbuster/exabgp-control.py

# Create pipe directory
sudo mkdir -p /var/run/lagbuster
sudo chown lagbuster:lagbuster /var/run/lagbuster  # If running as lagbuster user
```

### 3. Create ExaBGP Systemd Service

```bash
# On router-jc01
sudo tee /etc/systemd/system/exabgp.service << 'EOF'
[Unit]
Description=ExaBGP - BGP Route Injector
Documentation=https://github.com/Exa-Networks/exabgp
After=network.target

[Service]
Type=simple
User=root
Environment=exabgp.daemon.user=root
ExecStart=/usr/local/bin/exabgp /etc/exabgp/exabgp.conf
ExecReload=/bin/kill -USR1 $MAINPID
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# Enable and start ExaBGP
sudo systemctl daemon-reload
sudo systemctl enable exabgp
sudo systemctl start exabgp

# Check status
sudo systemctl status exabgp

# Monitor logs
sudo journalctl -u exabgp -f
```

### 4. Update Lagbuster Configuration

Update `/opt/lagbuster/config.yaml`:

```yaml
# REMOVE old bird section:
# bird:
#   priorities_file: /etc/bird/lagbuster-priorities.conf
#   birdc_path: /usr/sbin/birdc
#   birdc_timeout: 5

# ADD new exabgp section:
exabgp:
  enabled: true

# UPDATE peers - add nexthop addresses:
peers:
  - name: edgenyc01
    hostname: edge-nyc01.linuxburken.se
    expected_baseline: 15.0
    nexthop: 2a0e:97c0:e61:ff80::d  # IPv6 next-hop for BGP announcements

  - name: ash01
    hostname: router-ash01.linuxburken.se
    expected_baseline: 19.0
    nexthop: 2a0e:97c0:e61:ff80::9  # IPv6 next-hop for BGP announcements
```

### 5. Deploy Updated Lagbuster

The lagbuster binary needs to be updated with ExaBGP integration. This requires code changes to `lagbuster.go`:

**Key changes needed:**
1. Add `exabgp` import
2. Add `exabgp.Client` to `AppState`
3. Replace `applyBirdConfiguration()` with `applyExaBGPConfiguration()`
4. Update config structs (remove Bird, add ExaBGP)

See `exabgp/lagbuster-exabgp-integration.go` for code examples.

```bash
# On your dev machine
cd ~/dev/private/lagbuster

# Build with ExaBGP support (after making code changes)
GOOS=linux GOARCH=amd64 go build -o lagbuster-linux lagbuster.go

# Deploy to router
scp lagbuster-linux router-jc01.tailed6e7.ts.net:/tmp/lagbuster-exabgp

# On router
ssh router-jc01.tailed6e7.ts.net
sudo systemctl stop lagbuster
sudo cp /opt/lagbuster/lagbuster /opt/lagbuster/lagbuster.bird-backup
sudo cp /tmp/lagbuster-exabgp /opt/lagbuster/lagbuster
sudo chmod +x /opt/lagbuster/lagbuster
```

## Migration Steps

### Phase 1: Preparation (Can do now)

1. ✅ Install ExaBGP on router-jc01
2. ✅ Deploy ExaBGP config and control script
3. ✅ Create systemd service for ExaBGP
4. ✅ Test ExaBGP starts successfully

**At this point:** Bird still running, ExaBGP running alongside (not connected yet)

### Phase 2: Testing (Parallel environment)

1. Test ExaBGP control script manually:

```bash
# On router-jc01
# Send test command to ExaBGP
echo '{"action":"announce","prefixes":["2001:db8:test::/48"],"peer":"edgenyc01","priority":1,"nexthop":"2a0e:97c0:e61:ff80::d"}' > /var/run/lagbuster/exabgp.pipe

# Check ExaBGP logs
sudo journalctl -u exabgp -n 20

# Should see: "Announced 2001:db8:test::/48 via 2a0e:97c0:e61:ff80::d..."
```

2. Verify BGP session establishment:

```bash
# Check ExaBGP status
sudo systemctl status exabgp

# Monitor ExaBGP logs for BGP session establishment
sudo journalctl -u exabgp -f
# Look for: "Connected to 2a0e:97c0:e61:ff80::d"
```

### Phase 3: Cutover (Switching to ExaBGP)

**Preparation:**
- Backup current Bird config: `sudo cp /etc/bird/bird.conf /etc/bird/bird.conf.backup`
- Stop lagbuster: `sudo systemctl stop lagbuster`
- Stop Bird on router-jc01: `sudo systemctl stop bird`

**Deploy:**
1. Deploy lagbuster with ExaBGP support
2. Start lagbuster: `sudo systemctl start lagbuster`
3. Monitor logs: `sudo journalctl -u lagbuster -f`

**Verification:**
```bash
# Check lagbuster is running
sudo systemctl status lagbuster

# Check ExaBGP sessions
sudo journalctl -u exabgp -n 50 | grep -i established

# Check routes are being announced
sudo journalctl -u exabgp | grep "Announced"

# Verify from edges that they receive routes
ssh edge-nyc01.tailed6e7.ts.net "sudo birdc 'show route protocol JC01'"
```

### Phase 4: Validation

1. **Test ECMP is working:**
```bash
# From router-jc01, check if you have default route via both edges
# (This depends on edges' config - they should still announce default via iBGP)

# Test failover by simulating peer failure
# Temporarily block ping to one edge
sudo iptables -A OUTPUT -d edge-nyc01.linuxburken.se -p icmp -j DROP

# Wait ~30 seconds for lagbuster to detect unhealthy
# Check ExaBGP updated announcements (should see 8x prepending)
sudo journalctl -u exabgp | tail -20

# Restore
sudo iptables -D OUTPUT -d edge-nyc01.linuxburken.se -p icmp -j DROP
```

2. **Test Web UI:**
```
http://router-jc01.tailed6e7.ts.net:8080
```
Should show healthy peers and routing mode

## Rollback Plan

If issues arise, rollback to Bird:

```bash
# Stop lagbuster and ExaBGP
sudo systemctl stop lagbuster
sudo systemctl stop exabgp

# Restore Bird
sudo systemctl start bird
sudo birdc configure

# Restore old lagbuster
sudo cp /opt/lagbuster/lagbuster.bird-backup /opt/lagbuster/lagbuster
sudo systemctl start lagbuster

# Verify
sudo systemctl status lagbuster
sudo systemctl status bird
```

## Comparison: Bird vs ExaBGP

| Feature | Bird + Config File | ExaBGP + API |
|---------|-------------------|--------------|
| **Control Method** | Write config, reload | Direct API calls |
| **Latency** | ~10-20ms (file write + reload) | ~1-5ms (pipe write) |
| **Flexibility** | Limited to config syntax | Full programmatic control |
| **Maturity** | Very mature, production proven | Mature for route injection |
| **Complexity** | Simple (config files) | Moderate (API + script) |
| **Debugging** | Config files visible | JSON commands in logs |
| **RPKI** | Native support | Not supported |
| **Flowspec** | Limited support | Excellent support |

## Edge Router Configs (NO CHANGES!)

The edge routers (edge-nyc01, router-ash01) **don't need any changes**:

- They still run Bird
- They still receive iBGP routes from jc01
- They still apply prepending based on communities
- The only difference is router-jc01 now uses ExaBGP instead of Bird

The community tags `(215855, 100, 99)` still work exactly the same way!

## Troubleshooting

### ExaBGP won't start
```bash
# Check config syntax
exabgp --test /etc/exabgp/exabgp.conf

# Check logs
sudo journalctl -u exabgp -n 50
```

### Named pipe doesn't work
```bash
# Check pipe exists
ls -la /var/run/lagbuster/exabgp.pipe

# Check permissions
sudo chown lagbuster:lagbuster /var/run/lagbuster/exabgp.pipe

# Test manually
echo '{"action":"announce","prefixes":["2001:db8::/48"],"peer":"test","priority":1,"nexthop":"::1"}' > /var/run/lagbuster/exabgp.pipe
```

### BGP sessions not establishing
```bash
# Check ExaBGP neighbor config
cat /etc/exabgp/exabgp.conf | grep -A 10 "neighbor"

# Check IP addresses are correct
ip -6 addr show

# Check connectivity
ping6 2a0e:97c0:e61:ff80::d
ping6 2a0e:97c0:e61:ff80::9

# Check firewall
sudo ip6tables -L -n | grep 179
```

### Routes not being announced
```bash
# Monitor ExaBGP logs
sudo journalctl -u exabgp -f

# Check control script is running
ps aux | grep exabgp-control

# Send test command
echo '{"action":"announce","prefixes":["2a0e:97c0:e61::/48"],"peer":"test","priority":1,"nexthop":"2a0e:97c0:e61:ff80::d"}' > /var/run/lagbuster/exabgp.pipe

# Should see in logs: "Announced 2a0e:97c0:e61::/48..."
```

## Next Steps

After successful migration, you can explore ExaBGP's advanced features:

1. **Flowspec for DDoS mitigation**
2. **Dynamic blackhole routing**
3. **Real-time traffic engineering**
4. **Advanced BGP communities**

See ExaBGP documentation: https://github.com/Exa-Networks/exabgp
