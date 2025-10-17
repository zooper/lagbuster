import React, { useEffect, useState } from 'react';
import { getEvents, formatTimestamp } from '../api/client';
import { Event, TimeRange } from '../types';
import './EventLog.css';

interface EventLogProps {
  initialRange?: TimeRange;
  maxEvents?: number;
}

export function EventLog({ initialRange = '24h', maxEvents }: EventLogProps) {
  const [range, setRange] = useState<TimeRange>(initialRange);
  const [events, setEvents] = useState<Event[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let mounted = true;

    async function fetchData() {
      setLoading(true);
      setError(null);
      try {
        const response = await getEvents(range);
        if (mounted) {
          const eventList = maxEvents
            ? response.events.slice(0, maxEvents)
            : response.events;
          setEvents(eventList);
        }
      } catch (err) {
        if (mounted) {
          setError(err instanceof Error ? err.message : 'Failed to load events');
        }
      } finally {
        if (mounted) {
          setLoading(false);
        }
      }
    }

    fetchData();
    return () => {
      mounted = false;
    };
  }, [range, maxEvents]);

  function getEventIcon(eventType: string): string {
    switch (eventType) {
      case 'switch':
        return '🔄';
      case 'failback':
        return '↩️';
      case 'unhealthy':
        return '⚠️';
      case 'recovery':
        return '✅';
      case 'health_change':
        return '🔔';
      case 'startup':
        return '🚀';
      case 'shutdown':
        return '🛑';
      default:
        return '📋';
    }
  }

  function getEventClass(eventType: string): string {
    switch (eventType) {
      case 'switch':
      case 'failback':
        return 'event-switch';
      case 'unhealthy':
        return 'event-unhealthy';
      case 'recovery':
        return 'event-recovery';
      case 'startup':
        return 'event-startup';
      case 'shutdown':
        return 'event-shutdown';
      default:
        return '';
    }
  }

  function formatEventDescription(event: Event): string {
    switch (event.event_type) {
      case 'switch':
        return `Primary switched from ${event.old_primary} to ${event.new_primary}`;
      case 'failback':
        return `Failback to preferred primary ${event.new_primary}`;
      case 'unhealthy':
        return `Peer ${event.peer_name} became unhealthy`;
      case 'recovery':
        return `Peer ${event.peer_name} recovered`;
      case 'health_change':
        const status = event.new_health ? 'healthy' : 'unhealthy';
        return `Peer ${event.peer_name} became ${status}`;
      case 'startup':
        return 'Lagbuster started';
      case 'shutdown':
        return 'Lagbuster stopped';
      default:
        return event.event_type;
    }
  }

  return (
    <div className="event-log">
      <div className="event-log-header">
        <h2>Event Log</h2>
        <div className="range-selector">
          {(['1h', '24h', '7d', '30d'] as TimeRange[]).map((r) => (
            <button
              key={r}
              className={range === r ? 'active' : ''}
              onClick={() => setRange(r)}
            >
              {r}
            </button>
          ))}
        </div>
      </div>

      <div className="event-list">
        {loading && <div className="loading">Loading events...</div>}
        {error && <div className="error">Error: {error}</div>}
        {!loading && !error && events.length === 0 && (
          <div className="no-events">No events in this time range</div>
        )}
        {!loading &&
          !error &&
          events.map((event) => (
            <div key={event.id} className={`event-item ${getEventClass(event.event_type)}`}>
              <div className="event-icon">{getEventIcon(event.event_type)}</div>
              <div className="event-content">
                <div className="event-description">
                  {formatEventDescription(event)}
                </div>
                {event.reason && <div className="event-reason">{event.reason}</div>}
                <div className="event-timestamp">{formatTimestamp(event.timestamp)}</div>
              </div>
            </div>
          ))}
      </div>
    </div>
  );
}
