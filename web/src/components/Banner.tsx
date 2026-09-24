import React from 'react';
import { Check, AlertCircle, TriangleAlert, X } from 'lucide-react';

interface Props {
  variant: 'success' | 'error' | 'warning';
  children: React.ReactNode;
  onDismiss?: () => void;
  className?: string;
}

/** Inline feedback banner — the app-wide replacement for alert(). */
export const Banner: React.FC<Props> = ({ variant, children, onDismiss, className = '' }) => {
  const styles = {
    success: 'bg-success/10 border-success/30 text-success',
    error: 'bg-destructive/10 border-destructive/30 text-destructive-text',
    warning: 'bg-warning/10 border-warning/30 text-warning',
  };

  const icons = {
    success: Check,
    error: AlertCircle,
    warning: TriangleAlert,
  };

  const Icon = icons[variant];

  return (
    <div
      role={variant === 'error' ? 'alert' : 'status'}
      className={`rounded-md border px-3.5 py-3 text-xs flex items-center gap-2.5 ${styles[variant]} ${className}`}
    >
      <Icon className="size-4 shrink-0" />
      <div className="flex-1">{children}</div>
      {onDismiss && (
        <button
          type="button"
          onClick={onDismiss}
          className="shrink-0 opacity-70 hover:opacity-100 cursor-pointer"
          aria-label="Dismiss"
        >
          <X className="size-3.5" />
        </button>
      )}
    </div>
  );
};
