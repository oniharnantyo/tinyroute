import React from 'react';
import { LucideIcon } from 'lucide-react';

interface Props {
  title: string;
  value: string | number;
  description?: string;
  icon: LucideIcon;
  tooltip?: string;
}

export const KpiCard: React.FC<Props> = ({ title, value, description, icon: Icon, tooltip }) => {
  return (
    <div
      title={tooltip}
      className="bg-card border border-border rounded-lg p-5 flex flex-col justify-between gap-3 hover:border-muted-foreground/40 transition-colors"
    >
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">{title}</span>
        <div className="p-2 bg-muted/50 border border-border-subtle rounded-md">
          <Icon className="size-4 text-muted-foreground" />
        </div>
      </div>
      <div>
        <div className="text-2xl font-mono font-semibold tracking-tight text-foreground">{value}</div>
        {description && <p className="text-xs text-muted-foreground mt-1">{description}</p>}
      </div>
    </div>
  );
};
