#!/usr/bin/env python3
"""
ExaBGP Control Script for Lagbuster
Receives JSON commands via named pipe and controls ExaBGP route announcements
"""

import sys
import json
import os
import time
from typing import Dict, List, Any

# ExaBGP communication happens via stdout/stdin
# Commands from lagbuster come via named pipe

PIPE_PATH = "/var/run/lagbuster/exabgp.pipe"

def log(msg: str):
    """Log to stderr (ExaBGP captures stdout for BGP messages)"""
    sys.stderr.write(f"[exabgp-control] {msg}\n")
    sys.stderr.flush()

def announce_route(prefix: str, nexthop: str, communities: List[str] = None, aspath: List[int] = None):
    """
    Announce a route via ExaBGP

    Args:
        prefix: IPv6 prefix (e.g., "2a0e:97c0:e61::/48")
        nexthop: Next-hop IPv6 address
        communities: List of BGP communities (e.g., ["215855:100:99"])
        aspath: AS-path list (e.g., [215855, 215855, 215855] for prepending)
    """
    if aspath is None:
        aspath = [215855]  # Default: just our ASN

    route = {
        "announce": {
            "ipv6 unicast": {
                nexthop: {
                    prefix: {
                        "as-path": aspath,
                    }
                }
            }
        }
    }

    # Add communities if specified
    if communities:
        route["announce"]["ipv6 unicast"][nexthop][prefix]["community"] = communities

    output = json.dumps(route)
    sys.stdout.write(output + "\n")
    sys.stdout.flush()
    log(f"Announced {prefix} via {nexthop} with AS-path={aspath}, communities={communities}")

def withdraw_route(prefix: str, nexthop: str):
    """Withdraw a route from ExaBGP"""
    route = {
        "withdraw": {
            "ipv6 unicast": {
                nexthop: {
                    prefix: {}
                }
            }
        }
    }

    output = json.dumps(route)
    sys.stdout.write(output + "\n")
    sys.stdout.flush()
    log(f"Withdrew {prefix} via {nexthop}")

def handle_command(cmd: Dict[str, Any]):
    """
    Handle a command from lagbuster

    Command format:
    {
        "action": "announce|withdraw",
        "prefixes": ["2a0e:97c0:e61::/48", "2602:f9ba:a10::/48"],
        "peer": "edgenyc01|ash01",
        "priority": 1|99,
        "nexthop": "2a0e:97c0:e61:ff80::d"
    }
    """
    action = cmd.get("action")
    prefixes = cmd.get("prefixes", [])
    peer = cmd.get("peer")
    priority = cmd.get("priority", 1)
    nexthop = cmd.get("nexthop")

    if not action or not prefixes or not nexthop:
        log(f"Invalid command: {cmd}")
        return

    # Determine communities and AS-path based on priority
    if priority == 1:
        # Healthy: no prepending, no communities
        communities = []
        aspath = [215855]
    elif priority == 99:
        # Unhealthy: 8x prepending
        communities = [[215855, 100, 99]]  # Large community format
        aspath = [215855] * 9  # 9 times = 8x prepending
    else:
        log(f"Unknown priority {priority}, defaulting to healthy")
        communities = []
        aspath = [215855]

    # Process each prefix
    for prefix in prefixes:
        if action == "announce":
            announce_route(prefix, nexthop, communities, aspath)
        elif action == "withdraw":
            withdraw_route(prefix, nexthop)
        else:
            log(f"Unknown action: {action}")

def listen_pipe():
    """Listen for commands from lagbuster via named pipe"""
    # Create pipe if it doesn't exist
    pipe_dir = os.path.dirname(PIPE_PATH)
    if not os.path.exists(pipe_dir):
        os.makedirs(pipe_dir, exist_ok=True)

    if not os.path.exists(PIPE_PATH):
        os.mkfifo(PIPE_PATH)

    log(f"Listening for commands on {PIPE_PATH}")

    while True:
        try:
            # Open pipe for reading (blocks until writer connects)
            with open(PIPE_PATH, 'r') as pipe:
                for line in pipe:
                    line = line.strip()
                    if not line:
                        continue

                    try:
                        cmd = json.loads(line)
                        handle_command(cmd)
                    except json.JSONDecodeError as e:
                        log(f"Invalid JSON: {line} - {e}")
        except Exception as e:
            log(f"Error reading pipe: {e}")
            time.sleep(1)

def main():
    """Main entry point"""
    log("ExaBGP control script started")

    # Start listening for commands from lagbuster
    try:
        listen_pipe()
    except KeyboardInterrupt:
        log("Shutting down")
    except Exception as e:
        log(f"Fatal error: {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
