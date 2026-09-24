import React, { useState, useEffect } from 'react';
import {
  Layers,
  Plus,
  Trash2,
  ArrowUp,
  ArrowDown,
  Route,
  Shuffle,
  Network,
  Pencil,
} from 'lucide-react';
import { api } from '@/lib/api';
import { ComboItem } from '@/types/api';
import { Button } from '@/components/Button';
import { Input } from '@/components/Input';
import { Select } from '@/components/Select';
import { Modal } from '@/components/Modal';
import { Banner } from '@/components/Banner';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { ProviderLogo } from '@/components/ProviderLogo';

// stripAccountPin removes an @account pin for card display; the stored member
// string stays verbatim (spec: combo list with ordered members).
function stripAccountPin(member: string): string {
  const at = member.indexOf('@');
  if (at <= 0) return member;
  const colon = member.indexOf(':', at);
  if (colon < 0) return member;
  return member.slice(0, at) + member.slice(colon);
}

const MEMBER_CHIPS_MAX = 3;

export const CombosPage: React.FC = () => {
  const [combos, setCombos] = useState<ComboItem[]>([]);
  const [availableModels, setAvailableModels] = useState<{ provider: string; model: string }[]>([]);
  const [providerAccounts, setProviderAccounts] = useState<Record<string, string[]>>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  // Wizard state
  const [isWizardOpen, setIsWizardOpen] = useState(false);
  const [editingCombo, setEditingCombo] = useState<string | null>(null);
  const [wizardStep, setWizardStep] = useState<number>(1);
  const [wizardName, setWizardName] = useState('');
  const [wizardMode, setWizardMode] = useState<'ordered' | 'pool' | 'fused'>('ordered');
  const [wizardMembers, setWizardMembers] = useState<string[]>([]);
  const [selectedModelToAdd, setSelectedModelToAdd] = useState('');
  const [selectedConnection, setSelectedConnection] = useState('');
  const [memberError, setMemberError] = useState<string | null>(null);
  const [selectedCapabilities, setSelectedCapabilities] = useState<string[]>([]);

  // Destructive confirmation
  const [confirmDeleteCombo, setConfirmDeleteCombo] = useState<string | null>(null);

  const fetchCombos = async () => {
    try {
      setLoading(true);
      const res = await api.getCombos();
      setCombos(res.combos || []);
      setAvailableModels(res.available_models || []);
      setProviderAccounts(res.provider_accounts || {});
      if (res.available_models && res.available_models.length > 0) {
        setSelectedModelToAdd(`${res.available_models[0].provider}:${res.available_models[0].model}`);
      }
    } catch (err) {
      console.error('Failed to load combos', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchCombos();
  }, []);

  const handleOpenCreateWizard = () => {
    setEditingCombo(null);
    setWizardStep(1);
    setWizardName('');
    setWizardMode('ordered');
    setWizardMembers([]);
    setSelectedCapabilities([]);
    setMemberError(null);
    setIsWizardOpen(true);
  };

  const handleOpenEditWizard = (c: ComboItem) => {
    setEditingCombo(c.name);
    setWizardStep(1);
    setWizardName(c.name);
    setWizardMode((c.mode || 'ordered') as 'ordered' | 'pool' | 'fused');
    setWizardMembers([...c.members]); // verbatim — pins stay intact
    setSelectedCapabilities([...(c.capabilities || [])]);
    setMemberError(null);
    setIsWizardOpen(true);
  };

  // The Connection dropdown is scoped client-side to the selected model's
  // provider; "any" composes the plain provider:model member.
  const selectedModelProvider = selectedModelToAdd.split(':')[0] || '';
  const connectionAccounts = providerAccounts[selectedModelProvider] || [];
  const connectionEnabled =
    selectedModelToAdd !== '' && connectionAccounts.length >= 2;

  useEffect(() => {
    // Reset the connection choice whenever the model changes scope.
    if (connectionEnabled && connectionAccounts.length > 0 && !connectionAccounts.includes(selectedConnection)) {
      setSelectedConnection(connectionAccounts[0]);
    } else if (!connectionEnabled) {
      setSelectedConnection('');
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedModelToAdd, connectionEnabled]);

  const composeMember = (): string => {
    if (!connectionEnabled || !selectedConnection) {
      return selectedModelToAdd;
    }
    const colon = selectedModelToAdd.indexOf(':');
    const provider = selectedModelToAdd.slice(0, colon);
    const model = selectedModelToAdd.slice(colon + 1);
    return `${provider}@${selectedConnection}:${model}`;
  };

  const handleAddMemberToWizard = () => {
    if (!selectedModelToAdd) return;
    const member = composeMember();
    if (wizardMembers.includes(member)) {
      setMemberError(`"${member}" is already in the list.`);
      return;
    }
    setMemberError(null);
    setWizardMembers([...wizardMembers, member]);
  };

  const handleMoveMember = (index: number, direction: 'up' | 'down') => {
    const newMembers = [...wizardMembers];
    const target = direction === 'up' ? index - 1 : index + 1;
    if (target < 0 || target >= newMembers.length) return;
    const temp = newMembers[index];
    newMembers[index] = newMembers[target];
    newMembers[target] = temp;
    setWizardMembers(newMembers);
  };

  const handleRemoveMember = (index: number) => {
    setWizardMembers(wizardMembers.filter((_, i) => i !== index));
  };

  const handleToggleCapability = (cap: string) => {
    if (selectedCapabilities.includes(cap)) {
      setSelectedCapabilities(selectedCapabilities.filter((c) => c !== cap));
    } else {
      setSelectedCapabilities([...selectedCapabilities, cap]);
    }
  };

  const handleSaveWizard = async () => {
    if (!wizardName || wizardMembers.length === 0) return;
    try {
      await api.saveCombo({
        name: wizardName,
        mode: wizardMode,
        members: wizardMembers, // verbatim, pins preserved
        capabilities: selectedCapabilities,
      });
      setIsWizardOpen(false);
      setNotice(
        editingCombo
          ? `Combo "${wizardName}" updated (${wizardMembers.length} members).`
          : `Combo "${wizardName}" created (${wizardMembers.length} members).`
      );
      await fetchCombos();
    } catch (err: any) {
      setError(err.message || 'Failed to save combo');
    }
  };

  const handleToggleCombo = async (name: string, currentStatus: boolean) => {
    try {
      await api.toggleCombo(name, !currentStatus);
      setNotice(`Combo "${name}" ${currentStatus ? 'disabled' : 'enabled'}.`);
      await fetchCombos();
    } catch (err: any) {
      setError(err.message || 'Failed to toggle combo');
    }
  };

  const handleDeleteCombo = async (name: string) => {
    try {
      await api.deleteCombo(name);
      setNotice(`Combo "${name}" deleted.`);
      await fetchCombos();
    } catch (err: any) {
      setError(err.message || 'Failed to delete combo');
    }
  };

  const capabilityOptions = ['vision', 'tools', 'code', 'reasoning', 'audio', 'fast', 'creative'];

  const modeLabel = (mode: string) =>
    mode === 'ordered' ? 'Ordered fallback' : mode === 'pool' ? 'Load-balanced pool' : 'Parallel fusion';

  // Group available models by provider for the Model dropdown.
  const modelsByProvider = availableModels.reduce<Record<string, string[]>>((acc, am) => {
    (acc[am.provider] = acc[am.provider] || []).push(am.model);
    return acc;
  }, {});
  const providerNames = Object.keys(modelsByProvider).sort();

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground flex items-center gap-2.5">
            <span>Combos</span>
            <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-muted border border-border text-muted-foreground">
              {combos.length} active
            </span>
          </h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            Multi-model routes: ordered failover, load-balanced pools, parallel fusion
          </p>
        </div>

        <Button onClick={handleOpenCreateWizard} className="gap-2">
          <Plus className="size-4" />
          <span>New combo</span>
        </Button>
      </div>

      {error && <Banner variant="error" onDismiss={() => setError(null)}>{error}</Banner>}
      {notice && <Banner variant="success" onDismiss={() => setNotice(null)}>{notice}</Banner>}

      {/* Empty state */}
      {combos.length === 0 && !loading ? (
        <div className="py-16 text-center bg-card border border-border rounded-lg p-8 space-y-4">
          <div className="size-11 rounded-lg bg-accent text-accent-foreground border border-border flex items-center justify-center mx-auto">
            <Layers className="size-5" />
          </div>
          <div>
            <h3 className="font-semibold text-sm text-foreground">No combos configured</h3>
            <p className="text-xs text-muted-foreground mt-1 max-w-sm mx-auto leading-relaxed">
              A combo groups several models behind one slug — as an ordered failover, a
              load-balanced pool, or a parallel fusion.
            </p>
          </div>
          <Button onClick={handleOpenCreateWizard} variant="secondary" className="mt-2">
            <span>Create a combo</span>
          </Button>
        </div>
      ) : (
        /* Combo cards */
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {combos.map((c) => {
            const visibleMembers = c.members.slice(0, MEMBER_CHIPS_MAX);
            const hiddenMembers = c.members.slice(MEMBER_CHIPS_MAX);

            return (
              <div
                key={c.name}
                className={`bg-card border rounded-lg p-5 flex flex-col justify-between transition-colors ${
                  c.enabled ? 'border-border hover:border-muted-foreground/40' : 'border-border-subtle opacity-60'
                }`}
              >
                <div className="space-y-4">
                  {/* Header */}
                  <div className="flex items-center justify-between gap-3 pb-3.5 border-b border-border">
                    <div className="flex items-center gap-3 min-w-0">
                      <div
                        className={`p-2.5 rounded-md border ${
                          c.enabled
                            ? 'bg-accent text-accent-foreground border-border'
                            : 'bg-muted text-muted-foreground border-border'
                        }`}
                      >
                        {c.mode === 'pool' ? (
                          <Shuffle className="size-4" />
                        ) : c.mode === 'fused' ? (
                          <Network className="size-4" />
                        ) : (
                          <Route className="size-4" />
                        )}
                      </div>
                      <div>
                        <h3 className="font-semibold text-base text-foreground truncate">{c.name}</h3>
                        <span className="text-[11px] text-muted-foreground">
                          {modeLabel(c.mode)}
                        </span>
                      </div>
                    </div>

                    <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-muted border border-border text-muted-foreground shrink-0">
                      {c.mode}
                    </span>
                  </div>

                  {/* Member chips — position-numbered, capped at 3 */}
                  <div className="space-y-2">
                    <span className="text-[11px] font-medium text-muted-foreground block">
                      {c.mode === 'ordered' ? 'Fallback order' : 'Members'} ({c.members.length})
                    </span>

                    <div className="flex flex-wrap gap-1.5">
                      {visibleMembers.map((m, i) => {
                        const display = stripAccountPin(m);
                        const [prov, model] = display.split(':');
                        return (
                          <span
                            key={i}
                            className="inline-flex items-center gap-1.5 px-2 py-1 rounded-md bg-muted/40 border border-border-subtle text-[11px] font-mono text-foreground"
                            title={m}
                          >
                            <span className="text-muted-foreground font-semibold">{i + 1}.</span>
                            <ProviderLogo name={prov} className="size-3.5 shrink-0" />
                            <span className="truncate max-w-52">{model || prov}</span>
                          </span>
                        );
                      })}
                      {hiddenMembers.length > 0 && (
                        <span
                          className="inline-flex items-center px-2 py-1 rounded-md bg-muted border border-border text-[11px] font-mono text-muted-foreground"
                          title={hiddenMembers.join('\n')}
                        >
                          +{hiddenMembers.length} more…
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Capabilities */}
                  {c.capabilities && c.capabilities.length > 0 && (
                    <div className="pt-1">
                      <span className="text-[11px] font-medium text-muted-foreground block mb-1.5">
                        Capabilities
                      </span>
                      <div className="flex flex-wrap gap-1.5">
                        {c.capabilities.map((cap) => (
                          <span
                            key={cap}
                            className="px-2 py-0.5 rounded-md text-[10px] font-mono bg-muted/60 border border-border-subtle text-muted-foreground"
                          >
                            {cap}
                          </span>
                        ))}
                      </div>
                    </div>
                  )}
                </div>

                {/* Footer — switch, then edit and delete */}
                <div className="pt-4 border-t border-border mt-4 flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <button
                      role="switch"
                      aria-checked={c.enabled}
                      aria-label={`${c.enabled ? 'Disable' : 'Enable'} combo ${c.name}`}
                      onClick={() => handleToggleCombo(c.name, c.enabled)}
                      className={`relative w-9 h-5 rounded-full border transition-colors cursor-pointer ${
                        c.enabled ? 'bg-primary border-primary' : 'bg-muted border-border'
                      }`}
                    >
                      <span
                        className={`absolute top-0.5 size-3.5 rounded-full bg-background border transition-all ${
                          c.enabled ? 'left-[18px] border-primary' : 'left-0.5 border-border'
                        }`}
                      />
                    </button>
                    <span className="text-[11px] font-mono text-muted-foreground">
                      {c.enabled ? 'Active' : 'Disabled'}
                    </span>
                  </div>

                  <div className="flex items-center gap-1">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => handleOpenEditWizard(c)}
                      className="text-muted-foreground hover:text-foreground p-1.5 gap-1"
                      aria-label={`Edit combo ${c.name}`}
                    >
                      <Pencil className="size-3.5" />
                      <span className="text-xs">Edit</span>
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setConfirmDeleteCombo(c.name)}
                      className="text-muted-foreground hover:text-destructive-text p-1.5"
                      aria-label={`Delete combo ${c.name}`}
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Creation / edit wizard */}
      <Modal
        isOpen={isWizardOpen}
        onClose={() => setIsWizardOpen(false)}
        title={editingCombo ? `Edit combo — ${editingCombo}` : 'New combo'}
        description={`Step ${wizardStep} of 5`}
      >
        <div className="space-y-5">
          {/* Step rail */}
          <div className="flex items-center justify-between text-xs font-mono text-muted-foreground border-b border-border pb-2.5">
            {['1 Name', '2 Members', '3 Mode', '4 Capabilities', '5 Confirm'].map((label, i) => (
              <React.Fragment key={label}>
                {i > 0 && <span className="text-muted-foreground/40">·</span>}
                <span className={wizardStep === i + 1 ? 'text-accent-foreground font-semibold' : ''}>
                  {label}
                </span>
              </React.Fragment>
            ))}
          </div>

          {/* STEP 1: name */}
          {wizardStep === 1 && (
            <div className="space-y-4">
              <Input
                label="Combo slug"
                value={wizardName}
                onChange={(e) => setWizardName(e.target.value)}
                placeholder="e.g. smart-fallback, coding-pool, fast-cheap"
                className="text-xs"
                required
                helperText={'Clients request this as their model, e.g. model: "smart-fallback".'}
              />
            </div>
          )}

          {/* STEP 2: members */}
          {wizardStep === 2 && (
            <div className="space-y-4">
              <div className="space-y-2">
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                  <Select
                    label="Model"
                    value={selectedModelToAdd}
                    onChange={(e) => setSelectedModelToAdd(e.target.value)}
                    className="text-xs"
                    aria-label="Select upstream model"
                  >
                    {providerNames.map((prov) => (
                      <optgroup key={prov} label={prov}>
                        {modelsByProvider[prov].map((model) => (
                          <option key={`${prov}:${model}`} value={`${prov}:${model}`}>
                            {model}
                          </option>
                        ))}
                      </optgroup>
                    ))}
                  </Select>

                  <Select
                    label="Connection"
                    value={connectionEnabled ? selectedConnection : ''}
                    onChange={(e) => setSelectedConnection(e.target.value)}
                    className="text-xs"
                    disabled={!connectionEnabled}
                    aria-label="Select connection"
                    options={
                      !connectionEnabled
                        ? [{ value: '', label: selectedModelToAdd ? 'Any connection' : 'Select a model first' }]
                        : [
                            { value: '', label: 'Any connection' },
                            ...connectionAccounts.map((a) => ({ value: a, label: `${selectedModelProvider} · ${a}` })),
                          ]
                    }
                  />
                </div>

                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  onClick={handleAddMemberToWizard}
                  className="gap-1"
                  disabled={!selectedModelToAdd}
                >
                  <Plus className="size-3.5" />
                  Add
                </Button>

                {memberError && (
                  <p className="text-xs text-destructive-text">{memberError}</p>
                )}
              </div>

              <div className="space-y-1.5">
                <span className="block text-xs font-medium text-muted-foreground">
                  Members ({wizardMembers.length}) — position is priority
                </span>
                {wizardMembers.length === 0 ? (
                  <p className="text-xs text-warning py-4 text-center">
                    Add at least one model to continue.
                  </p>
                ) : (
                  <div className="space-y-1.5 max-h-48 overflow-y-auto">
                    {wizardMembers.map((m, idx) => (
                      <div
                        key={idx}
                        className="flex items-center justify-between p-2.5 bg-muted/40 border border-border-subtle rounded-md text-xs font-mono"
                      >
                        <div className="flex items-center gap-2 min-w-0">
                          <span className="size-4 rounded bg-muted border border-border text-muted-foreground flex items-center justify-center text-[10px]">
                            {idx + 1}
                          </span>
                          <span className="text-foreground font-medium truncate" title={m}>{m}</span>
                        </div>
                        <div className="flex items-center gap-1 shrink-0">
                          <button
                            type="button"
                            onClick={() => handleMoveMember(idx, 'up')}
                            disabled={idx === 0}
                            className="p-1 text-muted-foreground hover:text-foreground disabled:opacity-30 cursor-pointer"
                            aria-label="Move up"
                          >
                            <ArrowUp className="size-3" />
                          </button>
                          <button
                            type="button"
                            onClick={() => handleMoveMember(idx, 'down')}
                            disabled={idx === wizardMembers.length - 1}
                            className="p-1 text-muted-foreground hover:text-foreground disabled:opacity-30 cursor-pointer"
                            aria-label="Move down"
                          >
                            <ArrowDown className="size-3" />
                          </button>
                          <button
                            type="button"
                            onClick={() => handleRemoveMember(idx)}
                            className="p-1 text-muted-foreground hover:text-destructive-text cursor-pointer"
                            aria-label="Remove member"
                          >
                            <Trash2 className="size-3" />
                          </button>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}

          {/* STEP 3: mode */}
          {wizardStep === 3 && (
            <div className="space-y-3">
              <span className="block text-xs font-medium text-muted-foreground">Routing strategy</span>
              {[
                {
                  id: 'ordered',
                  label: 'Ordered fallback',
                  desc: 'Try the first model; on a rate limit (429) or failure (5xx), fall through to the next.',
                },
                {
                  id: 'pool',
                  label: 'Load-balanced pool',
                  desc: 'Spread requests evenly across members to avoid single-provider limits.',
                },
                {
                  id: 'fused',
                  label: 'Parallel fusion',
                  desc: 'Query multiple models concurrently and arbitrate the responses.',
                },
              ].map((m) => (
                <label
                  key={m.id}
                  className={`flex items-start gap-3 p-3.5 rounded-lg border cursor-pointer transition-colors ${
                    wizardMode === m.id
                      ? 'bg-accent border-primary/50 text-foreground'
                      : 'bg-card border-border hover:border-muted-foreground/40 text-muted-foreground'
                  }`}
                >
                  <input
                    type="radio"
                    name="wizard_mode"
                    value={m.id}
                    checked={wizardMode === m.id}
                    onChange={() => setWizardMode(m.id as any)}
                    className="mt-1 accent-[var(--primary)]"
                  />
                  <div>
                    <span className="text-xs font-semibold text-foreground block">{m.label}</span>
                    <span className="text-[11px] text-muted-foreground">{m.desc}</span>
                  </div>
                </label>
              ))}
            </div>
          )}

          {/* STEP 4: capabilities */}
          {wizardStep === 4 && (
            <div className="space-y-3">
              <span className="block text-xs font-medium text-muted-foreground">Capabilities</span>
              <div className="grid grid-cols-2 gap-2">
                {capabilityOptions.map((cap) => {
                  const isChecked = selectedCapabilities.includes(cap);
                  return (
                    <label
                      key={cap}
                      className={`flex items-center gap-2 p-2.5 rounded-lg border text-xs font-mono cursor-pointer transition-colors ${
                        isChecked
                          ? 'bg-accent border-primary/40 text-foreground font-medium'
                          : 'bg-muted/40 border-border-subtle text-muted-foreground'
                      }`}
                    >
                      <input
                        type="checkbox"
                        checked={isChecked}
                        onChange={() => handleToggleCapability(cap)}
                        className="accent-[var(--primary)]"
                      />
                      <span className="capitalize">{cap}</span>
                    </label>
                  );
                })}
              </div>
            </div>
          )}

          {/* STEP 5: confirm */}
          {wizardStep === 5 && (
            <div className="space-y-4 text-xs">
              <div className="bg-muted/40 border border-border-subtle rounded-lg p-4 space-y-2.5 font-mono">
                <div>
                  <span className="text-muted-foreground">Slug: </span>
                  <strong className="text-foreground font-semibold">{wizardName}</strong>
                </div>
                <div>
                  <span className="text-muted-foreground">Mode: </span>
                  <span className="capitalize text-accent-foreground font-medium">{wizardMode}</span>
                </div>
                <div>
                  <span className="text-muted-foreground">Members:</span>
                  <div className="pl-3 mt-1 space-y-1">
                    {wizardMembers.map((m, i) => (
                      <div key={i} className="text-foreground">
                        {i + 1}. {m}
                      </div>
                    ))}
                  </div>
                </div>
                {selectedCapabilities.length > 0 && (
                  <div>
                    <span className="text-muted-foreground">Capabilities: </span>
                    <span className="text-foreground">{selectedCapabilities.join(', ')}</span>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Footer */}
          <div className="flex items-center justify-between pt-4 border-t border-border">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                if (wizardStep > 1) {
                  setWizardStep(wizardStep - 1);
                } else {
                  setIsWizardOpen(false);
                }
              }}
            >
              {wizardStep === 1 ? 'Cancel' : 'Back'}
            </Button>

            {wizardStep < 5 ? (
              <Button
                type="button"
                size="sm"
                disabled={(wizardStep === 1 && !wizardName.trim()) || (wizardStep === 2 && wizardMembers.length === 0)}
                onClick={() => setWizardStep(wizardStep + 1)}
              >
                Next
              </Button>
            ) : (
              <Button
                type="button"
                size="sm"
                disabled={wizardMembers.length === 0}
                onClick={handleSaveWizard}
              >
                {editingCombo ? 'Save combo' : 'Create combo'}
              </Button>
            )}
          </div>
        </div>
      </Modal>

      {/* Confirm: delete combo */}
      <ConfirmDialog
        open={confirmDeleteCombo !== null}
        title="Delete combo"
        description={`Delete combo "${confirmDeleteCombo}"? Clients requesting this slug will get an unknown-model error.`}
        confirmLabel="Delete combo"
        onConfirm={() => confirmDeleteCombo && handleDeleteCombo(confirmDeleteCombo)}
        onClose={() => setConfirmDeleteCombo(null)}
      />
    </div>
  );
};
