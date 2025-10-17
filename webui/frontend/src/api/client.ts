import {
  StatusResponse,
  MetricsResponse,
  EventsResponse,
  WebSocketMessage,
  TimeRange,
} from '../types';

const API_BASE = '';

export async function getStatus(): Promise<StatusResponse> {
  const res = await fetch(`${API_BASE}/api/status`);
  if (!res.ok) {
    throw new Error(`Failed to fetch status: ${res.statusText}`);
  }
  return res.json();
}

export async function getPeers(): Promise<StatusResponse['peers']> {
  const res = await fetch(`${API_BASE}/api/peers`);
  if (!res.ok) {
    throw new Error(`Failed to fetch peers: ${res.statusText}`);
  }
  return res.json();
}

export async function getMetrics(
  peer: string,
  range: TimeRange
): Promise<MetricsResponse> {
  const res = await fetch(
    `${API_BASE}/api/metrics?peer=${encodeURIComponent(peer)}&range=${range}`
  );
  if (!res.ok) {
    throw new Error(`Failed to fetch metrics: ${res.statusText}`);
  }
  return res.json();
}

export async function getEvents(range: TimeRange): Promise<EventsResponse> {
  const res = await fetch(`${API_BASE}/api/events?range=${range}`);
  if (!res.ok) {
    throw new Error(`Failed to fetch events: ${res.statusText}`);
  }
  return res.json();
}

export function connectWebSocket(
  onMessage: (data: WebSocketMessage) => void,
  onError?: (error: Event) => void,
  onClose?: () => void
): WebSocket {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const ws = new WebSocket(`${protocol}//${window.location.host}/ws`);

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      onMessage(data);
    } catch (err) {
      console.error('Failed to parse WebSocket message:', err);
    }
  };

  ws.onerror = (error) => {
    console.error('WebSocket error:', error);
    if (onError) onError(error);
  };

  ws.onclose = () => {
    console.log('WebSocket disconnected');
    if (onClose) onClose();
  };

  return ws;
}

export function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);

  const parts = [];
  if (days > 0) parts.push(`${days}d`);
  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  if (secs > 0 || parts.length === 0) parts.push(`${secs}s`);

  return parts.join(' ');
}

export function formatTimestamp(timestamp: string): string {
  const date = new Date(timestamp);
  return date.toLocaleString();
}

export interface NotificationSettings {
  enabled: boolean;
  rate_limit_minutes: number;
  email: {
    enabled: boolean;
    smtp_host: string;
    smtp_port: number;
    from: string;
    to: string[];
    event_types: string[];
  };
  slack: {
    enabled: boolean;
    webhook_url: string;
    event_types: string[];
  };
  telegram: {
    enabled: boolean;
    bot_token: string;
    chat_id: string;
    event_types: string[];
  };
}

export async function getNotificationSettings(): Promise<NotificationSettings> {
  const res = await fetch(`${API_BASE}/api/settings/notifications`);
  if (!res.ok) {
    throw new Error(`Failed to fetch notification settings: ${res.statusText}`);
  }
  return res.json();
}

export async function updateNotificationSettings(
  settings: NotificationSettings
): Promise<void> {
  const res = await fetch(`${API_BASE}/api/settings/notifications`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(settings),
  });
  if (!res.ok) {
    throw new Error(`Failed to update notification settings: ${res.statusText}`);
  }
}

export async function testNotification(channel: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/settings/notifications/test`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ channel }),
  });
  if (!res.ok) {
    throw new Error(`Failed to send test notification: ${res.statusText}`);
  }
}
