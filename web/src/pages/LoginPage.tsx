import React, { useState } from 'react';
import { Route, ArrowRight } from 'lucide-react';
import { Button } from '@/components/Button';
import { Input } from '@/components/Input';
import { Banner } from '@/components/Banner';

interface Props {
  onLogin: (password: string) => Promise<{ success: boolean; error?: string }>;
}

export const LoginPage: React.FC<Props> = ({ onLogin }) => {
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!password) return;
    setLoading(true);
    setError('');
    try {
      const res = await onLogin(password);
      if (!res.success) {
        setError(res.error || 'Invalid password');
      }
    } catch (err: any) {
      setError(err.message || 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center p-4 bg-background text-foreground antialiased font-sans">
      <div className="w-full max-w-md">
        <div className="bg-card border border-border rounded-lg p-8 space-y-6">
          {/* Brand & header */}
          <div className="text-center space-y-2">
            <div className="inline-flex p-3 bg-accent text-accent-foreground rounded-lg border border-border mb-2">
              <Route className="size-6" />
            </div>
            <h1 className="text-2xl font-semibold tracking-tight text-foreground">tinyroute dashboard</h1>
            <p className="text-xs text-muted-foreground">Enter the management password to continue</p>
          </div>

          {/* Error */}
          {error && (
            <Banner variant="error" onDismiss={() => setError('')}>{error}</Banner>
          )}

          {/* Form */}
          <form onSubmit={handleSubmit} className="space-y-4">
            <Input
              id="password"
              label="Password"
              type="password"
              placeholder="••••••••"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoFocus
              required
            />

            <Button type="submit" disabled={loading} fullWidth className="gap-2">
              <span>{loading ? 'Signing in...' : 'Sign in'}</span>
              <ArrowRight className="size-4" />
            </Button>
          </form>

          {/* Default password hint */}
          <div className="pt-4 border-t border-border text-center">
            <p className="text-xs text-muted-foreground">
              Default password is{' '}
              <code className="px-1.5 py-0.5 bg-muted rounded border border-border text-foreground font-mono">
                123456
              </code>
            </p>
          </div>
        </div>
      </div>
    </div>
  );
};
