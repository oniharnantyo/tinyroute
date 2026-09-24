import React from 'react';

interface Props {
  variant?: 'success' | 'warning' | 'error' | 'neutral';
  children: React.ReactNode;
  className?: string;
  title?: string;
}

export const StatusBadge: React.FC<Props> = ({
  variant = 'neutral',
  children,
  className = '',
  title,
}) => {
  const styles = {
    success: 'bg-success/10 text-success border-success/30',
    warning: 'bg-warning/10 text-warning border-warning/30',
    error: 'bg-destructive/10 text-destructive-text border-destructive/30',
    neutral: 'bg-muted text-muted-foreground border-border',
  };

  const dots = {
    success: 'bg-success',
    warning: 'bg-warning',
    error: 'bg-destructive-text',
    neutral: 'bg-muted-foreground',
  };

  return (
    <span
      title={title}
      className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-[11px] font-mono border ${styles[variant]} ${className}`}
    >
      <span className={`size-1.5 rounded-full ${dots[variant]}`} />
      {children}
    </span>
  );
};
