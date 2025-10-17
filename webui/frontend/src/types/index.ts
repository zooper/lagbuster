export interface PeerStatus {
  name: string;
  hostname: string;
  latency: number;
  baseline: number;
  degradation: number;
  is_healthy: boolean;
  is_primary: boolean;
  consecutive_healthy_count: number;
  consecutive_unhealthy_count: number;
}

export interface StatusResponse {
  current_primary: string;
  uptime_seconds: number;
  last_switch?: string;
  measurement_interval: number;
  peers: { [key: string]: PeerStatus };
}

export interface MetricPoint {
  timestamp: string;
  latency: number;
  is_healthy: boolean;
}

export interface MetricsResponse {
  peer: string;
  range: string;
  points: MetricPoint[];
}

export interface Event {
  id: number;
  timestamp: string;
  event_type: string;
  peer_name?: string;
  old_primary?: string;
  new_primary?: string;
  old_health?: boolean;
  new_health?: boolean;
  reason: string;
}

export interface EventsResponse {
  range: string;
  events: Event[];
}

export interface WebSocketMessage {
  type: 'status_update' | 'event';
  data: StatusResponse | Event;
}

export type TimeRange = '1h' | '24h' | '7d' | '30d';
