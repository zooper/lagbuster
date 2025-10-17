# Lagbuster Web UI

Real-time monitoring dashboard and metrics visualization for Lagbuster BGP path optimizer.

## Features

- **Live Dashboard**: Real-time peer status with WebSocket updates
- **Latency Graphs**: Historical latency metrics with 1h/24h/7d/30d views
- **Event Log**: Complete history of switches, health changes, and failbacks
- **Responsive Design**: Works on desktop, tablet, and mobile

## Architecture

```
webui/
├── backend/         # Express proxy server (Node.js)
│   ├── server.js    # Proxies API requests to lagbuster:8080
│   └── package.json
└── frontend/        # React application (TypeScript)
    ├── src/
    │   ├── api/         # API client for lagbuster backend
    │   ├── components/  # Reusable UI components
    │   ├── pages/       # Route pages
    │   └── types/       # TypeScript type definitions
    └── package.json
```

## Development Setup

### Prerequisites

- Node.js 16+ and npm
- Lagbuster API server running on port 8080

### Quick Start

```bash
# Terminal 1: Run Lagbuster with API enabled
cd /opt/lagbuster
./lagbuster -config config.yaml

# Terminal 2: Install and run backend
cd webui/backend
npm install
npm start
# Backend runs on http://localhost:3000

# Terminal 3: Install and run frontend (development mode)
cd webui/frontend
npm install
npm start
# Frontend dev server runs on http://localhost:3001
```

Access the development UI at http://localhost:3001

## Production Deployment

### Build Frontend

```bash
cd webui/frontend
npm run build
```

The built files will be in `frontend/build/` and served by the Express backend.

### Run Backend

```bash
cd webui/backend
npm start
```

Access the production UI at http://localhost:3000

### systemd Service

Create `/etc/systemd/system/lagbuster-webui.service`:

```ini
[Unit]
Description=Lagbuster Web UI
After=network.target lagbuster.service
Requires=lagbuster.service

[Service]
Type=simple
User=lagbuster
WorkingDirectory=/opt/lagbuster/webui/backend
ExecStart=/usr/bin/node server.js
Restart=on-failure
Environment="LAGBUSTER_API=http://localhost:8080"
Environment="PORT=3000"

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable lagbuster-webui
sudo systemctl start lagbuster-webui
```

## Environment Variables

Backend server supports:

- `PORT`: HTTP server port (default: 3000)
- `LAGBUSTER_API`: Lagbuster API URL (default: http://localhost:8080)

Create `webui/backend/.env`:

```env
PORT=3000
LAGBUSTER_API=http://localhost:8080
```

## Configuration

Ensure your `config.yaml` has the API enabled:

```yaml
api:
  enabled: true
  listen_address: "0.0.0.0:8080"

database:
  path: "/var/lib/lagbuster/lagbuster.db"
  retention_days: 30
```

## Technology Stack

### Backend
- Express.js: Web server
- http-proxy-middleware: Proxy to Lagbuster API

### Frontend
- React 18: UI framework
- TypeScript: Type safety
- React Router: Client-side routing
- Chart.js: Latency graphs
- WebSocket: Real-time updates

## API Endpoints (Proxied)

All requests to `/api/*` and `/ws` are proxied to the Lagbuster backend:

- `GET /api/status` - Current system status
- `GET /api/peers` - All peer statuses
- `GET /api/metrics?peer=X&range=1h` - Historical latency
- `GET /api/events?range=24h` - Event log
- `WS /ws` - WebSocket for real-time updates

## Troubleshooting

### Backend won't start

```bash
# Check if port 3000 is available
lsof -i :3000

# Check if lagbuster API is running
curl http://localhost:8080/api/status
```

### Frontend shows connection errors

- Verify backend is running on port 3000
- Check browser console for errors
- Ensure CORS is enabled in lagbuster API (it is by default)

### WebSocket disconnects

- Check firewall settings
- Verify lagbuster API server is running
- Check browser console for WebSocket errors

## Development

### Adding a new page

1. Create page component in `frontend/src/pages/`
2. Add route in `frontend/src/App.tsx`
3. Add navigation link in `App.tsx`

### Adding a new API endpoint

1. Add handler in lagbuster `api/handlers.go`
2. Register route in `api/server.go`
3. Add TypeScript types in `frontend/src/types/`
4. Add client function in `frontend/src/api/client.ts`

## License

Same as Lagbuster main project.
