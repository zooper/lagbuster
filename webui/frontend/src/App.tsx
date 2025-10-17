import React from 'react';
import { BrowserRouter as Router, Routes, Route, NavLink } from 'react-router-dom';
import { Dashboard } from './pages/Dashboard';
import { Metrics } from './pages/Metrics';
import { Events } from './pages/Events';
import './App.css';

function App() {
  return (
    <Router>
      <div className="app">
        <nav className="app-nav">
          <div className="nav-container">
            <div className="nav-brand">
              <h1>Lagbuster</h1>
              <span className="nav-subtitle">BGP Path Optimizer</span>
            </div>
            <ul className="nav-links">
              <li>
                <NavLink to="/" end>
                  Dashboard
                </NavLink>
              </li>
              <li>
                <NavLink to="/metrics">Metrics</NavLink>
              </li>
              <li>
                <NavLink to="/events">Events</NavLink>
              </li>
            </ul>
          </div>
        </nav>

        <main className="app-main">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/metrics" element={<Metrics />} />
            <Route path="/events" element={<Events />} />
          </Routes>
        </main>
      </div>
    </Router>
  );
}

export default App;
