import React, { useState, useEffect } from 'react';
import {
  Activity,
  Check,
  Cpu,
  Clock,
  ChartColumn,
  Layers,
  ChevronRight,
  Sparkles,
} from 'lucide-react';
import { api } from '@/lib/api';
import { OverviewData, TimeWindow } from '@/types/api';
import { VolumeChart } from '@/components/VolumeChart';
import { StatusBadge } from '@/components/StatusBadge';
import { KpiCard } from '@/components/KpiCard';

function formatCompact(n: number): string {
  if (!n) return '0';
  if (n >= 1_000_000_000) {
    return (n / 1_000_000_000).toFixed(1).replace(/\.0$/, '') + 'B';
  }
  if (n >= 1_000_000) {
    return (n / 1_000_000).toFixed(1).replace(/\.0$/, '') + 'M';
  }
  if (n >= 1_000) {
    return (n / 1_000).toFixed(1).replace(/\.0$/, '') + 'k';
  }
  return n.toLocaleString();
}

interface Props {
  onNavigateTab?: (tab: string, itemParam?: string) => void;
}

// The active window lives in the URL query (?window=1h|24h|7d|30d) so
// windowed URLs are shareable and restore on load (default 24h).
const VALID_WINDOWS: TimeWindow[] = ['1h', '24h', '7d', '30d'];

function initialWindow(): TimeWindow {
  const v = new URLSearchParams(window.location.search).get('window') as TimeWindow | null;
  return v && VALID_WINDOWS.includes(v) ? v : '24h';
}

export const OverviewPage: React.FC<Props> = ({ onNavigateTab }) => {
  const [windowKey, setWindowKey] = useState<TimeWindow>(initialWindow);
  const [data, setData] = useState<OverviewData | null>(null);

  const fetchOverview = async (w: TimeWindow) => {
    try {
      const res = await api.getOverview(w);
      setData(res);
    } catch (err) {
      console.error('Failed to load overview data', err);
    }
  };

  useEffect(() => {
    fetchOverview(windowKey);
  }, [windowKey]);

  // Auto-refresh so statistics stay current while the overview stays open
  // (spec: management-dashboard / overview auto-refreshes).
  useEffect(() => {
    const timer = setInterval(() => fetchOverview(windowKey), 30_000);
    return () => clearInterval(timer);
  }, [windowKey]);

  const windows: { key: TimeWindow; label: string }[] = [
    { key: '1h', label: '1 Hour' },
    { key: '24h', label: '24 Hours' },
    { key: '7d', label: '7 Days' },
    { key: '30d', label: '30 Days' },
  ];

  const overview = data || {
    active_window: windowKey,
    total_requests: 0,
    success_requests: 0,
    success_rate: 100,
    total_prompt_tokens: 0,
    total_completion_tokens: 0,
    avg_latency_ms: 0,
    provider_traffic_list: [],
    top_models: [],
    buckets: [],
    recent_sessions: [],
  };

  const totalTokens = overview.total_prompt_tokens + overview.total_completion_tokens;
  const maxModelTokens = Math.max(...(overview.top_models?.map((m) => m.total_tokens) || [1]), 1);

  return (
    <div className="space-y-6">
      {/* Page head & time window */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold tracking-tight text-foreground">Overview</h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            Traffic, provider health, and token usage
          </p>
        </div>

        {/* Window selector */}
        <div className="inline-flex items-center bg-card border border-border rounded-lg p-0.5 self-start">
          {windows.map((w) => (
            <button
              key={w.key}
              onClick={() => {
                setWindowKey(w.key);
                const url = new URL(window.location.href);
                url.searchParams.set('window', w.key);
                window.history.replaceState(null, '', url);
              }}
              className={`text-xs px-3 py-1.5 rounded-md transition-colors font-medium cursor-pointer ${
                windowKey === w.key
                  ? 'bg-secondary text-foreground'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {w.label}
            </button>
          ))}
        </div>
      </div>

      {/* KPI row */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <KpiCard
          title="Requests"
          icon={Activity}
          value={formatCompact(overview.total_requests)}
          description={`in the last ${overview.active_window}`}
        />
        <KpiCard
          title="Success rate"
          icon={Check}
          value={`${overview.success_rate.toFixed(1)}%`}
          description={`${formatCompact(overview.success_requests)} completed`}
        />
        <KpiCard
          title="Tokens"
          icon={Sparkles}
          value={formatCompact(totalTokens)}
          description={`${formatCompact(overview.total_prompt_tokens)} in / ${formatCompact(overview.total_completion_tokens)} out`}
        />
        <KpiCard
          title="Avg latency"
          icon={Clock}
          value={`${overview.avg_latency_ms} ms`}
          description="across upstream calls"
        />
      </div>

      {/* Request volume */}
      <div className="bg-card border border-border rounded-lg overflow-hidden">
        <div className="flex flex-row items-center justify-between border-b border-border p-5">
          <div>
            <h3 className="text-sm font-semibold flex items-center gap-2 text-foreground">
              <ChartColumn className="size-4 text-muted-foreground" />
              Request volume
            </h3>
            <p className="text-xs text-muted-foreground mt-0.5">
              Requests per bucket in the selected window
            </p>
          </div>
          <span className="text-xs font-mono px-2.5 py-1 rounded-md bg-muted border border-border text-muted-foreground">
            {overview.active_window}
          </span>
        </div>
        <div className="p-5">
          <VolumeChart buckets={overview.buckets} window={overview.active_window} />
        </div>
      </div>

      {/* Providers & top models */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Providers */}
        <div className="bg-card border border-border rounded-lg overflow-hidden flex flex-col">
          <div className="flex flex-row items-center justify-between border-b border-border p-5">
            <h3 className="text-sm font-semibold flex items-center gap-2 text-foreground">
              <Cpu className="size-4 text-muted-foreground" />
              Providers
            </h3>
            <span className="text-xs font-mono text-muted-foreground">
              {overview.provider_traffic_list?.length || 0} configured
            </span>
          </div>

          <div className="p-5 flex-1">
            {!overview.provider_traffic_list || overview.provider_traffic_list.length === 0 ? (
              <div className="py-12 text-center text-xs text-muted-foreground">
                No providers configured
              </div>
            ) : (
              <div className="space-y-2.5">
                {overview.provider_traffic_list.map((p) => (
                  <div
                    key={p.name}
                    onClick={() => onNavigateTab?.('providers', p.name)}
                    className="flex items-center justify-between p-3.5 bg-muted/40 border border-border-subtle hover:border-primary/50 hover:bg-card rounded-lg text-xs transition-colors group cursor-pointer"
                  >
                    <div className="space-y-0.5">
                      <div className="font-medium text-foreground flex items-center gap-1.5">
                        {p.name}
                        <ChevronRight className="size-3.5 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
                      </div>
                      <div className="text-[11px] font-mono text-muted-foreground">{p.dialect}</div>
                    </div>
                    <div className="flex items-center gap-3 text-right">
                      <div className="space-y-0.5 font-mono">
                        <div className="text-foreground font-medium">{formatCompact(p.total_requests)} reqs</div>
                        <div className="text-[11px] text-muted-foreground">
                          {p.total_requests > 0 ? `${p.success_rate.toFixed(1)}% ok` : '—'}
                        </div>
                      </div>
                      {p.status === 'healthy' ? (
                        <StatusBadge variant="success">Healthy</StatusBadge>
                      ) : (
                        <StatusBadge
                          variant="error"
                          title={p.cooldown_until ? `Cooldown until ${p.cooldown_until}` : 'In cooldown'}
                        >
                          Cooldown
                        </StatusBadge>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Top models */}
        <div className="bg-card border border-border rounded-lg overflow-hidden flex flex-col">
          <div className="flex flex-row items-center justify-between border-b border-border p-5">
            <h3 className="text-sm font-semibold flex items-center gap-2 text-foreground">
              <Layers className="size-4 text-muted-foreground" />
              Top models
            </h3>
            <span className="text-xs font-mono text-muted-foreground">
              {overview.top_models?.length || 0} models
            </span>
          </div>

          <div className="p-5 flex-1">
            {!overview.top_models || overview.top_models.length === 0 ? (
              <div className="py-12 text-center text-xs text-muted-foreground">
                No token usage recorded in this window
              </div>
            ) : (
              <div className="space-y-3">
                {overview.top_models.map((m) => {
                  const pct = Math.min(Math.round((m.total_tokens / maxModelTokens) * 100), 100);
                  return (
                    <div key={m.model} className="p-3 bg-muted/40 border border-border-subtle rounded-lg space-y-2">
                      <div className="flex items-center justify-between text-xs font-mono">
                        <span className="font-medium text-foreground truncate">{m.model}</span>
                        <span className="text-accent-foreground font-semibold">{formatCompact(m.total_tokens)} tokens</span>
                      </div>
                      {/* Token volume bar */}
                      <div className="w-full bg-border-subtle h-1.5 rounded-full overflow-hidden">
                        <div
                          style={{ width: `${pct}%` }}
                          className="bg-primary h-full rounded-full"
                        />
                      </div>
                      <div className="flex justify-between text-[10px] font-mono text-muted-foreground">
                        <span>In: {formatCompact(m.input_tokens)}</span>
                        <span>Out: {formatCompact(m.output_tokens)}</span>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
