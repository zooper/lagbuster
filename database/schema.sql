-- Lagbuster database schema

-- Peer latency measurements
CREATE TABLE IF NOT EXISTS measurements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    peer_name TEXT NOT NULL,
    latency REAL NOT NULL,  -- -1 for timeout/unreachable
    is_healthy BOOLEAN NOT NULL,
    is_primary BOOLEAN NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_measurements_timestamp ON measurements(timestamp);
CREATE INDEX IF NOT EXISTS idx_measurements_peer ON measurements(peer_name, timestamp);

-- System events (switches, health changes, etc.)
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_type TEXT NOT NULL,  -- 'switch', 'health_change', 'failback', 'startup', 'shutdown'
    peer_name TEXT,  -- NULL for system events
    old_primary TEXT,  -- For switch events
    new_primary TEXT,  -- For switch events
    old_health BOOLEAN,  -- For health_change events
    new_health BOOLEAN,  -- For health_change events
    reason TEXT,  -- Human-readable reason
    metadata TEXT  -- JSON for additional data
);

CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type, timestamp);

-- Notification log
CREATE TABLE IF NOT EXISTS notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    channel_type TEXT NOT NULL,  -- 'email', 'slack', 'telegram'
    event_id INTEGER,  -- Reference to event that triggered it
    status TEXT NOT NULL,  -- 'sent', 'failed', 'rate_limited'
    message TEXT,
    error TEXT,  -- Error message if failed
    FOREIGN KEY (event_id) REFERENCES events(id)
);

-- Notification configuration (managed via UI)
CREATE TABLE IF NOT EXISTS notification_channels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_type TEXT NOT NULL UNIQUE,  -- 'email', 'slack', 'telegram'
    enabled BOOLEAN NOT NULL DEFAULT 1,
    config_json TEXT NOT NULL,  -- JSON configuration for the channel
    events_json TEXT NOT NULL,  -- JSON array of event types to notify on
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
