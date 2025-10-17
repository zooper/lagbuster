import React, { useEffect, useState } from 'react';
import { getStatus, connectWebSocket, formatUptime } from '../api/client';
import { StatusResponse, WebSocketMessage } from '../types';
import { PeerCard } from '../components/PeerCard';
import { EventLog } from '../components/EventLog';
import './Dashboard.css';

export function Dashboard() {
  const [status, setStatus] = useState<StatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [wsConnected, setWsConnected] = useState(false);

  useEffect(() => {
    // Initial fetch
    async function fetchInitialStatus() {
      try {
        const data = await getStatus();
        setStatus(data);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load status');
      } finally {
        setLoading(false);
      }
    }

    fetchInitialStatus();

    // Connect to WebSocket for real-time updates
    const ws = connectWebSocket(
      (message: WebSocketMessage) => {
        if (message.type === 'status_update') {
          setStatus(message.data as StatusResponse);
        }
      },
      () => {
        setWsConnected(false);
      },
      () => {
        setWsConnected(false);
      }
    );

    ws.onopen = () => {
      setWsConnected(true);
    };

    return () => {
      ws.close();
    };
  }, []);

  if (loading) {
    return (
      <div className="dashboard loading-container">
        <div className="loading">Loading dashboard...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="dashboard error-container">
        <div className="error">Error: {error}</div>
      </div>
    );
  }

  if (!status) {
    return null;
  }

  const peerArray = Object.values(status.peers);

  return (
    <div className="dashboard">
      <header className="dashboard-header">
        <div className="header-content">
          <h1>Lagbuster Monitor</h1>
          <div className="connection-status">
            <span className={`status-dot ${wsConnected ? 'connected' : 'disconnected'}`} />
            {wsConnected ? 'Live' : 'Disconnected'}
          </div>
        </div>
        <div className="system-info">
          <div className="info-item">
            <label>Current Primary</label>
            <div className="value primary-name">{status.current_primary}</div>
          </div>
          <div className="info-item">
            <label>Uptime</label>
            <div className="value">{formatUptime(status.uptime_seconds)}</div>
          </div>
          {status.last_switch && (
            <div className="info-item">
              <label>Last Switch</label>
              <div className="value">
                {new Date(status.last_switch).toLocaleString()}
              </div>
            </div>
          )}
        </div>
      </header>

      <section className="peers-section">
        <h2>BGP Peers</h2>
        <div className="peers-grid">
          {peerArray.map((peer) => (
            <PeerCard key={peer.name} peer={peer} />
          ))}
        </div>
      </section>

      <section className="events-section">
        <EventLog initialRange="24h" maxEvents={10} />
      </section>
    </div>
  );
}
