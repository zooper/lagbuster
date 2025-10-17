# Lagbuster Web UI & Notifications - Implementation Guide

## Status: Foundation Complete ✓

**Completed:**
- ✅ Database package with SQLite persistence (`database/`)
- ✅ Notifications package with Email, Slack, Telegram support (`notifications/`)
- ✅ Database schema for measurements, events, notifications

**Remaining Work:**
- API server with REST endpoints and WebSocket
- Integration into lagbuster.go core
- React web UI application
- Express backend proxy
- Configuration updates
- Testing and documentation

## Architecture Overview

This implementation adds a complete monitoring and notification system to Lagbuster:

```
┌─────────────────────────────────────────────────────────────┐
│                      Lagbuster Core (Go)                      │
│  ┌──────────────┐  ┌───────────┐  ┌──────────────────────┐ │
│  │ BGP Monitor  │──│  SQLite   │──│  Notifications       │ │
│  │ & Decision   │  │  Database │  │  (Email/Slack/       │ │
│  │ Logic        │  └───────────┘  │   Telegram)          │ │
│  └──────────────┘                 └──────────────────────┘ │
│         │                                    │               │
│         │           ┌────────────┐          │               │
│         └───────────│ HTTP API   │──────────┘               │
│                     │ + WebSocket│                          │
│                     └─────┬──────┘                          │
└───────────────────────────┼─────────────────────────────────┘
                            │ :8080
                ┌───────────┴──────────┐
                │   Express Proxy      │
                │   (Node.js)          │
                └──────────┬───────────┘
                           │ :3000
                ┌──────────┴───────────┐
                │   React Web UI       │
                │  - Dashboard         │
                │  - Latency Graphs    │
                │  - Events Log        │
                │  - Config Editor     │
                └──────────────────────┘
```

## Database Schema (COMPLETED)

Located in `database/schema.sql`:

### Tables:
1. **measurements** - Per-peer latency measurements every 10s
2. **events** - Switch events, health changes, failbacks
3. **notifications** - Log of sent notifications
4. **notification_channels** - Configuration for notification channels

### Key Functions (database/db.go):
- `RecordMeasurement(peerName, latency, isHealthy, isPrimary)`
- `RecordEvent(eventType, ...)`
- `GetMeasurements(peerName, since)` - Query historical latency data
- `GetEvents(since, eventTypes)` - Query event log
- `CleanupOldData(retentionDays)` - Prune old data

## Notifications System (COMPLETED)

Located in `notifications/`:

### Package Structure:
- `notifier.go` - Core notifier with rate limiting
- `email.go` - SMTP email notifications
- `slack.go` - Slack webhook notifications
- `telegram.go` - Telegram bot notifications

### Event Types:
```go
const (
    EventSwitch     = "switch"      // Primary peer switch
    EventUnhealthy  = "unhealthy"   // Peer became unhealthy
    EventRecovery   = "recovery"    // Peer recovered
    EventFailback   = "failback"    // Failback to preferred primary
    EventStartup    = "startup"     // Service started
    EventShutdown   = "shutdown"    // Service stopped
)
```

### Usage Example:
```go
notifier := notifications.NewNotifier(channels, rateLimitMins, logger)

// On primary switch
notifier.Notify(notifications.Event{
    Type:       notifications.EventSwitch,
    OldPrimary: "nyc02",
    NewPrimary: "nyc01",
    Reason:     "current primary degraded",
    Timestamp:  time.Now(),
})
```

## API Endpoints (TO IMPLEMENT)

Create `api/` directory with:

### REST Endpoints:
```
GET  /api/status                 # Current state snapshot
GET  /api/peers                  # All peer status
GET  /api/metrics?peer=X&range=1h # Historical latency
GET  /api/events?range=24h       # Event log
GET  /api/config                 # Current configuration
POST /api/config/notifications   # Update notification config
```

### WebSocket:
```
WS /ws                           # Real-time updates
    -> Sends updates every 10s with current peer status
    -> Sends event notifications immediately
```

### Example API Implementation (api/server.go):
```go
package api

import (
    "encoding/json"
    "net/http"
    "github.com/gorilla/mux"
    "github.com/gorilla/websocket"
)

type Server struct {
    state      *AppState
    db         *database.DB
    router     *mux.Router
    upgrader   websocket.Upgrader
}

func NewServer(state *AppState, db *database.DB) *Server {
    s := &Server{
        state:  state,
        db:     db,
        router: mux.NewRouter(),
        upgrader: websocket.Upgrader{
            CheckOrigin: func(r *http.Request) bool { return true },
        },
    }

    s.setupRoutes()
    return s
}

func (s *Server) setupRoutes() {
    s.router.HandleFunc("/api/status", s.handleStatus).Methods("GET")
    s.router.HandleFunc("/api/peers", s.handlePeers).Methods("GET")
    s.router.HandleFunc("/api/metrics", s.handleMetrics).Methods("GET")
    s.router.HandleFunc("/api/events", s.handleEvents).Methods("GET")
    s.router.HandleFunc("/ws", s.handleWebSocket)
}

func (s *Server) Start(addr string) error {
    return http.ListenAndServe(addr, s.router)
}
```

## Configuration Updates (TO IMPLEMENT)

Add to `lagbuster.go` Config struct:

```go
type Config struct {
    // ... existing fields ...
    API          APIConfig          `yaml:"api"`
    Database     DatabaseConfig     `yaml:"database"`
    Notifications NotificationsConfig `yaml:"notifications"`
}

type APIConfig struct {
    Enabled       bool   `yaml:"enabled"`
    ListenAddress string `yaml:"listen_address"`
}

type DatabaseConfig struct {
    Path          string `yaml:"path"`
    RetentionDays int    `yaml:"retention_days"`
}

type NotificationsConfig struct {
    Enabled         bool              `yaml:"enabled"`
    RateLimitMins   int               `yaml:"rate_limit_minutes"`
    Email           EmailConfig       `yaml:"email"`
    Slack           SlackConfig       `yaml:"slack"`
    Telegram        TelegramConfig    `yaml:"telegram"`
}
```

### Example config.yaml addition:
```yaml
api:
  enabled: true
  listen_address: "0.0.0.0:8080"

database:
  path: "/var/lib/lagbuster/lagbuster.db"
  retention_days: 30

notifications:
  enabled: true
  rate_limit_minutes: 5
  email:
    enabled: true
    smtp_host: "smtp.gmail.com"
    smtp_port: 587
    username: "your-email@gmail.com"
    password: "your-app-password"
    from: "lagbuster@example.com"
    to: ["ops@example.com"]
    events: ["switch", "unhealthy"]
  slack:
    enabled: true
    webhook_url: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
    events: ["switch", "failback", "unhealthy"]
  telegram:
    enabled: true
    bot_token: "YOUR_BOT_TOKEN"
    chat_id: "YOUR_CHAT_ID"
    events: ["switch", "unhealthy", "recovery"]
```

## Core Integration (TO IMPLEMENT)

Modify `lagbuster.go` main():

```go
func main() {
    // ... existing config loading ...

    // Initialize database
    var db *database.DB
    if config.Database.Path != "" {
        var err error
        db, err = database.Open(config.Database.Path)
        if err != nil {
            log.Fatalf("Failed to open database: %v", err)
        }
        defer db.Close()
    }

    // Initialize notifications
    var notifier *notifications.Notifier
    if config.Notifications.Enabled {
        channels := buildNotificationChannels(config.Notifications, logger)
        notifier = notifications.NewNotifier(channels, config.Notifications.RateLimitMins, logger)
    }

    // Initialize API server
    if config.API.Enabled {
        apiServer := api.NewServer(state, db)
        go apiServer.Start(config.API.ListenAddress)
        logger.Info("API server listening on %s", config.API.ListenAddress)
    }

    // ... existing monitoring loop ...
}
```

Add to `runMonitoringCycle()`:

```go
func runMonitoringCycle(state *AppState, db *database.DB, notifier *notifications.Notifier) {
    // ... existing measurement code ...

    // Record measurements to database
    if db != nil {
        for _, peer := range state.Peers {
            db.RecordMeasurement(peer.Config.Name, peer.CurrentLatency, peer.IsHealthy, peer.IsPrimary)
        }
    }

    // ... existing health evaluation ...

    // Record health change events
    if db != nil && wasHealthy != peer.IsHealthy {
        db.RecordEvent("health_change", &peerName, nil, nil, &wasHealthy, &peer.IsHealthy, reason, nil)
    }

    // Send notifications for health changes
    if notifier != nil && wasHealthy && !peer.IsHealthy {
        notifier.Notify(notifications.Event{
            Type:      notifications.EventUnhealthy,
            PeerName:  peer.Config.Name,
            Latency:   peer.CurrentLatency,
            Baseline:  peer.Config.ExpectedBaseline,
            Reason:    reason,
            Timestamp: time.Now(),
        })
    }
}
```

## Web UI Implementation (TO IMPLEMENT)

### Directory Structure:
```
webui/
├── backend/
│   ├── package.json
│   └── server.js          # Express proxy server
└── frontend/
    ├── package.json
    ├── public/
    └── src/
        ├── App.tsx
        ├── components/
        │   ├── PeerCard.tsx
        │   ├── LatencyGraph.tsx
        │   ├── EventLog.tsx
        │   └── NotificationConfig.tsx
        ├── pages/
        │   ├── Dashboard.tsx
        │   ├── Events.tsx
        │   └── Settings.tsx
        └── api/
            └── client.ts
```

### Backend (Express Proxy):

`webui/backend/server.js`:
```javascript
const express = require('express');
const { createProxyMiddleware } = require('http-proxy-middleware');

const app = express();

// Proxy API requests to lagbuster
app.use('/api', createProxyMiddleware({
  target: 'http://localhost:8080',
  changeOrigin: true
}));

// Proxy WebSocket
app.use('/ws', createProxyMiddleware({
  target: 'http://localhost:8080',
  ws: true
}));

// Serve React app
app.use(express.static('../frontend/build'));

app.listen(3000, () => {
  console.log('Web UI running on http://localhost:3000');
});
```

### Frontend (React):

`webui/frontend/src/api/client.ts`:
```typescript
const API_BASE = '';

export async function getStatus() {
  const res = await fetch(`${API_BASE}/api/status`);
  return res.json();
}

export async function getPeers() {
  const res = await fetch(`${API_BASE}/api/peers`);
  return res.json();
}

export async function getMetrics(peer: string, range: string) {
  const res = await fetch(`${API_BASE}/api/metrics?peer=${peer}&range=${range}`);
  return res.json();
}

export async function getEvents(range: string) {
  const res = await fetch(`${API_BASE}/api/events?range=${range}`);
  return res.json();
}

export function connectWebSocket(onMessage: (data: any) => void) {
  const ws = new WebSocket(`ws://${window.location.host}/ws`);
  ws.onmessage = (event) => onMessage(JSON.parse(event.data));
  return ws;
}
```

`webui/frontend/src/components/PeerCard.tsx`:
```typescript
interface PeerCardProps {
  peer: {
    name: string;
    latency: number;
    baseline: number;
    isHealthy: boolean;
    isPrimary: boolean;
    consecutiveHealthy: number;
    consecutiveUnhealthy: number;
  };
}

export function PeerCard({ peer }: PeerCardProps) {
  const healthColor = peer.isHealthy ? 'green' : 'red';
  const degradation = peer.latency - peer.baseline;

  return (
    <div className={`peer-card ${peer.isPrimary ? 'primary' : ''}`}>
      <h3>{peer.name} {peer.isPrimary && '⭐'}</h3>
      <div className="latency">
        <span className={`value ${healthColor}`}>{peer.latency.toFixed(2)}ms</span>
        <span className="baseline">baseline: {peer.baseline}ms</span>
      </div>
      <div className="degradation">
        Degradation: {degradation > 0 ? '+' : ''}{degradation.toFixed(2)}ms
      </div>
      <div className="health-counters">
        {peer.isHealthy ?
          `✓ Healthy ${peer.consecutiveHealthy}x` :
          `✗ Unhealthy ${peer.consecutiveUnhealthy}x`
        }
      </div>
    </div>
  );
}
```

## Deployment

### Development:
```bash
# Terminal 1: Run lagbuster
cd /opt/lagbuster
./lagbuster -config config.yaml

# Terminal 2: Run backend
cd webui/backend
npm install
node server.js

# Terminal 3: Run frontend
cd webui/frontend
npm install
npm start
```

### Production:
```bash
# Build frontend
cd webui/frontend
npm run build

# The backend will serve the built files
cd webui/backend
npm start
```

### systemd Services:

`/etc/systemd/system/lagbuster-webui.service`:
```ini
[Unit]
Description=Lagbuster Web UI
After=network.target lagbuster.service

[Service]
Type=simple
User=lagbuster
WorkingDirectory=/opt/lagbuster/webui/backend
ExecStart=/usr/bin/node server.js
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Next Steps

1. **Implement API package** - Create `api/server.go`, `api/handlers.go`, `api/websocket.go`
2. **Integrate into core** - Modify lagbuster.go to use database and notifications
3. **Create web UI** - Build React app with the components outlined above
4. **Test end-to-end** - Verify notifications work, graphs render correctly
5. **Update documentation** - Add web UI usage to README

## Dependencies to Add

`go.mod`:
```
github.com/mattn/go-sqlite3
github.com/gorilla/mux
github.com/gorilla/websocket
```

`webui/backend/package.json`:
```json
{
  "dependencies": {
    "express": "^4.18.0",
    "http-proxy-middleware": "^2.0.0"
  }
}
```

`webui/frontend/package.json`:
```json
{
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "chart.js": "^4.0.0",
    "react-chartjs-2": "^5.0.0",
    "tailwindcss": "^3.3.0"
  }
}
```
