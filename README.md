<div align="center">

# tinyroute

A tiny multi-provider LLM proxy

[![License](https://img.shields.io/github/license/oniharnantyo/tinyroute?style=for-the-badge)](https://github.com/oniharnantyo/tinyroute/blob/main/LICENSE)
[![Release](https://img.shields.io/github/v/release/oniharnantyo/tinyroute?style=for-the-badge)](https://github.com/oniharnantyo/tinyroute/releases)
[![Stars](https://img.shields.io/github/stars/oniharnantyo/tinyroute?style=for-the-badge)](https://github.com/oniharnantyo/tinyroute/stargazers)

</div>

## What is this?

tinyroute is a lightweight HTTP proxy that sits between LLM clients (like the Anthropic or OpenAI SDKs) and multiple LLM providers. It handles routing, authentication, rate limiting, and session history with minimal dependencies and configuration.

**Key features:**
- **Multi-provider routing** — Route requests to Anthropic, OpenAI, or compatible providers with automatic fallback chains
- **Hot-reload configuration** — Change routing rules and providers without restarting
- **API key management** — Generate and revoke internal API keys with rate limits and expiration
- **Session tracking** — Record request history with optional full-capture of request/response bodies
- **Health tracking** — Automatic cooldowns for failing providers (5xx errors, 429 rate limits)
- **Provider presets** — Built-in templates for popular providers (Anthropic, OpenAI, Azure, etc.)

## Quick Start

### Install

```bash
go install github.com/oniharnantyo/tinyroute@latest
```

Or build locally:

```bash
git clone https://github.com/oniharnantyo/tinyroute.git
cd tinyroute
go build -o tinyroute .
```

### Initialize

```bash
tinyroute init
```

This creates `~/.tinyroute/` with:
- `.env` — Environment variables (API keys, listen address)
- `config.yaml` — Provider and route configuration
- `keys.json` — Internal API keys (first key auto-generated)

### Configure providers

```bash
# List available presets
tinyroute add --list

# Add a provider (e.g., Anthropic)
tinyroute add anthropic

# Set credentials via stdin
echo "sk-ant-..." | tinyroute auth set anthropic
```

Or set credentials in `~/.tinyroute/.env`:

```bash
ANTHROPIC_API_KEY=sk-ant-...
OPENAI_API_KEY=sk-...
```

### Start the server

```bash
tinyroute serve
```

The proxy listens on `http://127.0.0.1:8787` by default.

### Use with clients

```bash
# Point Anthropic SDK to the proxy
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
export ANTHROPIC_AUTH_TOKEN=<your-internal-key-from-init>

# Or for OpenAI SDK
export OPENAI_BASE_URL=http://127.0.0.1:8787
export OPENAI_API_KEY=<your-internal-key>
```

## Project Structure

```
.
├── internal/
│   ├── accesslog/
│   ├── agent/
│   ├── auth/
│   ├── cli/
│   ├── config/
│   ├── core/
│   ├── credential/
│   ├── dialect/
│   ├── history/
│   ├── preset/
│   ├── proxy/
│   ├── route/
│   └── translate/
├── openspec/
│   ├── changes/
│   └── specs/
├── docs/
├── main.go
├── go.mod
├── go.sum
├── LICENSE
├── CLAUDE.md
├── README.md
└── tinyroute          # Built binary
```

## Documentation

| Resource | Description |
|----------|-------------|
| [Architecture](docs/ARCHITECTURE.md) | Internal design, data flow, and extension points |
| [Core Routing Spec](openspec/specs/core-routing/spec.md) | Request routing and provider chain design |
| [Provider Registry Spec](openspec/specs/provider-registry/spec.md) | Provider registration and model management |
| [Session History Spec](openspec/specs/session-history/spec.md) | Request logging and blob storage design |
| [API Keys Spec](openspec/specs/api-keys/spec.md) | Key management and rate limiting |
| [Agent Install Spec](openspec/specs/agent-install/spec.md) | Coding agent configuration adapters |

## Contributing

Contributions are welcome! Please read the [architecture docs](docs/ARCHITECTURE.md) for internal design and extension points.

<a href="https://github.com/oniharnantyo/tinyroute/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=oniharnantyo/tinyroute" />
</a>

## License

MIT

---

<div align="center">

[![Star History Chart](https://api.star-history.com/svg?repos=oniharnantyo/tinyroute&type=Date)](https://star-history.com/#oniharnantyo/tinyroute&Date)

</div>