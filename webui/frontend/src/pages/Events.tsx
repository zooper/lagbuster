import React from 'react';
import { EventLog } from '../components/EventLog';
import './Events.css';

export function Events() {
  return (
    <div className="events-page">
      <header className="events-header">
        <h1>Event History</h1>
        <p>View all system events including switches, health changes, and failbacks</p>
      </header>

      <div className="events-container">
        <EventLog initialRange="7d" />
      </div>
    </div>
  );
}
