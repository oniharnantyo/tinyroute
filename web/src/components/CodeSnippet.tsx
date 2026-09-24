import React, { useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { Button } from './Button';

interface Props {
  code: string;
  language?: string;
  title?: string;
  className?: string;
}

export const CodeSnippet: React.FC<Props> = ({ code, language = 'bash', title, className = '' }) => {
  const [copied, setCopied] = useState(false);

  const copyToClipboard = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // fallback
    }
  };

  return (
    <div className={`relative bg-muted/40 border border-border/80 rounded-md overflow-hidden font-mono text-xs ${className}`}>
      {title && (
        <div className="flex items-center justify-between px-3 py-1.5 border-b border-border/60 bg-muted/60 text-muted-foreground text-[11px]">
          <span>{title}</span>
          <span>{language}</span>
        </div>
      )}
      <div className="relative p-3 overflow-x-auto">
        <pre className="text-foreground/90 leading-relaxed whitespace-pre font-mono">
          <code>{code}</code>
        </pre>
        <div className="absolute top-2 right-2">
          <Button
            size="sm"
            variant="ghost"
            onClick={copyToClipboard}
            className="size-7 p-0 bg-card/80 hover:bg-card border border-border/60 text-muted-foreground hover:text-foreground"
            title="Copy to clipboard"
          >
            {copied ? <Check className="size-3.5 text-success" /> : <Copy className="size-3.5" />}
          </Button>
        </div>
      </div>
    </div>
  );
};
