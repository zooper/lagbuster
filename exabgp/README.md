# ExaBGP Integration for Lagbuster

This directory contains everything needed to integrate ExaBGP with Lagbuster for API-driven BGP route control.

## What's Included

### Configuration Files
- **`exabgp.conf`** - ExaBGP configuration for iBGP sessions to edge routers
- **`exabgp-control.py`** - Python control script that receives commands from Lagbuster

### Go Library
- **`exabgp.go`** - Go client library for sending commands to ExaBGP

### Integration Example
- **`lagbuster-exabgp-integration.go`** - Shows how to modify lagbuster.go to use ExaBGP

### Documentation
- **`EXABGP-MIGRATION.md`** - Complete migration guide from Bird to ExaBGP
- **`COMPARISON.md`** - Detailed comparison of Bird vs ExaBGP
- **`README.md`** - This file

### Testing
- **`test-exabgp.sh`** - Test script to validate ExaBGP installation

## Quick Start

### 1. Read the Comparison

**Start here:** Read `COMPARISON.md` to understand:
- How Bird and ExaBGP compare
- Whether you should migrate
- Recommended hybrid approach

**TL;DR:** Keep Bird for ECMP (it works great!), consider adding ExaBGP for advanced features like DDoS mitigation.

### 2. Review Migration Guide

Read `EXABGP-MIGRATION.md` for:
- Step-by-step installation
- Configuration examples
- Testing procedures
- Rollback plan

### 3. Test Installation

On router-jc01:
```bash
# Copy test script
scp test-exabgp.sh router-jc01.tailed6e7.ts.net:/tmp/

# Run tests
ssh router-jc01.tailed6e7.ts.net
chmod +x /tmp/test-exabgp.sh
sudo /tmp/test-exabgp.sh
```

## Architecture

### Current (Bird + Config Files)
```
Lagbuster → /etc/bird/lagbuster-priorities.conf → birdc configure → Bird
```

### With ExaBGP (API-Driven)
```
Lagbuster → Named Pipe (/var/run/lagbuster/exabgp.pipe) → ExaBGP → BGP
```

## Key Files to Modify

To integrate ExaBGP into Lagbuster, you need to modify:

1. **`lagbuster.go`**
   - Add `exabgp` import
   - Add `exabgp.Client` to `AppState`
   - Replace `applyBirdConfiguration()` with `applyExaBGPConfiguration()`

   See `lagbuster-exabgp-integration.go` for examples.

2. **`config.yaml`**
   - Remove `bird:` section
   - Add `exabgp:` section
   - Add `nexthop:` field to each peer

   Example:
   ```yaml
   exabgp:
     enabled: true

   peers:
     - name: edgenyc01
       hostname: edge-nyc01.linuxburken.se
       expected_baseline: 15.0
       nexthop: 2a0e:97c0:e61:ff80::d  # NEW
   ```

## Benefits of ExaBGP

✅ **Direct API control** - No config file writes
✅ **Faster updates** - ~3ms vs ~13ms
✅ **More flexible** - Flowspec, blackholing, dynamic routes
✅ **Programmatic** - Native Python/Go integration

## When to Use ExaBGP

### ✅ Good Use Cases
- Real-time DDoS mitigation (Flowspec)
- Dynamic blackhole routing
- Emergency traffic engineering
- Automated route injection
- Integration with monitoring/automation

### ❌ Not Needed For
- Basic ECMP routing (Bird works great!)
- Static routing policies
- Simple failover
- If you're happy with config files

## Hybrid Approach (Recommended)

**Best of both worlds:**
- Keep Bird for stable ECMP routing
- Add ExaBGP for dynamic features

This approach:
- ✅ Maintains stability (Bird)
- ✅ Adds flexibility (ExaBGP)
- ✅ Low risk (additive change)
- ✅ Future-proof (can expand ExaBGP use)

## Support

### ExaBGP Documentation
- GitHub: https://github.com/Exa-Networks/exabgp
- Wiki: https://github.com/Exa-Networks/exabgp/wiki

### Troubleshooting

**ExaBGP won't start:**
```bash
# Check config syntax
exabgp --test /etc/exabgp/exabgp.conf

# Check logs
sudo journalctl -u exabgp -n 50
```

**Named pipe issues:**
```bash
# Recreate pipe
sudo rm /var/run/lagbuster/exabgp.pipe
sudo mkdir -p /var/run/lagbuster
# ExaBGP control script will recreate it

# Test manually
echo '{"action":"announce","prefixes":["2001:db8::/48"],"peer":"test","priority":1,"nexthop":"::1"}' > /var/run/lagbuster/exabgp.pipe
```

**BGP sessions not establishing:**
```bash
# Check connectivity
ping6 2a0e:97c0:e61:ff80::d
ping6 2a0e:97c0:e61:ff80::9

# Check ExaBGP neighbor config
cat /etc/exabgp/exabgp.conf | grep -A 15 "neighbor"

# Monitor session establishment
sudo journalctl -u exabgp -f
```

## Next Steps

1. **Read** `COMPARISON.md` to understand the trade-offs
2. **Decide** if ExaBGP is right for your use case
3. **Test** ExaBGP installation on a non-production system
4. **Migrate** using the guide in `EXABGP-MIGRATION.md`

## Questions?

- Check `COMPARISON.md` for Bird vs ExaBGP analysis
- Read `EXABGP-MIGRATION.md` for step-by-step guide
- Review `lagbuster-exabgp-integration.go` for code examples
