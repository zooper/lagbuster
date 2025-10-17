package database

import (
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

// DB wraps the SQLite database connection
type DB struct {
	conn *sql.DB
}

// Measurement represents a peer latency measurement
type Measurement struct {
	ID        int64
	Timestamp time.Time
	PeerName  string
	Latency   float64
	IsHealthy bool
	IsPrimary bool
}

// Event represents a system event
type Event struct {
	ID         int64
	Timestamp  time.Time
	EventType  string
	PeerName   *string
	OldPrimary *string
	NewPrimary *string
	OldHealth  *bool
	NewHealth  *bool
	Reason     string
	Metadata   *string
}

// Notification represents a sent notification
type Notification struct {
	ID          int64
	Timestamp   time.Time
	ChannelType string
	EventID     *int64
	Status      string
	Message     string
	Error       *string
}

// Open opens or creates the database at the given path
func Open(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}

	// Create tables
	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.conn.Close()
}

// RecordMeasurement records a peer latency measurement
func (db *DB) RecordMeasurement(peerName string, latency float64, isHealthy, isPrimary bool) error {
	query := `INSERT INTO measurements (peer_name, latency, is_healthy, is_primary)
	          VALUES (?, ?, ?, ?)`
	_, err := db.conn.Exec(query, peerName, latency, isHealthy, isPrimary)
	if err != nil {
		return fmt.Errorf("recording measurement: %w", err)
	}
	return nil
}

// RecordEvent records a system event
func (db *DB) RecordEvent(eventType string, peerName, oldPrimary, newPrimary *string,
	oldHealth, newHealth *bool, reason string, metadata *string) (int64, error) {

	query := `INSERT INTO events (event_type, peer_name, old_primary, new_primary,
	                              old_health, new_health, reason, metadata)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := db.conn.Exec(query, eventType, peerName, oldPrimary, newPrimary,
		oldHealth, newHealth, reason, metadata)
	if err != nil {
		return 0, fmt.Errorf("recording event: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting event ID: %w", err)
	}

	return id, nil
}

// RecordNotification records a notification attempt
func (db *DB) RecordNotification(channelType string, eventID *int64, status, message string, errorMsg *string) error {
	query := `INSERT INTO notifications (channel_type, event_id, status, message, error)
	          VALUES (?, ?, ?, ?, ?)`
	_, err := db.conn.Exec(query, channelType, eventID, status, message, errorMsg)
	if err != nil {
		return fmt.Errorf("recording notification: %w", err)
	}
	return nil
}

// GetMeasurements retrieves measurements for a peer within a time range
func (db *DB) GetMeasurements(peerName string, since time.Time) ([]Measurement, error) {
	query := `SELECT id, timestamp, peer_name, latency, is_healthy, is_primary
	          FROM measurements
	          WHERE peer_name = ? AND timestamp >= ?
	          ORDER BY timestamp ASC`

	rows, err := db.conn.Query(query, peerName, since)
	if err != nil {
		return nil, fmt.Errorf("querying measurements: %w", err)
	}
	defer rows.Close()

	var measurements []Measurement
	for rows.Next() {
		var m Measurement
		if err := rows.Scan(&m.ID, &m.Timestamp, &m.PeerName, &m.Latency, &m.IsHealthy, &m.IsPrimary); err != nil {
			return nil, fmt.Errorf("scanning measurement: %w", err)
		}
		measurements = append(measurements, m)
	}

	return measurements, rows.Err()
}

// GetEvents retrieves events within a time range
func (db *DB) GetEvents(since time.Time, eventTypes []string) ([]Event, error) {
	query := `SELECT id, timestamp, event_type, peer_name, old_primary, new_primary,
	                 old_health, new_health, reason, metadata
	          FROM events
	          WHERE timestamp >= ?`

	args := []interface{}{since}

	if len(eventTypes) > 0 {
		query += ` AND event_type IN (`
		for i := range eventTypes {
			if i > 0 {
				query += ","
			}
			query += "?"
			args = append(args, eventTypes[i])
		}
		query += `)`
	}

	query += ` ORDER BY timestamp DESC`

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.EventType, &e.PeerName,
			&e.OldPrimary, &e.NewPrimary, &e.OldHealth, &e.NewHealth,
			&e.Reason, &e.Metadata); err != nil {
			return nil, fmt.Errorf("scanning event: %w", err)
		}
		events = append(events, e)
	}

	return events, rows.Err()
}

// CleanupOldData removes data older than the retention period
func (db *DB) CleanupOldData(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	// Delete old measurements
	if _, err := db.conn.Exec("DELETE FROM measurements WHERE timestamp < ?", cutoff); err != nil {
		return fmt.Errorf("cleaning measurements: %w", err)
	}

	// Delete old events (keep all notifications for audit trail)
	if _, err := db.conn.Exec("DELETE FROM events WHERE timestamp < ?", cutoff); err != nil {
		return fmt.Errorf("cleaning events: %w", err)
	}

	// Vacuum to reclaim space
	if _, err := db.conn.Exec("VACUUM"); err != nil {
		return fmt.Errorf("vacuuming database: %w", err)
	}

	return nil
}
