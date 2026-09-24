import React, { useState, useEffect } from 'react';
import {
  Lock,
  Check,
  TriangleAlert,
  Server,
} from 'lucide-react';
import { api } from '@/lib/api';
import { ServerSettings } from '@/types/api';
import { Button } from '@/components/Button';
import { Input } from '@/components/Input';
import { Banner } from '@/components/Banner';

export const SettingsPage: React.FC = () => {
  const [settings, setSettings] = useState<ServerSettings | null>(null);
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    api.getSettings().then(setSettings).catch(console.error);
  }, []);

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!currentPassword || !newPassword) return;
    if (newPassword !== confirmPassword) {
      setFeedback({ type: 'error', message: 'New passwords do not match' });
      return;
    }
    if (newPassword.length < 8) {
      setFeedback({ type: 'error', message: 'New password must be at least 8 characters' });
      return;
    }

    try {
      setLoading(true);
      await api.changePassword(currentPassword, newPassword);
      setFeedback({ type: 'success', message: 'Password updated.' });
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      const updated = await api.getSettings();
      setSettings(updated);
    } catch (err: any) {
      setFeedback({ type: 'error', message: err.message || 'Failed to update password' });
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6 max-w-2xl">
      {/* Header */}
      <div>
        <h2 className="text-xl font-semibold tracking-tight text-foreground">Settings</h2>
        <p className="text-xs text-muted-foreground mt-0.5">
          Dashboard security and runtime information
        </p>
      </div>

      {/* Default password warning */}
      {settings?.is_default_password && (
        <div className="p-4 bg-warning/10 border border-warning/30 rounded-lg text-xs text-warning flex items-start gap-3">
          <TriangleAlert className="size-4 shrink-0 mt-0.5" />
          <div className="space-y-1">
            <h3 className="font-semibold">Default password active</h3>
            <p className="text-warning/90 leading-relaxed">
              The dashboard still uses the default password ({' '}
              <code className="font-mono bg-muted px-1 py-0.5 rounded border border-border text-foreground">123456</code>
              ). Change it below to secure gateway administration.
            </p>
          </div>
        </div>
      )}

      {/* Feedback */}
      {feedback && (
        <Banner variant={feedback.type} onDismiss={() => setFeedback(null)}>
          {feedback.message}
        </Banner>
      )}

      {/* Password card */}
      <div className="bg-card border border-border rounded-lg overflow-hidden">
        <div className="border-b border-border p-5">
          <h3 className="text-sm font-semibold flex items-center gap-2 text-foreground">
            <Lock className="size-4 text-muted-foreground" />
            Dashboard password
          </h3>
          <p className="text-xs text-muted-foreground mt-1">
            Stored in{' '}
            <code className="px-1.5 py-0.5 bg-muted rounded border border-border text-foreground font-mono text-[11px]">
              ~/.tinyroute/dashboard.json
            </code>
          </p>
        </div>

        <form onSubmit={handleChangePassword} className="p-6 space-y-4">
          <Input
            label="Current password"
            type="password"
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            placeholder="••••••••"
            required
          />

          <Input
            label="New password"
            type="password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            placeholder="••••••••"
            minLength={8}
            required
          />

          <Input
            label="Confirm new password"
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            placeholder="••••••••"
            minLength={8}
            required
          />

          <div className="pt-2">
            <Button type="submit" disabled={loading} loading={loading} className="gap-1.5">
              <span>{loading ? 'Updating...' : 'Update password'}</span>
              {!loading && <Check className="size-4" />}
            </Button>
          </div>
        </form>
      </div>

      {/* Runtime card */}
      {settings && (
        <div className="bg-card border border-border rounded-lg overflow-hidden">
          <div className="border-b border-border p-5">
            <h3 className="text-sm font-semibold flex items-center gap-2 text-foreground">
              <Server className="size-4 text-muted-foreground" />
              Runtime
            </h3>
            <p className="text-xs text-muted-foreground mt-1">
              Active paths and daemon settings
            </p>
          </div>

          <div className="p-6 space-y-3 text-xs">
            {[
              { label: 'Listen address', value: settings.listen },
              { label: 'Config path', value: settings.config_path },
              { label: 'Keys path', value: settings.keys_path },
              { label: 'History database', value: settings.history_db_path },
              { label: 'Log level', value: (settings.log_level || 'info').toUpperCase() },
              { label: 'Version', value: settings.version || '0.1.0' },
            ].map((row) => (
              <div key={row.label} className="flex items-center justify-between gap-4 py-1.5 border-b border-border-subtle last:border-b-0">
                <span className="text-muted-foreground shrink-0">{row.label}</span>
                <span className="text-foreground font-mono truncate">{row.value}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
};
