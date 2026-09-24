import React from 'react';
import { Loader2, Check, AlertCircle } from 'lucide-react';

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'outline' | 'ghost' | 'destructive';
  size?: 'sm' | 'md' | 'lg' | 'icon';
  loading?: boolean;
  state?: 'default' | 'loading' | 'error' | 'success';
  fullWidth?: boolean;
}

export const Button: React.FC<ButtonProps> = ({
  children,
  variant = 'primary',
  size = 'md',
  loading = false,
  state = 'default',
  fullWidth = false,
  disabled,
  className = '',
  ...props
}) => {
  const currentState = loading ? 'loading' : state;

  const baseStyles =
    'inline-flex items-center justify-center font-medium font-sans rounded-md transition-colors duration-120 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none select-none focus-visible:outline-2 focus-visible:outline-offset-2 active:scale-[0.98]';

  const variants = {
    primary:
      'bg-primary text-primary-foreground hover:bg-primary-hover active:bg-primary-active focus-visible:outline-ring',
    secondary:
      'bg-secondary text-secondary-foreground hover:bg-secondary-hover focus-visible:outline-ring',
    outline:
      'bg-transparent text-foreground hover:bg-secondary border border-border focus-visible:outline-ring',
    ghost:
      'bg-transparent text-muted-foreground hover:text-foreground hover:bg-secondary focus-visible:outline-ring',
    destructive:
      'bg-destructive text-destructive-foreground hover:bg-destructive-hover focus-visible:outline-ring',
  };

  const sizes = {
    sm: 'h-8 px-3 text-xs gap-1.5',
    md: 'h-9 px-4 text-sm gap-2',
    lg: 'h-10 px-5 text-base gap-2.5',
    icon: 'size-9 p-0 text-sm',
  };

  return (
    <button
      disabled={disabled || currentState === 'loading'}
      data-state={currentState}
      className={`${baseStyles} ${variants[variant]} ${sizes[size]} ${fullWidth ? 'w-full' : ''} ${className}`}
      {...props}
    >
      {currentState === 'loading' && <Loader2 className="size-4 animate-spin shrink-0" />}
      {currentState === 'success' && <Check className="size-4 text-success shrink-0" />}
      {currentState === 'error' && <AlertCircle className="size-4 text-destructive-text shrink-0" />}
      {children}
    </button>
  );
};
