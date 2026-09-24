import React, { useState, useEffect } from 'react';
import {
  Search,
  ChevronRight,
  ArrowLeft,
  Terminal,
  RotateCcw,
  Check,
  Copy,
  Key,
  Server,
} from 'lucide-react';
import { api } from '@/lib/api';
import { ClientItem } from '@/types/api';
import { ProviderLogo } from '@/components/ProviderLogo';
import { StatusBadge } from '@/components/StatusBadge';
import { Button } from '@/components/Button';
import { Input } from '@/components/Input';
import { Select } from '@/components/Select';
import { Banner } from '@/components/Banner';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { ModelPickerDialog, ModelPickerTrigger } from '@/components/ModelPickerDialog';

export const ClientsPage: React.FC = () => {
  const [clients, setClients] = useState<ClientItem[]>([]);
  const [selectedClientId, setSelectedClientId] = useState<string | null>(null);
  const [clientDetail, setClientDetail] = useState<ClientItem | null>(null);
  const [loading, setLoading] = useState(true);

  // Grid search
  const [search, setSearch] = useState('');

  // Detail form state
  const [selectedBaseURL, setSelectedBaseURL] = useState('');
  const [keyStrategy, setKeyStrategy] = useState<'mint' | 'reuse'>('mint');
  const [selectedKeyID, setSelectedKeyID] = useState('');
  const [slotValues, setSlotValues] = useState<Record<string, string>>({});
  const [contextWindow, setContextWindow] = useState('');
  const [planDiff, setPlanDiff] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);
  const [copiedSnippet, setCopiedSnippet] = useState(false);
  const [confirmReset, setConfirmReset] = useState(false);
  const [confirmApply, setConfirmApply] = useState(false);

  // Slot picker dialog (one open at a time)
  const [pickerSlotId, setPickerSlotId] = useState<string | null>(null);

  // Minted key revealed exactly once on apply
  const [mintedKey, setMintedKey] = useState<string | null>(null);
  const [copiedMinted, setCopiedMinted] = useState(false);

  const fetchClients = async () => {
    try {
      setLoading(true);
      const res = await api.getClients();
      setClients(res.clients || []);
    } catch (err) {
      console.error('Failed to load clients', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchClients();
  }, []);

  const loadClientDetail = async (id: string) => {
    try {
      const detail = await api.getClientDetail(id);
      setClientDetail(detail);
      setSelectedBaseURL(detail.default_base_url || detail.endpoints?.[0]?.url || '');
      setSlotValues({ ...(detail.slot_values || {}) });
      setContextWindow('');
      const keys = detail.existing_keys || [];
      if (detail.selected_key_id) {
        setSelectedKeyID(detail.selected_key_id);
        setKeyStrategy('reuse');
      } else if (keys.length > 0) {
        setSelectedKeyID(keys[0].id);
        setKeyStrategy('mint');
      } else {
        setSelectedKeyID('');
        setKeyStrategy('mint');
      }
      return detail;
    } catch (err) {
      console.error('Failed to load client detail', err);
      return null;
    }
  };

  const handleSelectClient = async (id: string) => {
    setSelectedClientId(id);
    setPlanDiff(null);
    setFeedback(null);
    setMintedKey(null);
    await loadClientDetail(id);
  };

  const buildPayload = () => ({
    base_url: selectedBaseURL,
    key_strategy: keyStrategy,
    key_id: keyStrategy === 'reuse' ? selectedKeyID : undefined,
    slots: slotValues,
    context_window: contextWindow || undefined,
  });

  const handlePlan = async () => {
    if (!selectedClientId) return;
    try {
      const res = await api.planClient(selectedClientId, buildPayload());
      setPlanDiff(res.preview || 'Ready to apply changes.');
    } catch (err: any) {
      setFeedback({ type: 'error', message: err.message || 'Failed to preview plan' });
    }
  };

  const handleApply = async () => {
    if (!selectedClientId) return;
    try {
      const res = await api.applyClient(selectedClientId, buildPayload());
      setConfirmApply(false);
      setPlanDiff(null);
      if (res.minted_key) {
        setMintedKey(res.minted_key);
      } else {
        setFeedback({ type: 'success', message: res.message || 'Configuration applied.' });
      }
      await fetchClients();
      await loadClientDetail(selectedClientId);
    } catch (err: any) {
      setConfirmApply(false);
      setFeedback({ type: 'error', message: err.message || 'Failed to apply configuration' });
    }
  };

  const handleReset = async () => {
    if (!selectedClientId) return;
    try {
      await api.resetClient(selectedClientId);
      setFeedback({ type: 'success', message: 'Client reset to defaults.' });
      await fetchClients();
      await loadClientDetail(selectedClientId);
    } catch (err: any) {
      setFeedback({ type: 'error', message: err.message || 'Failed to reset client' });
    }
  };

  const handleCopySnippet = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedSnippet(true);
    setTimeout(() => setCopiedSnippet(false), 2000);
  };

  const filteredClients = clients.filter(
    (c) =>
      c.name.toLowerCase().includes(search.toLowerCase()) ||
      c.id.toLowerCase().includes(search.toLowerCase()) ||
      c.dialect?.toLowerCase().includes(search.toLowerCase())
  );

  // ==========================================
  // DETAIL VIEW
  // ==========================================
  if (selectedClientId && clientDetail) {
    const manualSnippet =
      clientDetail.manual_snippet ||
      (clientDetail.dialect === 'anthropic'
        ? `export ANTHROPIC_BASE_URL="${selectedBaseURL || clientDetail.default_base_url}"\nexport ANTHROPIC_AUTH_TOKEN="<YOUR_TINYROUTE_KEY>"`
        : `export OPENAI_BASE_URL="${selectedBaseURL || clientDetail.default_base_url}"\nexport OPENAI_API_KEY="<YOUR_TINYROUTE_KEY>"`);

    const endpointOptions = (clientDetail.endpoints || []).map((ep) => ({
      value: ep.url,
      label: `${ep.url}${ep.is_default ? ' (default)' : ''}${ep.is_current ? ' — current' : ''}`,
    }));

    const existingKeys = clientDetail.existing_keys || [];

    return (
      <div className="max-w-3xl mx-auto space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between gap-4 pb-4 border-b border-border">
          <div className="flex items-center gap-3">
            <button
              onClick={() => {
                setSelectedClientId(null);
                setClientDetail(null);
                setMintedKey(null);
              }}
              className="p-2 rounded-md bg-card border border-border text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
              aria-label="Back to clients"
            >
              <ArrowLeft className="size-4" />
            </button>
            <ProviderLogo name={clientDetail.id} className="size-8" />
            <div>
              <div className="flex items-center gap-3">
                <h2 className="text-xl font-semibold tracking-tight text-foreground">{clientDetail.name}</h2>
                <StatusBadge
                  variant={
                    clientDetail.status_state === 'connected'
                      ? 'success'
                      : clientDetail.status_state === 'not_configured'
                      ? 'warning'
                      : 'neutral'
                  }
                >
                  {clientDetail.status_label}
                </StatusBadge>
              </div>
              <p className="text-xs text-muted-foreground font-mono mt-0.5">
                {clientDetail.config_path || 'not installed'}
              </p>
            </div>
          </div>

          {/* In-page client switcher */}
          <div className="hidden sm:block w-52">
            <Select
              value={selectedClientId}
              onChange={(e) => handleSelectClient(e.target.value)}
              className="text-xs"
              aria-label="Switch client"
              options={clients.map((c) => ({ value: c.id, label: c.name }))}
            />
          </div>
        </div>

        {/* Host-local scope notice */}
        <div className="flex items-start gap-2.5 p-3 bg-muted/40 border border-border-subtle rounded-md text-xs text-muted-foreground">
          <Server className="size-4 shrink-0 mt-0.5" />
          <span>
            Client configuration is written on the <strong className="text-foreground font-medium">gateway machine</strong> —
            files are created at their home-directory paths on this host.
          </span>
        </div>

        {/* Feedback */}
        {feedback && (
          <Banner variant={feedback.type} onDismiss={() => setFeedback(null)}>
            {feedback.message}
          </Banner>
        )}

        {/* Minted key — revealed exactly once */}
        {mintedKey && (
          <div className="bg-success/10 border border-success/30 rounded-lg p-5 space-y-3">
            <div className="flex items-center gap-2 text-success font-semibold text-sm">
              <Key className="size-4" />
              <span>Key minted for {clientDetail.name}</span>
            </div>
            <p className="text-xs text-muted-foreground">
              The key is already embedded in the client config. Copy it now — it cannot be shown in full again.
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
              <Button variant="outline" onClick={() => setMintedKey(null)} className="text-xs">
                Dismiss
              </Button>
            </div>
          </div>
        )}

        {/* Configuration */}
        <div className="bg-card border border-border rounded-lg overflow-hidden">
          <div className="border-b border-border p-5">
            <h3 className="text-base font-semibold text-foreground">Endpoint & key</h3>
            <p className="text-xs text-muted-foreground mt-0.5">
              Routing endpoint, credentials, and model slots for {clientDetail.name}.
            </p>
          </div>

          <div className="p-6 space-y-6">
            {/* Endpoints */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <Select
                label="Gateway endpoint"
                value={selectedBaseURL}
                onChange={(e) => setSelectedBaseURL(e.target.value)}
                className="text-xs"
                options={
                  endpointOptions.length > 0
                    ? endpointOptions
                    : [{ value: selectedBaseURL, label: selectedBaseURL }]
                }
              />

              <Input
                label="Current endpoint"
                disabled
                value={clientDetail.current_base_url || '(not configured)'}
                className="text-xs"
              />
            </div>

            {/* Key strategy */}
            <div className="space-y-3 pt-4 border-t border-border">
              <span className="block text-xs font-medium text-muted-foreground">API key</span>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <label
                  className={`flex items-center gap-3 p-3.5 border rounded-lg cursor-pointer transition-colors ${
                    keyStrategy === 'mint'
                      ? 'bg-accent border-primary/50 text-foreground'
                      : 'bg-card border-border text-muted-foreground'
                  }`}
                >
                  <input
                    type="radio"
                    name="key_strategy"
                    value="mint"
                    checked={keyStrategy === 'mint'}
                    onChange={() => setKeyStrategy('mint')}
                    className="accent-[var(--primary)]"
                  />
                  <div>
                    <span className="block text-sm font-medium text-foreground">Mint a fresh key</span>
                    <span className="block text-xs text-muted-foreground">Auto-generate a scoped key</span>
                  </div>
                </label>

                <label
                  className={`flex items-center gap-3 p-3.5 border rounded-lg cursor-pointer transition-colors ${
                    keyStrategy === 'reuse'
                      ? 'bg-accent border-primary/50 text-foreground'
                      : 'bg-card border-border text-muted-foreground'
                  } ${existingKeys.length === 0 ? 'opacity-50 pointer-events-none' : ''}`}
                >
                  <input
                    type="radio"
                    name="key_strategy"
                    value="reuse"
                    checked={keyStrategy === 'reuse'}
                    onChange={() => setKeyStrategy('reuse')}
                    className="accent-[var(--primary)]"
                  />
                  <div>
                    <span className="block text-sm font-medium text-foreground">Reuse an existing key</span>
                    <span className="block text-xs text-muted-foreground">
                      {existingKeys.length > 0 ? 'Pick from active keys' : 'No active keys available'}
                    </span>
                  </div>
                </label>
              </div>

              {keyStrategy === 'reuse' && (
                <div className="pt-1">
                  <Select
                    label="Key"
                    value={selectedKeyID}
                    onChange={(e) => setSelectedKeyID(e.target.value)}
                    className="text-xs"
                    options={existingKeys.map((k) => ({
                      value: k.id,
                      label: `${k.name} (tr_live_${k.prefix}…)`,
                    }))}
                  />
                </div>
              )}
            </div>

            {/* Model slots */}
            {clientDetail.model_slots && clientDetail.model_slots.length > 0 && (
              <div className="space-y-3 pt-4 border-t border-border">
                <span className="block text-xs font-medium text-muted-foreground">Model slots</span>
                <div className="space-y-3">
                  {clientDetail.model_slots.map((slot) => (
                    <div key={slot.id} className="space-y-1">
                      <div className="flex items-center justify-between text-xs">
                        <span className="font-medium text-foreground">{slot.name}</span>
                        {slot.required && <span className="text-[10px] text-warning">Required</span>}
                      </div>
                      <ModelPickerTrigger
                        value={slotValues[slot.id] || ''}
                        required={slot.required}
                        onClick={() => setPickerSlotId(slot.id)}
                      />
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Context window */}
            <div className="pt-4 border-t border-border">
              <Input
                label="Context window (optional)"
                value={contextWindow}
                onChange={(e) => setContextWindow(e.target.value)}
                placeholder="e.g. 200000 — leave empty for the client default"
                className="text-xs"
              />
            </div>

            {/* Plan preview */}
            {planDiff && (
              <div className="space-y-2 pt-4 border-t border-border">
                <span className="block text-xs font-medium text-muted-foreground">Plan preview</span>
                <pre className="p-3 bg-muted/40 border border-border-subtle rounded-md text-xs font-mono overflow-x-auto text-foreground whitespace-pre-wrap">
                  {planDiff}
                </pre>
              </div>
            )}

            {/* Actions */}
            <div className="pt-4 border-t border-border flex items-center justify-between">
              <div className="flex gap-2">
                <Button type="button" variant="outline" size="sm" onClick={handlePlan}>
                  Preview plan
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => setConfirmReset(true)}
                  className="gap-1 text-muted-foreground hover:text-destructive-text"
                >
                  <RotateCcw className="size-3.5" />
                  Reset
                </Button>
              </div>
              <Button type="button" size="sm" onClick={() => setConfirmApply(true)}>
                Apply configuration
              </Button>
            </div>
          </div>
        </div>

        {/* Manual setup */}
        <div className="bg-card border border-border rounded-lg p-5 space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold flex items-center gap-2 text-foreground">
              <Terminal className="size-4 text-muted-foreground" />
              Manual setup
            </h3>
            <button
              onClick={() => handleCopySnippet(manualSnippet)}
              className="text-xs text-accent-foreground hover:underline flex items-center gap-1 cursor-pointer"
            >
              {copiedSnippet ? <Check className="size-3 text-success" /> : <Copy className="size-3" />}
              <span>{copiedSnippet ? 'Copied' : 'Copy'}</span>
            </button>
          </div>
          <pre className="p-3 bg-muted/40 border border-border-subtle rounded-md text-xs font-mono overflow-x-auto text-foreground whitespace-pre-wrap">
            {manualSnippet}
          </pre>
        </div>

        {/* Slot picker dialog */}
        <ModelPickerDialog
          isOpen={pickerSlotId !== null}
          slotName={
            clientDetail.model_slots?.find((s) => s.id === pickerSlotId)?.name || 'slot'
          }
          required={Boolean(
            clientDetail.model_slots?.find((s) => s.id === pickerSlotId)?.required
          )}
          value={(pickerSlotId && slotValues[pickerSlotId]) || ''}
          options={clientDetail.routable_models || []}
          onSelect={(v) => {
            if (pickerSlotId) {
              setSlotValues({ ...slotValues, [pickerSlotId]: v });
            }
          }}
          onClose={() => setPickerSlotId(null)}
        />

        {/* Confirm: apply */}
        <ConfirmDialog
          open={confirmApply}
          title="Apply configuration"
          description={`Write the configuration for ${clientDetail.name}${
            keyStrategy === 'mint' ? ' and mint a new API key' : ''
          }? The target file${
            clientDetail.config_path ? ` (${clientDetail.config_path})` : ''
          } will be backed up if it exists.`}
          confirmLabel="Apply configuration"
          onConfirm={handleApply}
          onClose={() => setConfirmApply(false)}
        />

        {/* Confirm: reset */}
        <ConfirmDialog
          open={confirmReset}
          title="Reset client"
          description={`Reset ${clientDetail.name} to its default configuration? Only fields tinyroute injected are removed; other settings are preserved.`}
          confirmLabel="Reset client"
          onConfirm={handleReset}
          onClose={() => setConfirmReset(false)}
        />
      </div>
    );
  }

  // ==========================================
  // GRID VIEW
  // ==========================================
  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-xl font-semibold tracking-tight text-foreground">Clients</h2>
        <p className="text-xs text-muted-foreground mt-0.5">
          Point coding tools and IDE extensions at the gateway — configured on this machine.
        </p>
      </div>

      {/* Search */}
      <div className="relative">
        <Search className="size-4 absolute left-3.5 top-1/2 -translate-y-1/2 text-muted-foreground pointer-events-none" />
        <Input
          type="search"
          placeholder="Search clients by name or id..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pl-10 text-xs"
        />
      </div>

      {/* Grid */}
      {filteredClients.length === 0 && !loading ? (
        <div className="py-16 text-center text-xs text-muted-foreground bg-card border border-border rounded-lg p-8">
          No clients match "{search}"
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredClients.map((c) => (
            <div
              key={c.id}
              onClick={() => handleSelectClient(c.id)}
              className="bg-card border border-border rounded-lg p-5 flex flex-col justify-between transition-colors hover:border-muted-foreground/40 group cursor-pointer"
            >
              <div>
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-center gap-3">
                    <ProviderLogo name={c.id} className="size-9" />
                    <div>
                      <h3 className="font-semibold text-sm text-foreground">
                        {c.name}
                      </h3>
                      <span className="text-xs font-mono text-muted-foreground">{c.id}</span>
                    </div>
                  </div>
                  <StatusBadge
                    variant={
                      c.status_state === 'connected'
                        ? 'success'
                        : c.status_state === 'not_configured'
                        ? 'warning'
                        : 'neutral'
                    }
                  >
                    {c.status_label}
                  </StatusBadge>
                </div>
              </div>

              <div className="mt-6 pt-4 border-t border-border flex items-center justify-between text-xs text-muted-foreground">
                <div className="flex items-center gap-1.5 font-mono">
                  <Terminal className="size-3.5" />
                  <span>{c.dialect}</span>
                </div>
                <span className="text-accent-foreground flex items-center gap-1 font-medium">
                  Configure
                  <ChevronRight className="size-3.5" />
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
