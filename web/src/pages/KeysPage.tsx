import React, { useState, useEffect } from 'react';
import {
  Key,
  Plus,
  Eye,
  EyeOff,
  Copy,
  Check,
  Trash2,
  Pencil,
} from 'lucide-react';
import { api } from '@/lib/api';
import { KeyItem } from '@/types/api';
import { Button } from '@/components/Button';
import { Input } from '@/components/Input';
import { Select } from '@/components/Select';
import { Modal } from '@/components/Modal';
import { StatusBadge } from '@/components/StatusBadge';
import { Banner } from '@/components/Banner';
import { ConfirmDialog } from '@/components/ConfirmDialog';

// Expiry choices for the create/edit dialog: never, 7 days, 30 days, or a
// custom absolute date (spec: API keys are managed from the dashboard).
type ExpiryPreset = 'never' | '7d' | '30d' | 'custom';

function resolveExpiry(preset: ExpiryPreset, customDate: string): string {
  switch (preset) {
    case '7d': {
      const d = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000);
      return d.toISOString().slice(0, 10);
    }
    case '30d': {
      const d = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000);
      return d.toISOString().slice(0, 10);
    }
    case 'custom':
      return customDate;
    default:
      return 'never';
  }
}

function envSnippet(keyName: string): string {
  return `# Point any OpenAI-compatible client at tinyroute with this key\nexport OPENAI_BASE_URL="http://127.0.0.1:8787/openai/v1"\nexport OPENAI_API_KEY="<key: ${keyName}>"`;
}

export const KeysPage: React.FC = () => {
  const [keys, setKeys] = useState<KeyItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  // Modals & revealed keys state
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [keyName, setKeyName] = useState('');
  const [rateCount, setRateCount] = useState('');
  const [rateInterval, setRateInterval] = useState<'1m' | '1d'>('1m');
  const [expiryPreset, setExpiryPreset] = useState<ExpiryPreset>('never');
  const [expiryDate, setExpiryDate] = useState('');
  const [revealedKeyIds, setRevealedKeyIds] = useState<Record<string, boolean>>({});
  const [copiedKeyId, setCopiedKeyId] = useState<string | null>(null);
  const [confirmRevokeId, setConfirmRevokeId] = useState<string | null>(null);

  // Edit dialog
  const [editingKey, setEditingKey] = useState<KeyItem | null>(null);
  const [editName, setEditName] = useState('');
  const [editRateCount, setEditRateCount] = useState('');
  const [editRateInterval, setEditRateInterval] = useState<'1m' | '1d'>('1m');
  const [editExpiryPreset, setEditExpiryPreset] = useState<ExpiryPreset>('never');
  const [editExpiryDate, setEditExpiryDate] = useState('');

  // Minted key display
  const [mintedKey, setMintedKey] = useState<string | null>(null);
  const [mintedKeyName, setMintedKeyName] = useState('');
  const [copiedMinted, setCopiedMinted] = useState(false);
  const [copiedSnippet, setCopiedSnippet] = useState(false);

  const fetchKeys = async () => {
    try {
      setLoading(true);
      const res = await api.getKeys();
      setKeys(res.keys || []);
    } catch (err) {
      console.error('Failed to load keys', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchKeys();
  }, []);

  const handleCreateKey = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!keyName) return;
    try {
      const count = rateCount ? parseInt(rateCount, 10) : undefined;
      const res = await api.createKey(
        keyName,
        rateInterval === '1m' ? count : undefined,
        rateInterval === '1d' ? count : undefined,
        resolveExpiry(expiryPreset, expiryDate) || undefined
      );
      setMintedKey(res.key);
      setMintedKeyName(keyName);
      setKeyName('');
      setRateCount('');
      setExpiryPreset('never');
      setExpiryDate('');
      setIsCreateOpen(false);
      await fetchKeys();
    } catch (err: any) {
      setError(err.message || 'Failed to create key');
    }
  };

  const handleRevokeKey = async (id: string) => {
    try {
      await api.revokeKey(id);
      setNotice('Key revoked. Clients using it lose access immediately.');
      await fetchKeys();
    } catch (err: any) {
      setError(err.message || 'Failed to revoke key');
    }
  };

  const openEdit = (k: KeyItem) => {
    setEditingKey(k);
    setEditName(k.name);
    if (k.rate_limit_rpm || k.rate_limit_rpd) {
      setEditRateCount(String(k.rate_limit_rpm || k.rate_limit_rpd));
      setEditRateInterval(k.rate_limit_rpd ? '1d' : '1m');
    } else {
      setEditRateCount('');
      setEditRateInterval('1m');
    }
    if (k.expires) {
      setEditExpiryPreset('custom');
      setEditExpiryDate(new Date(k.expires).toISOString().slice(0, 10));
    } else {
      setEditExpiryPreset('never');
      setEditExpiryDate('');
    }
  };

  const handleUpdateKey = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingKey || !editName) return;
    try {
      const count = editRateCount ? parseInt(editRateCount, 10) : undefined;
      await api.updateKey(editingKey.id, {
        name: editName,
        expires_at: resolveExpiry(editExpiryPreset, editExpiryDate),
        rate_limit_rpm: editRateInterval === '1m' ? count : undefined,
        rate_limit_rpd: editRateInterval === '1d' ? count : undefined,
      });
      setEditingKey(null);
      setNotice('Key updated. The secret is unchanged.');
      await fetchKeys();
    } catch (err: any) {
      setError(err.message || 'Failed to update key');
    }
  };

  const toggleReveal = (id: string) => {
    setRevealedKeyIds((prev) => ({ ...prev, [id]: !prev[id] }));
  };

  const handleCopySecret = (id: string, secret: string) => {
    navigator.clipboard.writeText(secret);
    setCopiedKeyId(id);
    setTimeout(() => setCopiedKeyId(null), 2000);
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground">API keys</h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            Client authentication keys — rate limits and expiry
          </p>
        </div>

        <Button onClick={() => setIsCreateOpen(true)} className="gap-2">
          <Plus className="size-4" />
          <span>Create key</span>
        </Button>
      </div>

      {error && <Banner variant="error" onDismiss={() => setError(null)}>{error}</Banner>}
      {notice && <Banner variant="success" onDismiss={() => setNotice(null)}>{notice}</Banner>}

      {/* Minted key */}
      {mintedKey && (
        <div className="bg-success/10 border border-success/30 rounded-lg p-5 space-y-3">
          <div className="flex items-center gap-2 text-success font-semibold text-sm">
            <Key className="size-4" />
            <span>Key created</span>
          </div>
          <p className="text-xs text-muted-foreground">
            Copy the secret now — it cannot be shown in full again.
          </p>
          <div className="flex items-center gap-2">
            <input
              type="text"
              readOnly
              value={mintedKey}
              className="flex-1 bg-background border border-success/40 rounded-md px-3 py-2 text-xs font-mono text-success selection:bg-success/30 select-all"
              onFocus={(e) => e.target.select()}
            />
            <Button
              onClick={() => {
                navigator.clipboard.writeText(mintedKey);
                setCopiedMinted(true);
                setTimeout(() => setCopiedMinted(false), 2000);
              }}
              className="gap-1.5 text-xs"
            >
              {copiedMinted ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
              <span>{copiedMinted ? 'Copied' : 'Copy key'}</span>
            </Button>
            <Button
              variant="outline"
              onClick={() => setMintedKey(null)}
              className="text-xs"
            >
              Dismiss
            </Button>
          </div>
          <div className="space-y-2">
            <span className="text-xs font-medium text-muted-foreground flex items-center justify-between">
              <span>Client environment</span>
              <button
                onClick={() => {
                  navigator.clipboard.writeText(envSnippet(mintedKeyName));
                  setCopiedSnippet(true);
                  setTimeout(() => setCopiedSnippet(false), 2000);
                }}
                className="text-xs text-accent-foreground hover:underline flex items-center gap-1 cursor-pointer"
              >
                {copiedSnippet ? <Check className="size-3 text-success" /> : <Copy className="size-3" />}
                <span>{copiedSnippet ? 'Copied' : 'Copy snippet'}</span>
              </button>
            </span>
            <pre className="p-3 bg-background border border-border-subtle rounded-md text-xs font-mono overflow-x-auto text-foreground whitespace-pre-wrap">
              {envSnippet(mintedKeyName)}
            </pre>
          </div>
        </div>
      )}

      {/* Keys table */}
      <div className="bg-card border border-border rounded-lg overflow-hidden">
        {keys.length === 0 && !loading ? (
          <div className="py-16 text-center text-xs text-muted-foreground p-8 space-y-3">
            <div className="size-11 rounded-lg bg-muted border border-border flex items-center justify-center mx-auto">
              <Key className="size-5 text-muted-foreground" />
            </div>
            <div>
              <p className="font-medium text-foreground">No API keys</p>
              <p className="mt-0.5">Create a key so clients can connect to tinyroute.</p>
            </div>
            <Button onClick={() => setIsCreateOpen(true)} variant="secondary" className="gap-2 mt-2">
              <Plus className="size-4" />
              <span>Create a key</span>
            </Button>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-border text-muted-foreground bg-muted/30">
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Name</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Key ID</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Secret</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Rate</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Expires</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Status</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px] text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border-subtle">
                {keys.map((k) => {
                  const isRevealed = revealedKeyIds[k.id];
                  const displaySecret = isRevealed && k.secret ? k.secret : k.masked;
                  const isCopied = copiedKeyId === k.id;

                  return (
                    <tr key={k.id} className="hover:bg-secondary/40 transition-colors">
                      <td className="py-3.5 px-4 font-medium text-foreground">{k.name}</td>
                      <td className="py-3.5 px-4 font-mono text-muted-foreground">{k.id}</td>
                      <td className="py-3.5 px-4">
                        <div className="flex items-center gap-1.5">
                          <span
                            className={
                              isRevealed
                                ? 'text-foreground font-mono select-all bg-muted/60 px-1 py-0.5 rounded'
                                : 'text-muted-foreground font-mono'
                            }
                          >
                            {displaySecret}
                          </span>
                          {k.secret && (
                            <>
                              <button
                                type="button"
                                onClick={() => toggleReveal(k.id)}
                                title="Reveal / mask secret"
                                className="p-1 text-muted-foreground hover:text-foreground rounded transition-colors cursor-pointer"
                              >
                                {isRevealed ? (
                                  <EyeOff className="size-3.5" />
                                ) : (
                                  <Eye className="size-3.5" />
                                )}
                              </button>
                              <button
                                type="button"
                                onClick={() => handleCopySecret(k.id, k.secret!)}
                                title="Copy secret"
                                className="p-1 text-muted-foreground hover:text-foreground rounded transition-colors cursor-pointer"
                              >
                                {isCopied ? (
                                  <Check className="size-3.5 text-success" />
                                ) : (
                                  <Copy className="size-3.5" />
                                )}
                              </button>
                            </>
                          )}
                        </div>
                      </td>
                      <td className="py-3.5 px-4 font-mono text-foreground/80">{k.rate_spec}</td>
                      <td className="py-3.5 px-4 font-mono text-muted-foreground">{k.expires_formatted || 'Never'}</td>
                      <td className="py-3.5 px-4">
                        <StatusBadge variant={k.is_active ? 'success' : 'warning'}>
                          {k.is_active ? 'Active' : 'Inactive'}
                        </StatusBadge>
                      </td>
                      <td className="py-3.5 px-4 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <button
                            onClick={() => openEdit(k)}
                            className="p-1.5 text-muted-foreground hover:text-foreground rounded transition-colors cursor-pointer"
                            title="Edit key"
                          >
                            <Pencil className="size-3.5" />
                          </button>
                          <button
                            onClick={() => setConfirmRevokeId(k.id)}
                            className="p-1.5 text-muted-foreground hover:text-destructive-text rounded transition-colors cursor-pointer"
                            title="Revoke key"
                          >
                            <Trash2 className="size-3.5" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Modal: create key */}
      <Modal isOpen={isCreateOpen} onClose={() => setIsCreateOpen(false)} title="Create API key">
        <form onSubmit={handleCreateKey} className="space-y-4">
          <Input
            label="Key name"
            value={keyName}
            onChange={(e) => setKeyName(e.target.value)}
            placeholder="e.g. claude-code, dev-agent, production-service"
            className="text-xs"
            required
          />

          <div className="grid grid-cols-2 gap-3">
            <Input
              label="Rate limit (optional)"
              type="number"
              min={1}
              value={rateCount}
              onChange={(e) => setRateCount(e.target.value)}
              placeholder="e.g. 60"
              className="text-xs"
            />
            <Select
              label="Interval"
              value={rateInterval}
              onChange={(e) => setRateInterval(e.target.value as '1m' | '1d')}
              className="text-xs"
              options={[
                { value: '1m', label: 'Per minute' },
                { value: '1d', label: 'Per day' },
              ]}
            />
          </div>

          <Select
            label="Expiry"
            value={expiryPreset}
            onChange={(e) => setExpiryPreset(e.target.value as ExpiryPreset)}
            className="text-xs"
            options={[
              { value: 'never', label: 'Never expires' },
              { value: '7d', label: '7 days' },
              { value: '30d', label: '30 days' },
              { value: 'custom', label: 'Custom date…' },
            ]}
          />
          {expiryPreset === 'custom' && (
            <Input
              label="Expiry date"
              type="date"
              value={expiryDate}
              onChange={(e) => setExpiryDate(e.target.value)}
              className="text-xs"
              required
            />
          )}

          <div className="flex justify-end gap-2.5 pt-2 border-t border-border">
            <Button type="button" variant="outline" onClick={() => setIsCreateOpen(false)}>
              Cancel
            </Button>
            <Button type="submit">Create key</Button>
          </div>
        </form>
      </Modal>

      {/* Modal: edit key */}
      <Modal
        isOpen={editingKey !== null}
        onClose={() => setEditingKey(null)}
        title="Edit API key"
        description={editingKey ? `${editingKey.id} — the secret is never rotated` : undefined}
      >
        <form onSubmit={handleUpdateKey} className="space-y-4">
          <Input
            label="Key name"
            value={editName}
            onChange={(e) => setEditName(e.target.value)}
            className="text-xs"
            required
          />

          <div className="grid grid-cols-2 gap-3">
            <Input
              label="Rate limit"
              type="number"
              min={1}
              value={editRateCount}
              onChange={(e) => setEditRateCount(e.target.value)}
              placeholder="Empty for unlimited"
              className="text-xs"
            />
            <Select
              label="Interval"
              value={editRateInterval}
              onChange={(e) => setEditRateInterval(e.target.value as '1m' | '1d')}
              className="text-xs"
              options={[
                { value: '1m', label: 'Per minute' },
                { value: '1d', label: 'Per day' },
              ]}
            />
          </div>

          <Select
            label="Expiry"
            value={editExpiryPreset}
            onChange={(e) => setEditExpiryPreset(e.target.value as ExpiryPreset)}
            className="text-xs"
            options={[
              { value: 'never', label: 'Never expires' },
              { value: 'custom', label: 'Custom date…' },
            ]}
          />
          {editExpiryPreset === 'custom' && (
            <Input
              label="Expiry date"
              type="date"
              value={editExpiryDate}
              onChange={(e) => setEditExpiryDate(e.target.value)}
              className="text-xs"
              required
            />
          )}

          <div className="flex justify-end gap-2.5 pt-2 border-t border-border">
            <Button type="button" variant="outline" onClick={() => setEditingKey(null)}>
              Cancel
            </Button>
            <Button type="submit">Save changes</Button>
          </div>
        </form>
      </Modal>

      {/* Confirm: revoke */}
      <ConfirmDialog
        open={confirmRevokeId !== null}
        title="Revoke key"
        description={`Revoke key ${confirmRevokeId}? Clients using it lose access immediately. This cannot be undone.`}
        confirmLabel="Revoke key"
        onConfirm={() => confirmRevokeId && handleRevokeKey(confirmRevokeId)}
        onClose={() => setConfirmRevokeId(null)}
      />
    </div>
  );
};
