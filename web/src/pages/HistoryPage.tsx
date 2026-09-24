import React, { useState, useEffect } from 'react';
import {
  History as HistoryIcon,
  Search,
  RotateCcw,
  Copy,
  Check,
  ChevronRight,
  Filter,
  Code2,
  FileText,
  ArrowLeftRight,
  ServerCog,
  X,
  ChevronDown,
} from 'lucide-react';
import { api } from '@/lib/api';
import { HistoryRecord, KeyItem, ProviderItem } from '@/types/api';
import { StatusBadge } from '@/components/StatusBadge';
import { Button } from '@/components/Button';
import { Input } from '@/components/Input';
import { Select } from '@/components/Select';
import { ProviderLogo } from '@/components/ProviderLogo';

const PAGE_SIZE = 50;
const MAX_ROWS = 500; // server-side window bound (history.MaxListLimit)

function formatTokens(n: number): string {
  if (!n) return '0';
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k';
  return n.toLocaleString();
}

function formatBytes(body: string): string {
  const bytes = new TextEncoder().encode(body).length;
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${bytes} B`;
}

// A collapsible payload pane: collapsed by default, size always visible,
// truncation noticed (spec: request detail exposes captured bodies).
const BodyPane: React.FC<{
  title: string;
  icon: React.ReactNode;
  body?: string;
  truncated?: boolean;
  onCopy: () => void;
  copied: boolean;
}> = ({ title, icon, body, truncated, onCopy, copied }) => {
  const [open, setOpen] = useState(false);
  if (!body) return null;

  return (
    <div className="border border-border-subtle rounded-md overflow-hidden">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="w-full flex items-center justify-between gap-3 px-3 py-2.5 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors cursor-pointer bg-muted/40"
        aria-expanded={open}
      >
        <span className="flex items-center gap-1.5">
          {icon}
          {title}
        </span>
        <span className="flex items-center gap-2.5 font-mono text-[10px]">
          <span>{formatBytes(body)}</span>
          {truncated && <span className="text-warning">truncated</span>}
          <ChevronDown className={`size-3.5 transition-transform ${open ? 'rotate-180' : ''}`} />
        </span>
      </button>
      {open && (
        <div className="relative border-t border-border-subtle">
          <button
            type="button"
            onClick={onCopy}
            className="absolute right-2 top-2 text-[10px] text-accent-foreground hover:underline flex items-center gap-1 cursor-pointer bg-card border border-border rounded px-1.5 py-0.5"
          >
            {copied ? <Check className="size-3 text-success" /> : <Copy className="size-3" />}
            <span>{copied ? 'Copied' : 'Copy'}</span>
          </button>
          <pre className="p-3 bg-background text-xs font-mono overflow-x-auto text-foreground max-h-64 overflow-y-auto">
            {body}
          </pre>
        </div>
      )}
    </div>
  );
};

export const HistoryPage: React.FC = () => {
  const [records, setRecords] = useState<HistoryRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [rowsShown, setRowsShown] = useState(PAGE_SIZE);
  const [filterOptions, setFilterOptions] = useState<{ providers: ProviderItem[]; keys: KeyItem[] }>({
    providers: [],
    keys: [],
  });

  // Filters
  const [filterProvider, setFilterProvider] = useState('');
  const [filterKey, setFilterKey] = useState('');
  const [filterSearch, setFilterSearch] = useState('');
  const [filterFrom, setFilterFrom] = useState('');
  const [filterTo, setFilterTo] = useState('');
  const [quickFilter, setQuickFilter] = useState<'all' | 'errors' | 'ratelimits' | 'slow'>('all');

  // Inspection drawer
  const [selectedRecordId, setSelectedRecordId] = useState<string | null>(null);
  const [recordDetail, setRecordDetail] = useState<HistoryRecord | null>(null);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [copiedSection, setCopiedSection] = useState<string | null>(null);

  const fetchHistory = async (limit: number) => {
    try {
      setLoading(true);
      const res = await api.getHistory({
        provider: filterProvider || undefined,
        key: filterKey || undefined,
        search: filterSearch || undefined,
        from: filterFrom || undefined,
        to: filterTo || undefined,
        limit,
      });
      setRecords(res.records || []);
    } catch (err) {
      console.error('Failed to load history', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    // Filter options come from live state: topology providers and known keys.
    api.getProviders().then((r) => setFilterOptions((p) => ({ ...p, providers: r.providers || [] }))).catch(() => {});
    api.getKeys().then((r) => setFilterOptions((p) => ({ ...p, keys: r.keys || [] }))).catch(() => {});
  }, []);

  useEffect(() => {
    fetchHistory(rowsShown);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filterProvider, filterKey, filterFrom, filterTo]);

  // Close the drawer on Escape
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setSelectedRecordId(null);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const handleSearchSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setRowsShown(PAGE_SIZE);
    fetchHistory(PAGE_SIZE);
  };

  const handleResetFilters = () => {
    setFilterProvider('');
    setFilterKey('');
    setFilterSearch('');
    setFilterFrom('');
    setFilterTo('');
    setQuickFilter('all');
    setRowsShown(PAGE_SIZE);
  };

  const handleLoadMore = () => {
    const next = Math.min(rowsShown + PAGE_SIZE, MAX_ROWS);
    setRowsShown(next);
    fetchHistory(next);
  };

  const handleOpenDetail = async (id: string) => {
    setSelectedRecordId(id);
    setDetailError(null);
    setRecordDetail(null);
    try {
      const detail = await api.getHistoryDetail(id);
      setRecordDetail(detail);
    } catch (err: any) {
      setDetailError(err.message || 'Record not found');
    }
  };

  const handleCopy = (key: string, text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedSection(key);
    setTimeout(() => setCopiedSection(null), 2000);
  };

  const filteredRecords = records.filter((r) => {
    if (quickFilter === 'errors') return r.status_code >= 400;
    if (quickFilter === 'ratelimits') return r.status_code === 429;
    if (quickFilter === 'slow') return r.latency_ms > 1500;
    return true;
  });

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground flex items-center gap-2.5">
            <span>History</span>
            <span className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-muted border border-border text-muted-foreground">
              {filteredRecords.length} shown
            </span>
          </h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            Every proxied request — latency, tokens, attempt chain, payloads
          </p>
        </div>
      </div>

      {/* Quick filters & search */}
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
          {/* Quick filter pills */}
          <div className="inline-flex items-center bg-card border border-border rounded-lg p-0.5 overflow-x-auto max-w-full">
            {[
              { key: 'all', label: 'All' },
              { key: 'errors', label: 'Errors' },
              { key: 'ratelimits', label: 'Rate limits' },
              { key: 'slow', label: 'Slow (> 1.5s)' },
            ].map((qf) => (
              <button
                key={qf.key}
                onClick={() => setQuickFilter(qf.key as any)}
                className={`text-xs px-3 py-1.5 rounded-md transition-colors font-medium cursor-pointer whitespace-nowrap ${
                  quickFilter === qf.key
                    ? 'bg-secondary text-foreground'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
              >
                {qf.label}
              </button>
            ))}
          </div>

          <Button
            variant="ghost"
            size="sm"
            onClick={handleResetFilters}
            className="text-xs gap-1.5"
          >
            <RotateCcw className="size-3.5" />
            <span>Reset filters</span>
          </Button>
        </div>

        {/* Filters */}
        <form onSubmit={handleSearchSubmit} className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <Select
            label="Provider"
            value={filterProvider}
            onChange={(e) => setFilterProvider(e.target.value)}
            className="text-xs"
            options={[
              { value: '', label: 'All providers' },
              ...filterOptions.providers.map((p) => ({ value: p.name, label: p.display_name || p.name })),
            ]}
          />

          <Select
            label="Key"
            value={filterKey}
            onChange={(e) => setFilterKey(e.target.value)}
            className="text-xs"
            options={[
              { value: '', label: 'All keys' },
              ...filterOptions.keys.map((k) => ({ value: k.id, label: `${k.name} (${k.id})` })),
            ]}
          />

          <div className="relative">
            <Search className="size-3.5 absolute left-3 top-[26px] -translate-y-1/2 text-muted-foreground pointer-events-none" />
            <Input
              label="Session or model"
              value={filterSearch}
              onChange={(e) => setFilterSearch(e.target.value)}
              placeholder="Search session or model…"
              className="pl-8 text-xs"
            />
          </div>

          <div className="flex gap-2">
            <Input
              label="From"
              type="date"
              value={filterFrom}
              onChange={(e) => setFilterFrom(e.target.value)}
              className="text-xs"
              aria-label="From date"
            />
            <Input
              label="To"
              type="date"
              value={filterTo}
              onChange={(e) => setFilterTo(e.target.value)}
              className="text-xs"
              aria-label="To date"
            />
          </div>

          <div className="flex items-end">
            <Button type="submit" size="sm" variant="secondary" className="gap-1.5 w-full sm:w-auto">
              <Filter className="size-3.5" />
              <span>Apply filter</span>
            </Button>
          </div>
        </form>
      </div>

      {/* Table */}
      <div className="bg-card border border-border rounded-lg overflow-hidden">
        {filteredRecords.length === 0 && !loading ? (
          <div className="py-16 text-center text-xs text-muted-foreground p-8 space-y-3">
            <div className="size-11 rounded-lg bg-muted border border-border flex items-center justify-center mx-auto">
              <HistoryIcon className="size-5 text-muted-foreground" />
            </div>
            <div>
              <p className="font-medium text-foreground">No requests found</p>
              <p className="mt-0.5">Try widening the search or date range.</p>
            </div>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="border-b border-border text-muted-foreground bg-muted/30">
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Time</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Provider</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Model</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Status</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Latency</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px]">Tokens</th>
                  <th className="py-3 px-4 font-medium uppercase tracking-wider text-[11px] text-right">Inspect</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border-subtle">
                {filteredRecords.map((r) => (
                  <tr
                    key={r.id}
                    onClick={() => handleOpenDetail(r.id)}
                    className="hover:bg-secondary/40 transition-colors group cursor-pointer"
                  >
                    <td className="py-3 px-4 font-mono text-muted-foreground">{r.time_formatted}</td>
                    <td className="py-3 px-4">
                      <div className="flex items-center gap-2">
                        <ProviderLogo name={r.provider} className="size-4 shrink-0" />
                        <span className="font-medium text-foreground">{r.provider || '—'}</span>
                      </div>
                    </td>
                    <td className="py-3 px-4 font-mono text-foreground truncate max-w-xs">{r.model_requested}</td>
                    <td className="py-3 px-4">
                      <StatusBadge variant={r.status_variant as any}>
                        {r.status_code || r.outcome}
                      </StatusBadge>
                    </td>
                    <td className="py-3 px-4 font-mono text-foreground/80">{r.latency_ms}ms</td>
                    <td className="py-3 px-4 font-mono text-muted-foreground">
                      {formatTokens(r.input_tokens)} in / {formatTokens(r.output_tokens)} out
                    </td>
                    <td className="py-3 px-4 text-right">
                      <ChevronRight className="size-4 text-muted-foreground group-hover:text-accent-foreground transition-colors inline" />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Load More: the window grows within the same filter set */}
        {records.length >= rowsShown && rowsShown < MAX_ROWS && (
          <div className="border-t border-border p-3 flex items-center justify-center">
            <Button variant="outline" size="sm" onClick={handleLoadMore} disabled={loading} className="text-xs">
              {loading ? 'Loading…' : `Load more (${records.length} shown)`}
            </Button>
          </div>
        )}
      </div>

      {/* Inspection drawer */}
      {selectedRecordId && (
        <div
          className="fixed inset-0 z-50 bg-[var(--scrim)] animate-backdrop-in flex justify-end"
          onClick={() => setSelectedRecordId(null)}
        >
          <div
            className="w-full max-w-2xl bg-card border-l border-border h-full flex flex-col shadow-2xl p-6 overflow-y-auto space-y-6 animate-drawer-in"
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-label="Request inspector"
          >
            {/* Drawer header */}
            <div className="flex items-center justify-between pb-4 border-b border-border">
              <div className="flex items-center gap-3">
                <ProviderLogo name={recordDetail?.provider || ''} className="size-7" />
                <div>
                  <h3 className="font-semibold text-base text-foreground flex items-center gap-2">
                    <span>Request</span>
                    {recordDetail && (
                      <StatusBadge variant={recordDetail.status_variant as any}>
                        HTTP {recordDetail.status_code}
                      </StatusBadge>
                    )}
                  </h3>
                  <span className="text-xs font-mono text-muted-foreground">{selectedRecordId}</span>
                </div>
              </div>
              <button
                onClick={() => setSelectedRecordId(null)}
                className="p-1.5 rounded-md text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
                aria-label="Close inspector"
              >
                <X className="size-4" />
              </button>
            </div>

            {detailError && (
              <div className="p-4 border border-border rounded-md text-xs text-muted-foreground">
                {detailError} —{' '}
                <button onClick={() => setSelectedRecordId(null)} className="text-accent-foreground hover:underline cursor-pointer">
                  back to the history list
                </button>
              </div>
            )}

            {recordDetail && (
              <>
                {/* Metrics */}
                <div className="grid grid-cols-3 gap-3">
                  <div className="p-3 bg-muted/40 border border-border-subtle rounded-md">
                    <span className="text-[10px] uppercase tracking-wider text-muted-foreground block">Latency</span>
                    <span className="font-mono font-semibold text-foreground text-sm">{recordDetail.latency_ms}ms</span>
                  </div>
                  <div className="p-3 bg-muted/40 border border-border-subtle rounded-md">
                    <span className="text-[10px] uppercase tracking-wider text-muted-foreground block">Input tokens</span>
                    <span className="font-mono font-semibold text-foreground text-sm">{formatTokens(recordDetail.input_tokens)}</span>
                  </div>
                  <div className="p-3 bg-muted/40 border border-border-subtle rounded-md">
                    <span className="text-[10px] uppercase tracking-wider text-muted-foreground block">Output tokens</span>
                    <span className="font-mono font-semibold text-foreground text-sm">{formatTokens(recordDetail.output_tokens)}</span>
                  </div>
                </div>

                {/* Attempt chain */}
                {recordDetail.attempts && recordDetail.attempts.length > 0 && (
                  <div className="space-y-2">
                    <span className="text-xs font-medium text-muted-foreground block">
                      Attempts ({recordDetail.attempts.length})
                    </span>
                    <div className="space-y-2">
                      {recordDetail.attempts.map((att, idx) => (
                        <div
                          key={idx}
                          className="p-3 bg-muted/40 border border-border-subtle rounded-md text-xs font-mono flex items-center justify-between gap-3"
                        >
                          <div className="flex items-center gap-2 min-w-0">
                            <span className="size-5 rounded-md bg-muted border border-border text-muted-foreground font-semibold flex items-center justify-center text-[10px] shrink-0">
                              {idx + 1}
                            </span>
                            <ProviderLogo name={att.provider} className="size-4 shrink-0" />
                            <span className="font-medium text-foreground truncate">{att.provider}:{att.model}</span>
                          </div>
                          <div className="flex items-center gap-2 shrink-0">
                            <span className="text-muted-foreground">{att.elapsed_ms}ms</span>
                            <StatusBadge variant={att.status < 400 ? 'success' : 'error'}>
                              {att.status}
                            </StatusBadge>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Captured bodies — four panes, collapsed by default */}
                <div className="space-y-2">
                  <span className="text-xs font-medium text-muted-foreground block">Captured bodies</span>
                  <BodyPane
                    title="Client request"
                    icon={<Code2 className="size-3.5" />}
                    body={recordDetail.request_body}
                    truncated={recordDetail.request_body_truncated}
                    copied={copiedSection === 'req'}
                    onCopy={() => handleCopy('req', recordDetail.request_body || '')}
                  />
                  <BodyPane
                    title="Translated provider request"
                    icon={<ArrowLeftRight className="size-3.5" />}
                    body={recordDetail.translated_request_body}
                    truncated={recordDetail.translated_req_truncated}
                    copied={copiedSection === 'treq'}
                    onCopy={() => handleCopy('treq', recordDetail.translated_request_body || '')}
                  />
                  <BodyPane
                    title="Raw provider response"
                    icon={<ServerCog className="size-3.5" />}
                    body={recordDetail.raw_response_body}
                    truncated={recordDetail.raw_resp_truncated}
                    copied={copiedSection === 'rresp'}
                    onCopy={() => handleCopy('rresp', recordDetail.raw_response_body || '')}
                  />
                  <BodyPane
                    title="Final response to client"
                    icon={<FileText className="size-3.5" />}
                    body={recordDetail.response_body}
                    truncated={recordDetail.response_body_truncated}
                    copied={copiedSection === 'resp'}
                    onCopy={() => handleCopy('resp', recordDetail.response_body || '')}
                  />

                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
};
