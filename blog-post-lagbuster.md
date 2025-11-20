# Building Lagbuster: Because 3 AM Manual BGP Tweaking Isn't Fun

So here's the thing about running AS215855 with multiple transit providers - BGP is great at keeping you connected, but it doesn't really care if your traffic is taking the scenic route through half of Europe at 200ms latency. As long as the BGP session is up, it's happy. You? Not so much.

## The Problem I Kept Running Into

I've got three edge routers spread across different locations in Europe. Most of the time, everything works great. My primary peer in Amsterdam gives me nice 45ms latency to most destinations. But then... things happen.

Maybe there's congestion somewhere upstream. Maybe someone fat-fingered a route. Maybe the internet is just being the internet. Suddenly my primary path is running at 95ms when it should be 45ms. The BGP session stays up (because why wouldn't it?), traffic keeps flowing through the degraded path, and I'm stuck with two options:

1. Notice it (if I'm even looking), SSH in, manually tweak BGP local preferences, reload Bird, hope I didn't break anything
2. Don't notice it and let things be slow until whatever is broken fixes itself

Neither of these options felt great. Especially at 3 AM when I'm definitely not monitoring my home network.

What I really wanted was something that would just... handle this. Monitor the actual performance of each path, and automatically switch to a better one when the current path goes bad. Without flapping routes every time there's a tiny latency spike.

Turns out, I couldn't find a tool that did exactly what I wanted. So I built one.

## Enter Lagbuster

Lagbuster is pretty simple in concept: it pings your edge routers every 10 seconds, measures the latency, and switches your BGP primary path if the current one is performing poorly. It integrates with Bird routing daemon by updating priority variables and triggering a config reload.

The whole thing is about 600 lines of Go, runs as a systemd service on your router, and just... works.

### The Tricky Part: How Do You Define "Bad"?

This was actually the most interesting part to figure out. My three edge routers have different expected latencies because they're in different places:

- Amsterdam: normally ~45ms
- Frankfurt: normally ~52ms
- London: normally ~55ms

If I just compared absolute latency values, I'd always prefer Amsterdam even when it's degraded. That's not what I want. What I want is to know when each peer is performing worse than *its own normal baseline*.

So that's what Lagbuster does. You tell it "Amsterdam should normally be around 45ms", and then it monitors for degradation from that baseline. If Amsterdam suddenly jumps to 95ms (+50ms degradation), that's bad and we should switch. But if London is at 60ms (+5ms from its baseline), that's totally fine.

This per-peer baseline approach means I can have geographically distributed peers without comparing apples to oranges.

### Preventing Route Flapping (The Hard Part)

Here's what I learned pretty quickly when testing this: if you just switch every time another peer looks slightly better, you're going to have a bad time. Route flapping is real and it will make your network more unstable than just accepting some minor degradation.

So Lagbuster has a bunch of damping logic:

**Comfort threshold**: If my current primary is within 10ms of its baseline, I stay put even if another peer looks slightly better. Stability wins.

**Consecutive unhealthy counter**: I need to see 3 consecutive bad measurements (30 seconds with my config) before switching away. This filters out temporary spikes.

**Cooldown period**: After switching, I won't switch again for at least 3 minutes. This prevents rapid oscillation if two paths are both having issues.

The decision logic basically goes:
1. Is current primary healthy and comfortable? → Stay
2. Has current primary been unhealthy for 3+ measurements? → Switch to best alternative
3. Did I switch recently? → Stay (even if current isn't great)

It's not rocket science, but it works really well in practice.

## How It Actually Works

The monitoring loop is straightforward:
1. Ping all edge routers every 10 seconds
2. Compare each peer's latency to its configured baseline
3. Run decision logic (with all that damping stuff)
4. If we need to switch, update Bird config and reload

### The Bird Integration

This is where it gets interesting. I didn't want Lagbuster to manage my entire Bird configuration - I already have Ansible doing that. I just wanted it to control which peer is primary at any given time.

So Lagbuster writes to a single file: `/etc/bird/lagbuster-priorities.conf`

```bash
define core01_edge01_lagbuster_priority = 1;  # Primary
define core01_edge02_lagbuster_priority = 3;  # Tertiary
define core01_edge03_lagbuster_priority = 2;  # Secondary
```

Then in my Bird import filters, I just use those variables to set BGP local preference:

```
if core01_edge01_lagbuster_priority = 1 then {
    bgp_local_pref = 130;  # Highest = primary
}
```

When Lagbuster decides to switch, it updates the priorities file and runs `birdc configure` to reload Bird. The whole switchover takes less than 2 seconds. Bird doesn't care that priorities changed - it just sees the new values and adjusts route preferences accordingly.

This separation means I can still use Ansible to manage my BGP peer configs, filter logic, and everything else. Lagbuster only touches that one file with the priority values. No conflicts, everything plays nice together.

## What It Looks Like in Production

Here's a real degradation event from my logs:

```
Initial state:
Amsterdam: 45ms (baseline: 45ms) → Primary, healthy
Frankfurt: 52ms (baseline: 52ms) → Secondary, healthy
London: 55ms (baseline: 55ms) → Tertiary, healthy

10 seconds later - something breaks in Amsterdam:
Amsterdam: 95ms (+50ms degradation) → UNHEALTHY
Frankfurt: 53ms (+1ms) → HEALTHY
London: 56ms (+1ms) → HEALTHY

20 seconds later - still bad:
Amsterdam: 98ms → UNHEALTHY (2 consecutive)

30 seconds later - yep, definitely broken:
Amsterdam: 92ms → UNHEALTHY (3 consecutive)

[INFO] SWITCHING PRIMARY: Amsterdam -> Frankfurt
[INFO] Reason: current primary degraded (47ms above baseline)
[INFO] Bird configuration updated successfully
```

Within 30 seconds of sustained degradation, traffic automatically moves to Frankfurt. No alerts, no manual intervention, just works.

And here's the cool part - later when Amsterdam recovers, Lagbuster has an automatic failback feature. After Amsterdam has been healthy for 30 minutes straight (configurable), it'll automatically switch back. This is perfect because Amsterdam is my preferred peer for cost/performance reasons, but I don't want to manually monitor when it's safe to switch back.

## Configuration

The config file is pretty straightforward. Here's what mine looks like:

```yaml
peers:
  - name: amsterdam
    hostname: edge01.example.com
    expected_baseline: 45.0  # What's "normal" for this peer
    bird_variable: core01_edge01_lagbuster_priority

  - name: frankfurt
    hostname: edge02.example.com
    expected_baseline: 52.0
    bird_variable: core01_edge02_lagbuster_priority

thresholds:
  degradation_threshold: 20.0  # Switch if > baseline + 20ms
  comfort_threshold: 10.0      # Stay if < baseline + 10ms
  absolute_max_latency: 150.0  # Hard limit regardless of baseline

damping:
  consecutive_unhealthy_count: 3  # Need 3 bad measurements to switch
  measurement_interval: 10        # Ping every 10 seconds
  cooldown_period: 180           # Wait 3min between switches

failback:
  enabled: true
  consecutive_healthy_count: 180  # Need 30min healthy to failback

startup:
  initial_primary: amsterdam      # Start here
  preferred_primary: amsterdam    # Always try to come back here

logging:
  level: info
  log_decisions: true  # Show why we made each decision
```

The only tricky part is figuring out your baselines. Just run some manual pings to each edge router when things are working normally and use those values. For me, Amsterdam is usually ~45ms, Frankfurt ~52ms, etc.

## Running It

It's just a systemd service:

```bash
sudo systemctl enable lagbuster
sudo systemctl start lagbuster
sudo journalctl -u lagbuster -f
```

That's it. It sits there, pings your edges every 10 seconds, and switches when needed.

The logging is pretty detailed - every decision includes the full rationale:

```
[INFO] Peer amsterdam became UNHEALTHY: latency=95.23ms, baseline=45.00ms, degradation=50.23ms
[INFO] Best peer selection: frankfurt (latency=52.11ms, healthy=true)
[INFO] SWITCHING PRIMARY: amsterdam -> frankfurt | Reason: current primary degraded
[INFO] Bird configuration updated successfully
```

I like being able to see exactly why it made each decision. Makes it a lot easier to tune the thresholds if needed.

## Things I Learned Building This

**Static baselines are important.** I initially tried adaptive baselines that would adjust over time, but that creates a "boiling frog" problem where slow degradation gets normalized. Static baselines force you to notice when performance degrades from your known-good state.

**Damping is REALLY important.** Route flapping will make your network worse than just accepting some degradation. The consecutive unhealthy counter and cooldown period are not optional - they're what make this stable in production.

**Go was the right choice.** Single binary, no dependencies, runs on any Linux router or even macOS for testing. The whole thing is one file (~600 lines), which makes it easy to understand and modify.

**Integration matters more than features.** I could have built a web dashboard, Prometheus metrics, fancy alerting, etc. But what I actually needed was something that plays nice with my existing Bird + Ansible setup. Keeping it simple and focused on that one integration point was the right call.

## Get It

Lagbuster is open source (MIT license) on GitHub: [github.com/zooper/lagbuster](https://github.com/zooper/lagbuster)

The repo has everything you need:
- Source code (it's just one Go file)
- Example config with comments
- Setup guide for integrating with Bird
- Ansible integration examples if you use that
- Systemd service file

If you're running a multi-homed network and tired of manually managing BGP path selection, give it a shot. It's been running on my network for a while now and I honestly forget it's there most of the time - which is exactly what I wanted.

## What's Next

I've got a few ideas for future enhancements:

- Packet loss monitoring (currently only does latency)
- Prometheus metrics export so I can graph this stuff
- Maybe a simple web dashboard to see current state
- Slack/email alerts when it switches paths
- Historical data in SQLite

But honestly, it works well enough right now that these are all "nice to have" rather than "need to have". The core functionality - automatic failover based on real performance - is solid.

If you try it out or have ideas for improvements, let me know! Issues and PRs welcome on GitHub.

---

*Jon Jonsson - AS215855*
