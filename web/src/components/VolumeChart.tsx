import React from 'react';
import { TrafficBucket } from '@/types/api';

interface Props {
  buckets: TrafficBucket[];
  window: string;
}

export const VolumeChart: React.FC<Props> = ({ buckets, window }) => {
  if (!buckets || buckets.length === 0) {
    return (
      <div className="py-12 text-center text-xs text-muted-foreground font-mono">
        No request traffic recorded in {window} window
      </div>
    );
  }

  const maxVal = Math.max(...buckets.map((b) => b.count), 1);

  return (
    <div className="space-y-3">
      <div className="h-[220px] flex items-end gap-1.5 pt-6 pb-2 px-1">
        {buckets.map((b, i) => {
          const heightPct = Math.max((b.count / maxVal) * 100, 3);
          return (
            <div
              key={i}
              className="flex-1 min-w-0 flex flex-col items-center h-full justify-end group relative"
            >
              {/* Tooltip */}
              <div className="opacity-0 group-hover:opacity-100 transition-opacity absolute -top-8 bg-popover text-popover-foreground border border-border rounded-md px-2 py-1 text-[10px] font-mono whitespace-nowrap pointer-events-none z-20">
                {b.count} reqs ({b.time_label || new Date(b.timestamp).toLocaleTimeString()})
              </div>

              {/* Bar */}
              <div
                style={{ height: `${heightPct}%` }}
                className="w-full bg-primary/75 group-hover:bg-primary rounded-t-sm transition-colors duration-120"
              />
            </div>
          );
        })}
      </div>

      {/* Axis labels */}
      <div className="flex justify-between text-[10px] font-mono text-muted-foreground border-t border-border-subtle pt-2 px-1">
        <span>{buckets[0]?.time_label || 'Start'}</span>
        <span className="text-center">Window: {window}</span>
        <span>{buckets[buckets.length - 1]?.time_label || 'Now'}</span>
      </div>
    </div>
  );
};
