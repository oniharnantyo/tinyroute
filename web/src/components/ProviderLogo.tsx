import React, { useState } from 'react';

interface Props {
  name: string;
  logoName?: string;
  className?: string;
  alt?: string;
}

const LOGO_MAP: Record<string, string> = {
  anthropic: 'anthropic-dark.svg',
  claude: 'claude.svg',
  'claude-code': 'claude.svg',
  claudecode: 'claude.svg',
  cursor: 'anthropic-dark.svg',
  windsurf: 'codebuddy-intl.svg',
  continue: 'openai-compatible.svg',
  aider: 'openai-compatible.svg',
  openai: 'openai.svg',
  'openai-compatible': 'openai-compatible.svg',
  gemini: 'gemini.svg',
  'gemini-cli': 'gemini-cli.svg',
  vertex: 'vertex.svg',
  groq: 'groq.svg',
  deepseek: 'deepseek.svg',
  openrouter: 'openrouter.svg',
  cloudflare: 'cloudflare.svg',
  'cloudflare-ai': 'cloudflare-ai.svg',
  ollama: 'ollama.svg',
  'ollama-local': 'ollama-local.svg',
  minimax: 'minimax.svg',
  kimi: 'kimi.svg',
  xai: 'xai.svg',
  'xai-grok-dark': 'xai-grok-dark.svg',
  grok: 'grok.svg',
  'grok-cli': 'grok-cli.svg',
  nvidia: 'nvidia.svg',
  byteplus: 'byteplus.svg',
  antigravity: 'antigravity.svg',
  cline: 'cline-mono.svg',
  'cline-mono': 'cline-mono.svg',
  codex: 'codex-dark.svg',
  copilot: 'copilot.svg',
  devin: 'antigravity.svg',
  droid: 'gemini.svg',
  opencode: 'opencode.svg',
  'opencode-zen': 'opencode-zen.svg',
  trae: 'trae.svg',
  zed: 'zed.svg',
  github: 'github-dark.svg',
  gitlab: 'gitlab.svg',
  cerebras: 'groq.svg',
  cohere: 'openai.svg',
  mistral: 'deepseek.svg',
};

export const ProviderLogo: React.FC<Props> = ({ name, logoName, className = 'size-7', alt }) => {
  const [error, setError] = useState(false);
  const target = (logoName || name || '').toLowerCase().trim();
  const file = LOGO_MAP[target] || `${target}.svg`;

  if (error || !target) {
    return (
      <div
        className={`${className} bg-muted/80 border border-border rounded-lg text-primary font-bold text-xs flex items-center justify-center uppercase font-mono shrink-0 select-none shadow-xs`}
      >
        {name ? name.slice(0, 2) : 'TR'}
      </div>
    );
  }

  return (
    <div className={`${className} flex items-center justify-center shrink-0`}>
      <img
        src={`/dashboard/logos/${file}`}
        alt={alt || name}
        className="w-full h-full object-contain"
        onError={() => setError(true)}
      />
    </div>
  );
};
