#!/bin/bash
# Test script for ExaBGP integration
# Run this on router-jc01 after deploying ExaBGP

set -e

PIPE="/var/run/lagbuster/exabgp.pipe"
TEST_PREFIX="2001:db8:test::/48"

echo "=== ExaBGP Integration Test ==="
echo

# Test 1: Check ExaBGP is running
echo "[1/7] Checking ExaBGP service..."
if systemctl is-active --quiet exabgp; then
    echo "✅ ExaBGP service is running"
else
    echo "❌ ExaBGP service is not running"
    echo "   Start with: sudo systemctl start exabgp"
    exit 1
fi
echo

# Test 2: Check named pipe exists
echo "[2/7] Checking named pipe..."
if [ -p "$PIPE" ]; then
    echo "✅ Named pipe exists: $PIPE"
else
    echo "❌ Named pipe not found: $PIPE"
    echo "   ExaBGP control script should create it automatically"
    echo "   Check ExaBGP logs: sudo journalctl -u exabgp -n 50"
    exit 1
fi
echo

# Test 3: Check ExaBGP logs for startup
echo "[3/7] Checking ExaBGP startup logs..."
if sudo journalctl -u exabgp --no-pager -n 50 | grep -q "exabgp-control.*started"; then
    echo "✅ ExaBGP control script started successfully"
else
    echo "⚠️  Warning: Could not find control script startup message"
    echo "   This might be okay - continuing..."
fi
echo

# Test 4: Check BGP sessions
echo "[4/7] Checking BGP session status..."
if sudo journalctl -u exabgp --no-pager | grep -q "Connected"; then
    echo "✅ BGP sessions appear to be established"
    sudo journalctl -u exabgp --no-pager | grep "Connected" | tail -2
else
    echo "⚠️  Warning: No BGP connection messages found"
    echo "   This might be okay if sessions are still establishing"
fi
echo

# Test 5: Send test announcement (priority 1 - healthy)
echo "[5/7] Testing route announcement (priority 1)..."
TEST_CMD='{"action":"announce","prefixes":["'$TEST_PREFIX'"],"peer":"test","priority":1,"nexthop":"2a0e:97c0:e61:ff80::d"}'
echo "   Sending: $TEST_CMD"
echo "$TEST_CMD" > "$PIPE"
sleep 2

if sudo journalctl -u exabgp --no-pager -n 20 | grep -q "Announced.*$TEST_PREFIX"; then
    echo "✅ Route announcement successful"
    sudo journalctl -u exabgp --no-pager -n 20 | grep "Announced.*$TEST_PREFIX"
else
    echo "❌ Route announcement not found in logs"
    echo "   Check ExaBGP logs for errors"
    exit 1
fi
echo

# Test 6: Send test announcement (priority 99 - unhealthy with prepending)
echo "[6/7] Testing route announcement (priority 99)..."
TEST_CMD='{"action":"announce","prefixes":["'$TEST_PREFIX'"],"peer":"test","priority":99,"nexthop":"2a0e:97c0:e61:ff80::d"}'
echo "   Sending: $TEST_CMD"
echo "$TEST_CMD" > "$PIPE"
sleep 2

if sudo journalctl -u exabgp --no-pager -n 20 | grep -q "Announced.*$TEST_PREFIX"; then
    echo "✅ Route announcement with prepending successful"
    sudo journalctl -u exabgp --no-pager -n 20 | grep "Announced.*$TEST_PREFIX" | tail -1
else
    echo "❌ Route announcement not found in logs"
    exit 1
fi
echo

# Test 7: Send test withdrawal
echo "[7/7] Testing route withdrawal..."
TEST_CMD='{"action":"withdraw","prefixes":["'$TEST_PREFIX'"],"peer":"test","nexthop":"2a0e:97c0:e61:ff80::d"}'
echo "   Sending: $TEST_CMD"
echo "$TEST_CMD" > "$PIPE"
sleep 2

if sudo journalctl -u exabgp --no-pager -n 20 | grep -q "Withdrew.*$TEST_PREFIX"; then
    echo "✅ Route withdrawal successful"
    sudo journalctl -u exabgp --no-pager -n 20 | grep "Withdrew.*$TEST_PREFIX"
else
    echo "❌ Route withdrawal not found in logs"
    exit 1
fi
echo

echo "==================================="
echo "✅ All tests passed!"
echo
echo "ExaBGP is ready for Lagbuster integration."
echo
echo "Next steps:"
echo "1. Deploy updated lagbuster binary with ExaBGP support"
echo "2. Update /opt/lagbuster/config.yaml with nexthop addresses"
echo "3. Restart lagbuster: sudo systemctl restart lagbuster"
echo "4. Monitor: sudo journalctl -u lagbuster -f"
echo "==================================="
