import React, { useState, useEffect } from 'react';
import {
  Search,
  Plus,
  ArrowLeft,
  Key,
  Trash2,
  RotateCw,
  Zap,
  Copy,
  Check,
  Pencil,
  ExternalLink,
} from 'lucide-react';
import { api } from '@/lib/api';
import { ProviderDetail, ProviderItem, PresetItem } from '@/types/api';
import { ProviderLogo } from '@/components/ProviderLogo';
import { StatusBadge } from '@/components/StatusBadge';
import { Button } from '@/components/Button';
import { Input } from '@/components/Input';
import { Select } from '@/components/Select';
import { Modal } from '@/components/Modal';
import { Banner } from '@/components/Banner';
import { ConfirmDialog } from '@/components/ConfirmDialog';

interface Props {
  initialProvider?: string | null;
  onClearInitialProvider?: () => void;
}

const SECTION_ORDER = ['Free Tier', 'OAuth', 'API Key'] as const;

const STATUS_META: Record<string, { label: string; variant: 'success' | 'warning' | 'error' | 'neutral' }> = {
  connected: { label: 'Connected', variant: 'success' },
  awaiting_credentials: { label: 'Awaiting credentials', variant: 'warning' },
  cooldown: { label: 'Cooldown', variant: 'error' },
  not_connected: { label: 'Not connected', variant: 'neutral' },
};

const AVAILABLE_MODELS_PAGE = 12;

interface CatalogCard {
  name: string;
  displayName: string;
  logo: string;
  dialect: string;
  tier?: string;
  freeNote?: string;
  oauthCapable?: boolean;
  configured?: ProviderItem;
  section: string;
}

function titleCaseFallback(s: string): string {
  return s
    .split(/[-_\s]+/)
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(' ');
}

export const ProvidersPage: React.FC<Props> = ({ initialProvider, onClearInitialProvider }) => {
  const [providers, setProviders] = useState<ProviderItem[]>([]);
  const [presets, setPresets] = useState<PresetItem[]>([]);
  const [search, setSearch] = useState('');
  const [selectedProviderName, setSelectedProviderName] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Detail state
  const [detail, setDetail] = useState<ProviderDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [modelFilter, setModelFilter] = useState('');
  const [availableShown, setAvailableShown] = useState(AVAILABLE_MODELS_PAGE);

  // Modals & form state
  const [isAddCustomOpen, setIsAddCustomOpen] = useState(false);
  const [customName, setCustomName] = useState('');
  const [customDialect, setCustomDialect] = useState('openai');
  const [customBaseURL, setCustomBaseURL] = useState('');
  const [customAPIKey, setCustomAPIKey] = useState('');
  const [customModels, setCustomModels] = useState('');

  // Connections form inside detail
  const [isAddCredOpen, setIsAddCredOpen] = useState(false);
  const [credAccountName, setCredAccountName] = useState('');
  const [credAPIKey, setCredAPIKey] = useState('');
  const [oauthAccount, setOauthAccount] = useState('');

  // Rename Account modal
  const [isRenameOpen, setIsRenameOpen] = useState(false);
  const [renameOldAccount, setRenameOldAccount] = useState('');
  const [renameNewAccount, setRenameNewAccount] = useState('');

  // Destructive confirmations
  const [confirmDeleteProvider, setConfirmDeleteProvider] = useState<string | null>(null);
  const [confirmDeleteCred, setConfirmDeleteCred] = useState<{ name: string; affected: number } | null>(null);

  // In-situ model probing
  const [probingModel, setProbingModel] = useState<string | null>(null);
  const [probeResult, setProbeResult] = useState<Record<string, { success: boolean; status_code: number; duration_ms: number; error?: string }>>({});
  const [copiedModel, setCopiedModel] = useState<string | null>(null);

  const fetchProviders = async () => {
    try {
      const res = await api.getProviders();
      setProviders(res.providers || []);
      setPresets(res.presets || []);
    } catch (err) {
      console.error('Failed to load providers', err);
    }
  };

  useEffect(() => {
    fetchProviders();
  }, []);

  useEffect(() => {
    if (initialProvider) {
      setSelectedProviderName(initialProvider);
      onClearInitialProvider?.();
    }
  }, [initialProvider]);

  const fetchDetail = async (name: string) => {
    setDetailLoading(true);
    setModelFilter('');
    setAvailableShown(AVAILABLE_MODELS_PAGE);
    try {
      const d = await api.getProviderDetail(name);
      setDetail(d);
    } catch (err: any) {
      setError(err.message || 'Failed to load provider detail');
      setDetail(null);
    } finally {
      setDetailLoading(false);
    }
  };

  useEffect(() => {
    if (selectedProviderName) {
      fetchDetail(selectedProviderName);
    } else {
      setDetail(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedProviderName]);

  const handleAddCustomProvider = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!customName || !customBaseURL) return;
    try {
      const models = customModels
        .split('\n')
        .map((m) => m.trim())
        .filter(Boolean);
      await api.addCustomProvider({
        name: customName,
        dialect: customDialect,
        base_url: customBaseURL,
        api_key: customAPIKey,
        models,
      });
      setIsAddCustomOpen(false);
      setCustomName('');
      setCustomBaseURL('');
      setCustomAPIKey('');
      setCustomModels('');
      await fetchProviders();
    } catch (err: any) {
      setError(err.message || 'Failed to create custom provider');
    }
  };

  const handleDeleteProvider = async (name: string) => {
    try {
      await api.deleteProvider(name);
      setSelectedProviderName(null);
      await fetchProviders();
    } catch (err: any) {
      setError(err.message || 'Failed to delete provider');
    }
  };

  const handleSaveCredential = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedProviderName || !credAPIKey) return;
    try {
      await api.saveCredential(selectedProviderName, credAccountName || 'default', credAPIKey);
      setIsAddCredOpen(false);
      setCredAccountName('');
      setCredAPIKey('');
      await Promise.all([fetchProviders(), fetchDetail(selectedProviderName)]);
    } catch (err: any) {
      setError(err.message || 'Failed to save credential');
    }
  };

  const handleDeleteCredential = async (account: string) => {
    if (!selectedProviderName) return;
    try {
      await api.deleteCredential(selectedProviderName, account);
      await Promise.all([fetchProviders(), fetchDetail(selectedProviderName)]);
    } catch (err: any) {
      setError(err.message || 'Failed to delete credential');
    }
  };

  const handleRenameAccount = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedProviderName || !renameOldAccount || !renameNewAccount) return;
    try {
      await api.renameAccount(selectedProviderName, renameOldAccount, renameNewAccount);
      setIsRenameOpen(false);
      setRenameOldAccount('');
      setRenameNewAccount('');
      await Promise.all([fetchProviders(), fetchDetail(selectedProviderName)]);
    } catch (err: any) {
      setError(err.message || 'Failed to rename account');
    }
  };

  const handleModelAction = async (model: string, action: 'add' | 'remove') => {
    if (!selectedProviderName) return;
    try {
      if (action === 'add') {
        await api.addModel(selectedProviderName, model);
      } else {
        await api.removeModel(selectedProviderName, model);
      }
      await Promise.all([fetchProviders(), fetchDetail(selectedProviderName)]);
    } catch (err: any) {
      setError(err.message || `Failed to ${action} model`);
    }
  };

  const handleTestProbe = async (model: string) => {
    if (!selectedProviderName || !detail) return;
    setProbingModel(model);
    try {
      const res = await api.testModelProbe(selectedProviderName, model, detail.dialect);
      setProbeResult((prev) => ({
        ...prev,
        [model]: {
          success: res.success,
          status_code: res.status_code,
          duration_ms: res.duration_ms,
          error: res.error,
        },
      }));
    } catch (err: any) {
      setProbeResult((prev) => ({
        ...prev,
        [model]: {
          success: false,
          status_code: 500,
          duration_ms: 0,
          error: err.message,
        },
      }));
    } finally {
      setProbingModel(null);
    }
  };

  const handleCopyModel = (model: string) => {
    if (!selectedProviderName) return;
    navigator.clipboard.writeText(`${selectedProviderName}:${model}`);
    setCopiedModel(model);
    setTimeout(() => setCopiedModel(null), 2000);
  };

  // ==========================================
  // DETAIL VIEW
  // ==========================================
  if (selectedProviderName && detail) {
    const statusMeta = STATUS_META[detail.status] || STATUS_META.not_connected;
    const connections = detail.connections || [];
    const match = modelFilter.toLowerCase();

    const whitelisted = (detail.whitelisted_models || []).filter(
      (m) => !match || m.id.toLowerCase().includes(match)
    );
    const availableAll = (detail.available_models || []).filter(
      (m) => !match || m.id.toLowerCase().includes(match)
    );
    const available = availableAll.slice(0, availableShown);

    return (
      <div className="space-y-6 max-w-5xl mx-auto">
        {error && <Banner variant="error" onDismiss={() => setError(null)}>{error}</Banner>}

        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-border">
          <div className="flex items-center gap-3">
            <button
              onClick={() => setSelectedProviderName(null)}
              className="p-2 rounded-md bg-card border border-border text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
              aria-label="Back to providers"
            >
              <ArrowLeft className="size-4" />
            </button>
            <ProviderLogo name={detail.logo || detail.name} className="size-8" />
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-xl font-semibold tracking-tight text-foreground">
                  {detail.display_name || detail.name}
                </h2>
                <StatusBadge variant={statusMeta.variant}>{statusMeta.label}</StatusBadge>
              </div>
              <p className="text-xs text-muted-foreground font-mono mt-0.5">
                {detail.dialect} · {detail.base_url || 'default base URL'} · {detail.connection_count}{' '}
                {detail.connection_count === 1 ? 'connection' : 'connections'}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            {detail.oauth_capable && (
              <div className="flex items-center gap-1.5">
                <Input
                  value={oauthAccount}
                  onChange={(e) => setOauthAccount(e.target.value)}
                  placeholder="account label (optional)"
                  className="text-xs w-44"
                  aria-label="OAuth account label"
                />
                <a
                  href={`/dashboard/providers/${detail.name}/oauth/start${oauthAccount ? `?account=${encodeURIComponent(oauthAccount)}` : ''}`}
                  className="inline-flex items-center gap-1.5 text-xs font-medium px-3 py-2 rounded-md bg-primary text-primary-foreground hover:bg-primary-hover transition-colors"
                >
                  <ExternalLink className="size-3.5" />
                  Connect
                </a>
              </div>
            )}
            {detail.configured && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setConfirmDeleteProvider(detail.name)}
                className="gap-1.5 text-muted-foreground hover:text-destructive-text"
              >
                <Trash2 className="size-3.5" />
                Remove
              </Button>
            )}
          </div>
        </div>

        {/* Connections & models */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Left: connections */}
          <div className="bg-card border border-border rounded-lg overflow-hidden flex flex-col">
            <div className="p-5 border-b border-border flex items-center justify-between">
              <div>
                <h3 className="text-sm font-semibold flex items-center gap-2 text-foreground">
                  <Key className="size-4 text-muted-foreground" />
                  Connections
                </h3>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Accounts whose keys route through this provider
                </p>
              </div>
              <Button size="sm" variant="secondary" onClick={() => setIsAddCredOpen(true)} className="gap-1">
                <Plus className="size-3.5" />
                Add key
              </Button>
            </div>

            <div className="p-5 flex-1 space-y-2.5">
              {connections.length === 0 && (
                <div className="py-8 text-center text-xs text-muted-foreground bg-muted/40 border border-border-subtle rounded-lg">
                  No connections yet. Adding a key or whitelisting a model
                  activates this provider.
                </div>
              )}

              {connections.map((acc) => (
                <div
                  key={acc.name}
                  className="flex items-center justify-between p-3 bg-muted/40 border border-border-subtle rounded-lg text-xs"
                >
                  <div>
                    <div className="flex items-center gap-1.5">
                      <span className="font-medium text-foreground">{acc.name}</span>
                      <span className="text-[10px] font-mono px-1.5 rounded bg-muted border border-border text-muted-foreground uppercase">
                        {acc.type}
                      </span>
                    </div>
                    <div className="text-[11px] font-mono text-muted-foreground mt-0.5">
                      {acc.masked_token || '••••••••'}
                      {acc.expires_at && acc.expires_at !== 'Never' && (
                        <span className="text-muted-foreground"> · expires {acc.expires_at}</span>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => {
                        setRenameOldAccount(acc.name);
                        setRenameNewAccount(acc.name);
                        setIsRenameOpen(true);
                      }}
                      className="p-1.5 text-muted-foreground hover:text-foreground rounded-md transition-colors cursor-pointer"
                      title="Rename account"
                    >
                      <Pencil className="size-3.5" />
                    </button>
                    <button
                      onClick={() =>
                        setConfirmDeleteCred({ name: acc.name, affected: acc.affected_combo_count || 0 })
                      }
                      className="p-1.5 text-muted-foreground hover:text-destructive-text rounded-md transition-colors cursor-pointer"
                      title="Delete account"
                    >
                      <Trash2 className="size-3.5" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Right: models */}
          <div className="bg-card border border-border rounded-lg overflow-hidden flex flex-col">
            <div className="p-5 border-b border-border space-y-3">
              <div>
                <h3 className="text-sm font-semibold flex items-center gap-2 text-foreground">
                  <Zap className="size-4 text-muted-foreground" />
                  Models
                </h3>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Whitelisted models plus the provider catalog — add to route
                </p>
              </div>
              <div className="relative">
                <Search className="size-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground pointer-events-none" />
                <Input
                  type="search"
                  placeholder="Filter models…"
                  value={modelFilter}
                  onChange={(e) => {
                    setModelFilter(e.target.value);
                    setAvailableShown(AVAILABLE_MODELS_PAGE);
                  }}
                  className="pl-8 text-xs"
                />
              </div>
            </div>

            <div className="p-5 flex-1 space-y-5 max-h-[32rem] overflow-y-auto">
              {/* Whitelisted */}
              <div className="space-y-2">
                <span className="text-[11px] font-medium text-muted-foreground block">
                  Whitelisted ({whitelisted.length})
                </span>
                {whitelisted.length === 0 && (
                  <p className="text-xs text-muted-foreground py-2">Nothing whitelisted yet.</p>
                )}
                {whitelisted.map((m) => {
                  const probe = probeResult[m.id];
                  const isProbing = probingModel === m.id;
                  return (
                    <div
                      key={m.id}
                      className="flex items-center justify-between p-3 bg-muted/40 border border-border-subtle rounded-lg text-xs font-mono"
                    >
                      <div className="space-y-1 min-w-0">
                        <span className="font-medium text-foreground truncate block">{m.id}</span>
                        {probe && (
                          <div
                            className={`text-[10px] flex items-center gap-1.5 font-medium ${
                              probe.success ? 'text-success' : 'text-destructive-text'
                            }`}
                          >
                            <span>HTTP {probe.status_code}</span>
                            <span>·</span>
                            <span>{probe.duration_ms}ms</span>
                            {probe.error && <span className="text-destructive-text/80">({probe.error})</span>}
                          </div>
                        )}
                      </div>

                      <div className="flex items-center gap-1.5 shrink-0">
                        <button
                          onClick={() => handleCopyModel(m.id)}
                          className="p-1.5 text-muted-foreground hover:text-foreground rounded-md transition-colors cursor-pointer"
                          title={`Copy ${detail.name}:${m.id}`}
                        >
                          {copiedModel === m.id ? (
                            <Check className="size-3.5 text-success" />
                          ) : (
                            <Copy className="size-3.5" />
                          )}
                        </button>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={isProbing}
                          onClick={() => handleTestProbe(m.id)}
                          className="text-[11px] h-7 px-2 gap-1"
                        >
                          <RotateCw className={`size-3 ${isProbing ? 'animate-spin' : ''}`} />
                          <span>{isProbing ? 'Testing…' : 'Test'}</span>
                        </Button>
                        <button
                          onClick={() => handleModelAction(m.id, 'remove')}
                          className="p-1.5 text-muted-foreground hover:text-destructive-text rounded-md transition-colors cursor-pointer"
                          title="Remove from whitelist"
                        >
                          <Trash2 className="size-3.5" />
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>

              {/* Available catalog */}
              {available.length > 0 && (
                <div className="space-y-2">
                  <span className="text-[11px] font-medium text-muted-foreground block">
                    Available ({availableAll.length})
                  </span>
                  {available.map((m) => (
                    <div
                      key={m.id}
                      className="flex items-center justify-between p-3 border border-border-subtle rounded-lg text-xs font-mono"
                    >
                      <span className="text-muted-foreground truncate">{m.id}</span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => handleModelAction(m.id, 'add')}
                        className="text-[11px] h-7 px-2 gap-1"
                      >
                        <Plus className="size-3" />
                        Add
                      </Button>
                    </div>
                  ))}
                  {availableAll.length > availableShown && (
                    <button
                      onClick={() => setAvailableShown(availableShown + AVAILABLE_MODELS_PAGE)}
                      className="w-full py-2 text-[11px] font-medium text-accent-foreground hover:underline cursor-pointer"
                    >
                      Show more ({availableAll.length - availableShown} remaining)
                    </button>
                  )}
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Modal: add credential */}
        <Modal
          isOpen={isAddCredOpen}
          onClose={() => setIsAddCredOpen(false)}
          title={`Add connection — ${detail.display_name || detail.name}`}
        >
          <form onSubmit={handleSaveCredential} className="space-y-4">
            <Input
              label="Account label (optional)"
              value={credAccountName}
              onChange={(e) => setCredAccountName(e.target.value)}
              placeholder="e.g. personal, team-pro — first free slot if empty"
              className="text-xs"
            />
            <Input
              label="API key"
              type="password"
              value={credAPIKey}
              onChange={(e) => setCredAPIKey(e.target.value)}
              placeholder="sk-••••••••"
              className="text-xs"
              required
            />
            <p className="text-[11px] text-muted-foreground">
              The key is stored write-only — it never renders again and is masked everywhere.
            </p>
            <div className="flex justify-end gap-2.5 pt-2 border-t border-border">
              <Button type="button" variant="outline" onClick={() => setIsAddCredOpen(false)}>
                Cancel
              </Button>
              <Button type="submit">Save key</Button>
            </div>
          </form>
        </Modal>

        {/* Modal: rename account */}
        <Modal isOpen={isRenameOpen} onClose={() => setIsRenameOpen(false)} title="Rename connection">
          <form onSubmit={handleRenameAccount} className="space-y-4">
            <Input
              label={`New name for "${renameOldAccount}"`}
              value={renameNewAccount}
              onChange={(e) => setRenameNewAccount(e.target.value)}
              className="text-xs"
              required
            />
            <p className="text-[11px] text-muted-foreground">
              Renaming re-keys the credential, the provider account, and any combo member pinned
              to it — in one write.
            </p>
            <div className="flex justify-end gap-2.5 pt-2 border-t border-border">
              <Button type="button" variant="outline" onClick={() => setIsRenameOpen(false)}>
                Cancel
              </Button>
              <Button type="submit">Save name</Button>
            </div>
          </form>
        </Modal>

        {/* Confirm: remove provider */}
        <ConfirmDialog
          open={confirmDeleteProvider !== null}
          title="Remove provider"
          description={`Remove "${confirmDeleteProvider}" from the gateway? ${
            detail.oauth_capable
              ? 'It returns to the provider list as an unconnected preset. '
              : ''
          }Stored credentials are removed and routing to it stops.`}
          confirmLabel="Remove provider"
          onConfirm={() => confirmDeleteProvider && handleDeleteProvider(confirmDeleteProvider)}
          onClose={() => setConfirmDeleteProvider(null)}
        />

        {/* Confirm: delete connection */}
        <ConfirmDialog
          open={confirmDeleteCred !== null}
          title="Remove connection"
          description={
            confirmDeleteCred && confirmDeleteCred.affected > 0
              ? `Remove connection "${confirmDeleteCred.name}"? ${confirmDeleteCred.affected} combo${
                  confirmDeleteCred.affected === 1 ? '' : 's'
                } pin this account — their members downgrade to unpinned in the same write.`
              : `Remove connection "${confirmDeleteCred?.name}"? Requests will no longer use this account.`
          }
          confirmLabel="Remove connection"
          onConfirm={() => confirmDeleteCred && handleDeleteCredential(confirmDeleteCred.name)}
          onClose={() => setConfirmDeleteCred(null)}
        />
      </div>
    );
  }

  // Detail loading state
  if (selectedProviderName && detailLoading) {
    return (
      <div className="space-y-6">
        {error && <Banner variant="error" onDismiss={() => setError(null)}>{error}</Banner>}
        <div className="p-10 text-center text-xs text-muted-foreground animate-pulse">Loading provider…</div>
      </div>
    );
  }

  // ==========================================
  // CATALOG VIEW
  // ==========================================
  // Every known provider: configured entries (custom providers included) plus
  // presets not yet in the topology, grouped Free Tier → OAuth → API Key.
  const configuredByName = new Map(providers.map((p) => [p.name, p]));
  const cards: CatalogCard[] = [];

  for (const p of providers) {
    const pre = presets.find((x) => x.name === p.name);
    cards.push({
      name: p.name,
      displayName: p.display_name || titleCaseFallback(p.name),
      logo: p.logo || p.name,
      dialect: p.dialect,
      tier: pre?.tier || p.tier,
      freeNote: pre?.free_note || p.free_note,
      oauthCapable: pre?.oauth_capable ?? p.oauth_capable,
      configured: p,
      section: p.section || 'API Key',
    });
  }
  for (const pre of presets) {
    if (configuredByName.has(pre.name)) continue;
    cards.push({
      name: pre.name,
      displayName: pre.display_name || titleCaseFallback(pre.name),
      logo: pre.logo || pre.name,
      dialect: pre.dialect,
      tier: pre.tier,
      freeNote: pre.free_note,
      oauthCapable: pre.oauth_capable,
      section: pre.section || 'API Key',
    });
  }

  const match = search.toLowerCase();
  const filteredCards = cards.filter(
    (c) =>
      !match ||
      c.name.toLowerCase().includes(match) ||
      c.displayName.toLowerCase().includes(match) ||
      c.dialect.toLowerCase().includes(match)
  );

  const sections = SECTION_ORDER.map((section) => ({
    section,
    cards: filteredCards.filter((c) => c.section === section),
  })).filter((s) => s.cards.length > 0);

  return (
    <div className="space-y-6">
      {error && <Banner variant="error" onDismiss={() => setError(null)}>{error}</Banner>}

      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground flex items-center gap-2.5">
            <span>Providers</span>
            <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-muted border border-border text-muted-foreground">
              {cards.length} available
            </span>
          </h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            Connect inference providers and manage model routing
          </p>
        </div>

        <Button onClick={() => setIsAddCustomOpen(true)} className="gap-2">
          <Plus className="size-4" />
          <span>Add custom provider</span>
        </Button>
      </div>

      {/* Search */}
      <div className="relative w-full md:w-72">
        <Search className="size-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground pointer-events-none" />
        <Input
          type="search"
          placeholder="Filter by name or dialect..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pl-8 text-xs"
        />
      </div>

      {/* Grouped sections */}
      <div className="space-y-8">
        {sections.map(({ section, cards: sectionCards }) => (
          <section key={section} className="space-y-3">
            <div className="flex items-center gap-3">
              <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                {section}
              </h3>
              <span className="text-[10px] font-mono text-muted-foreground">
                {sectionCards.length}
              </span>
              <div className="flex-1 border-t border-border-subtle" />
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {sectionCards.map((c) => {
                const configured = c.configured;
                const connCount = configured?.connection_count ?? 0;
                const tiered = c.tier === 'free' || c.tier === 'freemium';

                return (
                  <div
                    key={c.name}
                    onClick={() => setSelectedProviderName(c.name)}
                    className="bg-card border border-border rounded-lg p-5 flex flex-col justify-between gap-4 transition-colors hover:border-muted-foreground/40 cursor-pointer group"
                  >
                    <div className="space-y-3">
                      {/* Logo, name, badges */}
                      <div className="flex items-start justify-between gap-3">
                        <div className="flex items-center gap-3">
                          <ProviderLogo name={c.logo} className="size-9 shrink-0" />
                          <div>
                            <h3 className="font-semibold text-sm text-foreground">
                              {c.displayName}
                            </h3>
                            <span className="text-xs font-mono text-muted-foreground">{c.dialect}</span>
                          </div>
                        </div>

                        <div className="flex items-center gap-1.5 shrink-0">
                          {configured && configured.status && (
                            <StatusBadge variant={STATUS_META[configured.status]?.variant || 'neutral'}>
                              {STATUS_META[configured.status]?.label || '—'}
                            </StatusBadge>
                          )}
                          {tiered && (
                            <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-muted text-muted-foreground border border-border capitalize">
                              {c.tier}
                            </span>
                          )}
                          {!tiered && c.oauthCapable && (
                            <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-accent text-accent-foreground border border-primary/30 font-medium">
                              OAuth
                            </span>
                          )}
                        </div>
                      </div>

                      {/* Free tier note */}
                      {c.freeNote && (
                        <p className="text-[11px] text-muted-foreground bg-muted/40 border border-border-subtle rounded-md p-2.5 leading-relaxed">
                          {c.freeNote}
                        </p>
                      )}
                    </div>

                    {/* Footer */}
                    <div className="pt-3 border-t border-border flex items-center justify-between">
                      <span className="text-[11px] font-mono text-muted-foreground">
                        {configured
                          ? `${connCount} ${connCount === 1 ? 'connection' : 'connections'} · ${configured.models?.length || 0} models`
                          : `${c.dialect} preset`}
                      </span>
                      <span className="text-[11px] text-accent-foreground flex items-center gap-1 font-medium opacity-0 group-hover:opacity-100 transition-opacity">
                        Manage
                      </span>
                    </div>
                  </div>
                );
              })}
            </div>
          </section>
        ))}
      </div>

      {/* Modal: add custom provider */}
      <Modal
        isOpen={isAddCustomOpen}
        onClose={() => setIsAddCustomOpen(false)}
        title="Add custom provider"
        description="Any OpenAI- or Anthropic-compatible endpoint"
      >
        <form onSubmit={handleAddCustomProvider} className="space-y-4">
          <Input
            label="Provider name"
            value={customName}
            onChange={(e) => setCustomName(e.target.value)}
            placeholder="e.g. self-hosted-vllm, internal-ollama"
            className="text-xs"
            required
          />

          <Select
            label="Protocol dialect"
            value={customDialect}
            onChange={(e) => setCustomDialect(e.target.value)}
            className="text-xs"
            options={[
              { value: 'openai', label: 'OpenAI compatible (default)' },
              { value: 'anthropic', label: 'Anthropic Messages' },
              { value: 'gemini', label: 'Google Gemini' },
            ]}
          />

          <Input
            label="Base URL"
            value={customBaseURL}
            onChange={(e) => setCustomBaseURL(e.target.value)}
            placeholder="e.g. http://localhost:11434/v1"
            className="text-xs"
            required
          />

          <Input
            label="API key (optional)"
            type="password"
            value={customAPIKey}
            onChange={(e) => setCustomAPIKey(e.target.value)}
            placeholder="sk-••••••••"
            className="text-xs"
          />

          <div className="space-y-1.5">
            <label htmlFor="custom-models" className="block text-xs font-medium text-muted-foreground">
              Routable models (one per line)
            </label>
            <textarea
              id="custom-models"
              value={customModels}
              onChange={(e) => setCustomModels(e.target.value)}
              placeholder="llama3:latest&#10;deepseek-r1:7b&#10;qwen2.5:14b"
              rows={3}
              className="w-full bg-input border border-border rounded-md p-3 text-xs font-mono text-foreground placeholder:text-muted-foreground transition-colors focus-visible:outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring"
            />
          </div>

          <div className="flex justify-end gap-2.5 pt-2 border-t border-border">
            <Button type="button" variant="outline" onClick={() => setIsAddCustomOpen(false)}>
              Cancel
            </Button>
            <Button type="submit">Add provider</Button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
