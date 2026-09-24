import React, { useEffect, useMemo, useState } from 'react';
import { Check, ChevronDown, Search } from 'lucide-react';
import { Modal } from '@/components/Modal';

interface Props {
  isOpen: boolean;
  slotName: string;
  required: boolean;
  value: string;
  options: string[];
  onSelect: (value: string) => void;
  onClose: () => void;
}

const NONE_LABEL = '(None / Default)';

function matches(query: string, candidate: string): boolean {
  return candidate.toLowerCase().includes(query.toLowerCase());
}

/**
 * Model picker for client slot editors: a modal dialog with Models and Combos
 * tab panes (tab bar omitted when no combos are routable), each pane carrying
 * its own search. Option rows — including the (None / Default) clear option —
 * render as the same styled row; the selected row carries the accent and a
 * check. Selecting closes the dialog; dismissing leaves the slot unchanged.
 */
export const ModelPickerDialog: React.FC<Props> = ({
  isOpen,
  slotName,
  required,
  value,
  options,
  onSelect,
  onClose,
}) => {
  const models = useMemo(() => options.filter((o) => !o.startsWith('combo:')), [options]);
  const combos = useMemo(
    () =>
      options
        .filter((o) => o.startsWith('combo:'))
        .map((o) => o.slice('combo:'.length))
        .sort((a, b) => a.localeCompare(b)),
    [options]
  );

  const [activeTab, setActiveTab] = useState<'models' | 'combos'>('models');
  const [modelQuery, setModelQuery] = useState('');
  const [comboQuery, setComboQuery] = useState('');

  useEffect(() => {
    if (isOpen) {
      // The Combos pane is active when the slot's current value is a combo.
      setActiveTab(value.startsWith('combo:') ? 'combos' : 'models');
      setModelQuery('');
      setComboQuery('');
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen]);

  // Models grouped by provider; a group whose members all miss the query
  // hides its header along with its rows.
  const modelGroups = useMemo(() => {
    const groups: { provider: string; models: string[] }[] = [];
    for (const m of models) {
      if (modelQuery && !matches(modelQuery, m)) continue;
      const colon = m.indexOf(':');
      const provider = colon > 0 ? m.slice(0, colon) : m;
      const last = groups[groups.length - 1];
      if (last && last.provider === provider) {
        last.models.push(m);
      } else {
        groups.push({ provider, models: [m] });
      }
    }
    return groups;
  }, [models, modelQuery]);

  const filteredCombos = useMemo(
    () => combos.filter((c) => !comboQuery || matches(comboQuery, c)),
    [combos, comboQuery]
  );

  const rowClass = (selected: boolean) =>
    `w-full flex items-center justify-between gap-3 px-3 py-2.5 border rounded-md text-xs font-mono transition-colors text-left cursor-pointer ${
      selected
        ? 'bg-accent border-primary/50 text-foreground'
        : 'bg-card border-border-subtle text-muted-foreground hover:border-muted-foreground/40 hover:text-foreground'
    }`;

  const choose = (v: string) => {
    onSelect(v);
    onClose();
  };

  const noneRow = (visible: boolean) =>
    !required &&
    visible && (
      <button type="button" onClick={() => choose('')} className={rowClass(value === '')}>
        <span>{NONE_LABEL}</span>
        {value === '' && <Check className="size-3.5 text-accent-foreground shrink-0" />}
      </button>
    );

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={`Model for ${slotName}`}>
      <div className="space-y-4">
        {/* Tab bar — omitted entirely when no combos are routable */}
        {combos.length > 0 && (
          <div className="inline-flex items-center bg-card border border-border rounded-lg p-0.5">
            {(
              [
                { key: 'models', label: `Models (${models.length})` },
                { key: 'combos', label: `Combos (${combos.length})` },
              ] as const
            ).map((t) => (
              <button
                key={t.key}
                onClick={() => setActiveTab(t.key)}
                className={`text-xs px-3 py-1.5 rounded-md transition-colors font-medium cursor-pointer ${
                  activeTab === t.key
                    ? 'bg-secondary text-foreground'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                {t.label}
              </button>
            ))}
          </div>
        )}

        {activeTab === 'models' ? (
          <div className="space-y-3">
            <div className="relative">
              <Search className="size-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground pointer-events-none" />
              <input
                type="search"
                value={modelQuery}
                onChange={(e) => setModelQuery(e.target.value)}
                placeholder="Search models…"
                className="w-full h-9 pl-8 pr-3 bg-input text-foreground border border-border rounded-md text-xs transition-colors focus-visible:outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring"
              />
            </div>

            <div className="space-y-3 max-h-72 overflow-y-auto pr-0.5">
              {noneRow(!modelQuery || matches(modelQuery, 'none default'))}
              {modelGroups.map((g) => (
                <div key={g.provider} className="space-y-1.5">
                  <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground block">
                    {g.provider}
                  </span>
                  <div className="space-y-1.5">
                    {g.models.map((m) => (
                      <button
                        key={m}
                        type="button"
                        onClick={() => choose(m)}
                        className={rowClass(value === m)}
                      >
                        <span className="truncate">{m.slice(m.indexOf(':') + 1) || m}</span>
                        {value === m && <Check className="size-3.5 text-accent-foreground shrink-0" />}
                      </button>
                    ))}
                  </div>
                </div>
              ))}
              {modelQuery && modelGroups.length === 0 && !(!required && matches(modelQuery, 'none default')) && (
                <p className="text-xs text-muted-foreground py-4 text-center">
                  No models match "{modelQuery}".
                </p>
              )}
            </div>
          </div>
        ) : (
          <div className="space-y-3">
            <div className="relative">
              <Search className="size-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground pointer-events-none" />
              <input
                type="search"
                value={comboQuery}
                onChange={(e) => setComboQuery(e.target.value)}
                placeholder="Search combos…"
                className="w-full h-9 pl-8 pr-3 bg-input text-foreground border border-border rounded-md text-xs transition-colors focus-visible:outline-none focus-visible:border-ring focus-visible:ring-1 focus-visible:ring-ring"
              />
            </div>

            <div className="space-y-1.5 max-h-72 overflow-y-auto pr-0.5">
              {noneRow(!comboQuery || matches(comboQuery, 'none default'))}
              {filteredCombos.map((c) => (
                <button
                  key={c}
                  type="button"
                  onClick={() => choose(`combo:${c}`)}
                  className={rowClass(value === `combo:${c}`)}
                >
                  <span className="truncate">{c}</span>
                  {value === `combo:${c}` && (
                    <Check className="size-3.5 text-accent-foreground shrink-0" />
                  )}
                </button>
              ))}
              {comboQuery && filteredCombos.length === 0 && (
                <p className="text-xs text-muted-foreground py-4 text-center">
                  No combos match "{comboQuery}".
                </p>
              )}
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
};

/** The closed control that opens a ModelPickerDialog for one slot. */
export const ModelPickerTrigger: React.FC<{
  value: string;
  onClick: () => void;
  required: boolean;
}> = ({ value, onClick, required }) => (
  <button
    type="button"
    onClick={onClick}
    className="w-full h-9 flex items-center justify-between gap-2 pl-3 pr-2.5 bg-input text-foreground border border-border rounded-md text-xs transition-colors hover:border-muted-foreground/40 cursor-pointer"
  >
    <span className={`truncate font-mono ${value ? '' : 'text-muted-foreground'}`}>
      {value || (required ? 'Select a model…' : NONE_LABEL)}
    </span>
    <ChevronDown className="size-3.5 text-muted-foreground shrink-0" />
  </button>
);
