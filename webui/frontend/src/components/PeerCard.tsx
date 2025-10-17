import React from 'react';
import { PeerStatus } from '../types';
import './PeerCard.css';

interface PeerCardProps {
  peer: PeerStatus;
  measurementInterval: number;
}

// Format duration from consecutive count (assuming 10 second measurement interval)
function formatDuration(consecutiveCount: number, measurementInterval: number = 10): string {
  const totalSeconds = consecutiveCount * measurementInterval;

  if (totalSeconds < 60) {
    return `${totalSeconds} seconds`;
  }

  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;

  if (minutes < 60) {
    if (seconds === 0) {
      return `${minutes} ${minutes === 1 ? 'minute' : 'minutes'}`;
    }
    return `${minutes}m ${seconds}s`;
  }

  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;

  if (remainingMinutes === 0) {
    return `${hours} ${hours === 1 ? 'hour' : 'hours'}`;
  }
  return `${hours}h ${remainingMinutes}m`;
}

export function PeerCard({ peer, measurementInterval }: PeerCardProps) {
  const healthClass = peer.is_healthy ? 'healthy' : 'unhealthy';
  const primaryClass = peer.is_primary ? 'primary' : '';
  const degradationPercent =
    peer.baseline > 0 ? ((peer.degradation / peer.baseline) * 100).toFixed(1) : '0';

  return (
    <div className={`peer-card ${healthClass} ${primaryClass}`}>
      <div className="peer-header">
        <h3>
          {peer.name}
          {peer.is_primary && <span className="primary-badge">⭐ PRIMARY</span>}
        </h3>
        <div className={`health-indicator ${healthClass}`}>
          {peer.is_healthy ? '✓ Healthy' : '✗ Unhealthy'}
        </div>
      </div>

      <div className="peer-hostname">{peer.hostname}</div>

      <div className="peer-metrics">
        <div className="metric">
          <label>Current Latency</label>
          <div className={`value ${healthClass}`}>
            {peer.latency >= 0 ? `${peer.latency.toFixed(2)} ms` : 'Unreachable'}
          </div>
        </div>

        <div className="metric">
          <label>Baseline</label>
          <div className="value">{peer.baseline.toFixed(2)} ms</div>
        </div>

        <div className="metric">
          <label>Degradation</label>
          <div className={`value ${peer.degradation > 0 ? 'warning' : ''}`}>
            {peer.degradation > 0 ? '+' : ''}
            {peer.degradation.toFixed(2)} ms ({degradationPercent}%)
          </div>
        </div>
      </div>

      <div className="peer-counters">
        {peer.is_healthy ? (
          <div className="counter healthy-counter">
            Healthy for: {formatDuration(peer.consecutive_healthy_count, measurementInterval)}
          </div>
        ) : (
          <div className="counter unhealthy-counter">
            Unhealthy for: {formatDuration(peer.consecutive_unhealthy_count, measurementInterval)}
          </div>
        )}
      </div>
    </div>
  );
}
