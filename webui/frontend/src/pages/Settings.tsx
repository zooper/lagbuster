import React, { useEffect, useState } from 'react';
import { getNotificationSettings, updateNotificationSettings } from '../api/client';
import './Settings.css';

interface NotificationSettings {
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

const EVENT_TYPES = [
  { value: 'switch', label: 'Primary Switch' },
  { value: 'unhealthy', label: 'Peer Unhealthy' },
  { value: 'recovery', label: 'Peer Recovery' },
  { value: 'failback', label: 'Failback' },
  { value: 'startup', label: 'System Startup' },
  { value: 'shutdown', label: 'System Shutdown' },
];

export function Settings() {
  const [settings, setSettings] = useState<NotificationSettings | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [emailInput, setEmailInput] = useState('');

  useEffect(() => {
    loadSettings();
  }, []);

  async function loadSettings() {
    try {
      const data = await getNotificationSettings();
      setSettings(data);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load settings');
    } finally {
      setLoading(false);
    }
  }

  async function handleSave() {
    if (!settings) return;

    setSaving(true);
    setSuccess(false);
    try {
      await updateNotificationSettings(settings);
      setSuccess(true);
      setError(null);
      setTimeout(() => setSuccess(false), 3000);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save settings');
    } finally {
      setSaving(false);
    }
  }

  function toggleEventType(channel: 'email' | 'slack' | 'telegram', eventType: string) {
    if (!settings) return;

    const currentTypes = settings[channel].event_types;
    const newTypes = currentTypes.includes(eventType)
      ? currentTypes.filter(t => t !== eventType)
      : [...currentTypes, eventType];

    setSettings({
      ...settings,
      [channel]: { ...settings[channel], event_types: newTypes }
    });
  }

  function addEmail() {
    if (!settings || !emailInput.trim()) return;

    setSettings({
      ...settings,
      email: {
        ...settings.email,
        to: [...settings.email.to, emailInput.trim()]
      }
    });
    setEmailInput('');
  }

  function removeEmail(email: string) {
    if (!settings) return;

    setSettings({
      ...settings,
      email: {
        ...settings.email,
        to: settings.email.to.filter(e => e !== email)
      }
    });
  }

  if (loading) return <div className="loading">Loading settings...</div>;
  if (error && !settings) return <div className="error">Error: {error}</div>;
  if (!settings) return null;

  return (
    <div className="settings-page">
      <h1>Notification Settings</h1>

      {error && <div className="error-banner">{error}</div>}
      {success && <div className="success-banner">Settings saved successfully!</div>}

      <div className="settings-section">
        <h2>Global Settings</h2>
        <div className="form-group">
          <label>
            <input
              type="checkbox"
              checked={settings.enabled}
              onChange={(e) => setSettings({ ...settings, enabled: e.target.checked })}
            />
            Enable Notifications
          </label>
        </div>
        <div className="form-group">
          <label>
            Rate Limit (minutes between notifications)
            <input
              type="number"
              value={settings.rate_limit_minutes}
              onChange={(e) => setSettings({ ...settings, rate_limit_minutes: parseInt(e.target.value) || 0 })}
              min="0"
            />
          </label>
        </div>
      </div>

      <div className="settings-section">
        <h2>Email Notifications</h2>
        <div className="form-group">
          <label>
            <input
              type="checkbox"
              checked={settings.email.enabled}
              onChange={(e) => setSettings({
                ...settings,
                email: { ...settings.email, enabled: e.target.checked }
              })}
            />
            Enable Email Notifications
          </label>
        </div>

        {settings.email.enabled && (
          <>
            <div className="form-row">
              <div className="form-group">
                <label>
                  SMTP Host
                  <input
                    type="text"
                    value={settings.email.smtp_host}
                    onChange={(e) => setSettings({
                      ...settings,
                      email: { ...settings.email, smtp_host: e.target.value }
                    })}
                  />
                </label>
              </div>
              <div className="form-group">
                <label>
                  SMTP Port
                  <input
                    type="number"
                    value={settings.email.smtp_port}
                    onChange={(e) => setSettings({
                      ...settings,
                      email: { ...settings.email, smtp_port: parseInt(e.target.value) || 587 }
                    })}
                  />
                </label>
              </div>
            </div>

            <div className="form-group">
              <label>
                From Address
                <input
                  type="email"
                  value={settings.email.from}
                  onChange={(e) => setSettings({
                    ...settings,
                    email: { ...settings.email, from: e.target.value }
                  })}
                />
              </label>
            </div>

            <div className="form-group">
              <label>To Addresses</label>
              <div className="email-list">
                {settings.email.to.map(email => (
                  <div key={email} className="email-item">
                    <span>{email}</span>
                    <button onClick={() => removeEmail(email)}>×</button>
                  </div>
                ))}
                <div className="email-input">
                  <input
                    type="email"
                    value={emailInput}
                    onChange={(e) => setEmailInput(e.target.value)}
                    onKeyPress={(e) => e.key === 'Enter' && addEmail()}
                    placeholder="Add email address"
                  />
                  <button onClick={addEmail}>Add</button>
                </div>
              </div>
            </div>

            <div className="form-group">
              <label>Event Types</label>
              <div className="event-types">
                {EVENT_TYPES.map(({ value, label }) => (
                  <label key={value} className="checkbox-label">
                    <input
                      type="checkbox"
                      checked={settings.email.event_types.includes(value)}
                      onChange={() => toggleEventType('email', value)}
                    />
                    {label}
                  </label>
                ))}
              </div>
            </div>
          </>
        )}
      </div>

      <div className="settings-section">
        <h2>Slack Notifications</h2>
        <div className="form-group">
          <label>
            <input
              type="checkbox"
              checked={settings.slack.enabled}
              onChange={(e) => setSettings({
                ...settings,
                slack: { ...settings.slack, enabled: e.target.checked }
              })}
            />
            Enable Slack Notifications
          </label>
        </div>

        {settings.slack.enabled && (
          <>
            <div className="form-group">
              <label>
                Webhook URL
                <input
                  type="text"
                  value={settings.slack.webhook_url}
                  onChange={(e) => setSettings({
                    ...settings,
                    slack: { ...settings.slack, webhook_url: e.target.value }
                  })}
                  placeholder="https://hooks.slack.com/services/..."
                />
              </label>
            </div>

            <div className="form-group">
              <label>Event Types</label>
              <div className="event-types">
                {EVENT_TYPES.map(({ value, label }) => (
                  <label key={value} className="checkbox-label">
                    <input
                      type="checkbox"
                      checked={settings.slack.event_types.includes(value)}
                      onChange={() => toggleEventType('slack', value)}
                    />
                    {label}
                  </label>
                ))}
              </div>
            </div>
          </>
        )}
      </div>

      <div className="settings-section">
        <h2>Telegram Notifications</h2>
        <div className="form-group">
          <label>
            <input
              type="checkbox"
              checked={settings.telegram.enabled}
              onChange={(e) => setSettings({
                ...settings,
                telegram: { ...settings.telegram, enabled: e.target.checked }
              })}
            />
            Enable Telegram Notifications
          </label>
        </div>

        {settings.telegram.enabled && (
          <>
            <div className="form-group">
              <label>
                Bot Token
                <input
                  type="text"
                  value={settings.telegram.bot_token}
                  onChange={(e) => setSettings({
                    ...settings,
                    telegram: { ...settings.telegram, bot_token: e.target.value }
                  })}
                  placeholder="123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
                />
              </label>
            </div>

            <div className="form-group">
              <label>
                Chat ID
                <input
                  type="text"
                  value={settings.telegram.chat_id}
                  onChange={(e) => setSettings({
                    ...settings,
                    telegram: { ...settings.telegram, chat_id: e.target.value }
                  })}
                  placeholder="-1001234567890"
                />
              </label>
            </div>

            <div className="form-group">
              <label>Event Types</label>
              <div className="event-types">
                {EVENT_TYPES.map(({ value, label }) => (
                  <label key={value} className="checkbox-label">
                    <input
                      type="checkbox"
                      checked={settings.telegram.event_types.includes(value)}
                      onChange={() => toggleEventType('telegram', value)}
                    />
                    {label}
                  </label>
                ))}
              </div>
            </div>
          </>
        )}
      </div>

      <div className="settings-actions">
        <button
          className="save-button"
          onClick={handleSave}
          disabled={saving}
        >
          {saving ? 'Saving...' : 'Save Settings'}
        </button>
      </div>
    </div>
  );
}
