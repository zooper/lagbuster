#!/bin/bash
# Deploy ExaBGP integration to router-jc01
# This script installs ExaBGP, stops Bird iBGP, and starts ExaBGP with lagbuster

set -e

ROUTER="router-jc01.tailed6e7.ts.net"
LAGBUSTER_DIR="/opt/lagbuster"
EXABGP_DIR="/etc/exabgp"

echo "==================================="
echo "ExaBGP Deployment for router-jc01"
echo "==================================="
echo

# Step 1: Check prerequisites
echo "[1/9] Checking prerequisites..."
if ! command -v scp &> /dev/null; then
    echo "Error: scp not found. Please install openssh-client."
    exit 1
fi
echo "✅ Prerequisites OK"
echo

# Step 2: Build lagbuster binary
echo "[2/9] Building lagbuster Linux binary..."
cd "$(dirname "$0")/.."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o lagbuster-linux lagbuster.go
if [ ! -f lagbuster-linux ]; then
    echo "❌ Build failed"
    exit 1
fi
echo "✅ Binary built: $(ls -lh lagbuster-linux | awk '{print $5}')"
echo

# Step 3: Copy files to router
echo "[3/9] Copying files to router..."
scp lagbuster-linux "$ROUTER:/tmp/lagbuster-exabgp"
scp exabgp/exabgp.conf "$ROUTER:/tmp/"
scp exabgp/exabgp-control.py "$ROUTER:/tmp/"
scp exabgp/test-exabgp.sh "$ROUTER:/tmp/"
echo "✅ Files copied"
echo

# Step 4: Install ExaBGP
echo "[4/9] Installing ExaBGP..."
ssh "$ROUTER" 'bash -s' << 'ENDSSH'
    # Check if ExaBGP is already installed
    if command -v exabgp &> /dev/null; then
        echo "ExaBGP already installed: $(exabgp --version 2>&1 | head -1)"
    else
        echo "Installing ExaBGP..."
        sudo apt update
        sudo apt install -y python3 python3-pip
        sudo pip3 install exabgp
        echo "ExaBGP installed: $(exabgp --version 2>&1 | head -1)"
    fi
ENDSSH
echo "✅ ExaBGP installed"
echo

# Step 5: Deploy ExaBGP configuration
echo "[5/9] Deploying ExaBGP configuration..."
ssh "$ROUTER" 'bash -s' << 'ENDSSH'
    # Create ExaBGP config directory
    sudo mkdir -p /etc/exabgp
    sudo cp /tmp/exabgp.conf /etc/exabgp/
    sudo chmod 644 /etc/exabgp/exabgp.conf

    # Install control script
    sudo mkdir -p /opt/lagbuster
    sudo cp /tmp/exabgp-control.py /opt/lagbuster/
    sudo chmod +x /opt/lagbuster/exabgp-control.py

    # Create pipe directory
    sudo mkdir -p /var/run/lagbuster

    echo "ExaBGP configuration deployed"
ENDSSH
echo "✅ ExaBGP config deployed"
echo

# Step 6: Create ExaBGP systemd service
echo "[6/9] Creating ExaBGP systemd service..."
ssh "$ROUTER" 'bash -s' << 'ENDSSH'
    sudo tee /etc/systemd/system/exabgp.service > /dev/null << 'EOF'
[Unit]
Description=ExaBGP - BGP Route Injector
Documentation=https://github.com/Exa-Networks/exabgp
After=network.target
Before=lagbuster.service

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

    sudo systemctl daemon-reload
    echo "ExaBGP systemd service created"
ENDSSH
echo "✅ ExaBGP service created"
echo

# Step 7: Test ExaBGP configuration
echo "[7/9] Testing ExaBGP configuration..."
ssh "$ROUTER" 'exabgp --test /etc/exabgp/exabgp.conf'
echo "✅ ExaBGP config valid"
echo

# Step 8: Stop Bird iBGP on router-jc01 (to avoid conflicts)
echo "[8/9] Preparing Bird to coexist with ExaBGP..."
echo "NOTE: We'll disable Bird's iBGP protocols to let ExaBGP handle them."
read -p "Press Enter to continue or Ctrl+C to abort..."

ssh "$ROUTER" 'bash -s' << 'ENDSSH'
    # Disable Bird iBGP protocols (keep Bird running for BGP session checks)
    # We'll comment out the protocol definitions temporarily
    echo "Disabling Bird iBGP protocols..."

    # Backup current config
    sudo cp /etc/bird/ibgp_edge.conf /etc/bird/ibgp_edge.conf.backup-pre-exabgp

    # Disable the iBGP protocols by commenting them out
    sudo sed -i 's/^protocol bgp JC01_EDGENYC01/#protocol bgp JC01_EDGENYC01/' /etc/bird/ibgp_edge.conf
    sudo sed -i 's/^protocol bgp JC01_ASH01/#protocol bgp JC01_ASH01/' /etc/bird/ibgp_edge.conf

    # Reload Bird config
    sudo birdc configure

    echo "Bird iBGP protocols disabled"
ENDSSH
echo "✅ Bird iBGP disabled"
echo

# Step 9: Start ExaBGP
echo "[9/9] Starting ExaBGP..."
ssh "$ROUTER" 'bash -s' << 'ENDSSH'
    sudo systemctl enable exabgp
    sudo systemctl start exabgp

    # Wait a moment for startup
    sleep 3

    # Check status
    echo "ExaBGP status:"
    sudo systemctl status exabgp --no-pager -l || true

    echo
    echo "Recent ExaBGP logs:"
    sudo journalctl -u exabgp -n 20 --no-pager || true
ENDSSH
echo "✅ ExaBGP started"
echo

echo "==================================="
echo "ExaBGP Deployment Complete!"
echo "==================================="
echo
echo "Next steps:"
echo "1. Run test script: ssh $ROUTER 'sudo /tmp/test-exabgp.sh'"
echo "2. Deploy updated lagbuster binary"
echo "3. Update lagbuster config to enable ExaBGP mode"
echo "4. Restart lagbuster"
echo
echo "To monitor ExaBGP:"
echo "  ssh $ROUTER 'sudo journalctl -u exabgp -f'"
echo
echo "To rollback to Bird:"
echo "  ssh $ROUTER 'sudo systemctl stop exabgp'"
echo "  ssh $ROUTER 'sudo cp /etc/bird/ibgp_edge.conf.backup-pre-exabgp /etc/bird/ibgp_edge.conf'"
echo "  ssh $ROUTER 'sudo birdc configure'"
echo
