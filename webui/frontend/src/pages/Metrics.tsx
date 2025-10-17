import React, { useEffect, useState } from 'react';
import { getStatus } from '../api/client';
import { LatencyGraph } from '../components/LatencyGraph';
import './Metrics.css';

export function Metrics() {
  const [peerNames, setPeerNames] = useState<string[]>([]);
  const [selectedPeer, setSelectedPeer] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function fetchPeers() {
      try {
        const status = await getStatus();
        const names = Object.keys(status.peers);
        setPeerNames(names);
        if (names.length > 0) {
          setSelectedPeer(names[0]);
        }
      } catch (err) {
        console.error('Failed to load peers:', err);
      } finally {
        setLoading(false);
      }
    }

    fetchPeers();
  }, []);

  if (loading) {
    return (
      <div className="metrics loading-container">
        <div className="loading">Loading metrics...</div>
      </div>
    );
  }

  return (
    <div className="metrics">
      <header className="metrics-header">
        <h1>Latency Metrics</h1>
      </header>

      <div className="peer-selector">
        <label>Select Peer:</label>
        <select
          value={selectedPeer || ''}
          onChange={(e) => setSelectedPeer(e.target.value)}
        >
          {peerNames.map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </select>
      </div>

      {selectedPeer && (
        <div className="graph-container">
          <LatencyGraph peerName={selectedPeer} />
        </div>
      )}

      {!selectedPeer && peerNames.length === 0 && (
        <div className="no-peers">No peers configured</div>
      )}
    </div>
  );
}
