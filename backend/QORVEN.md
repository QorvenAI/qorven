# QORVEN.md — System Knowledge for AI Agents

> This document is injected into Prime's context so it understands the system it supervises.
> Other Qors receive relevant sections based on their role.

## Architecture

```
Backend: Go (internal/) → port 4200
Frontend: Next.js (qorven-web/) → port 3000
Database: PostgreSQL → port 5432
LLM Proxy: LiteLLM → port 4100 (routes to AWS Bedrock)
STT: Whisper small → port 8881
TTS: Edge TTS (Microsoft neural, free)
```

## Backend Package Map

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `agent` | Core agent loop (ReAct), context building, hooks | `loop.go`, `types.go`, `context.go` |
| `providers` | LLM provider registry, OpenAI-compatible driver | `openai.go`, `registry.go`, `smart_router.go` |
| `tools` | 38 tools (file, exec, web, memory, etc.) | `filesystem.go`, `exec.go`, `web.go`, `registry.go` |
| `gateway` | HTTP server, all API routes, middleware | `gateway.go`, `handlers.go`, `middleware.go` |
| `memory` | Memory hierarchy, extraction, knowledge graph | `store.go`, `hierarchy.go`, `certainty.go` |
| `supervisor` | Inter-agent protocol, Prime's audit loop | `bus.go`, `supervisor.go`, `autofix.go` |
| `council` | Multi-model consensus (LLM Council) | `council.go`, `depth.go` |
| `voice` | STT/TTS providers, realtime voice | `providers.go`, `realtime.go`, `format.go` |
| `search` | Multi-provider web search pipeline | `pipeline.go`, `extract.go` |
| `channels` | Telegram, Slack, Discord, WhatsApp, etc. | `channel.go`, `health.go` |
| `skills` | SKILL.md loader, security guard | `loader.go`, `skill_loader.go` |
| `session` | Conversation session management | `session.go` |
| `config` | TOML config loading | `config.go` |
| `cron` | Scheduled job runner | `runner.go`, `scheduler.go` |

## Config Format (config.toml)

```toml
[server]
listen = "0.0.0.0:4200"

[database]
dsn = "postgres://user:pass@localhost:5432/dbname?sslmode=disable"

[auth]
token = "your-api-token"
encryption_key = "64-hex-chars"

# Add LLM providers:
[[providers]]
name = "bedrock"
type = "openai_compat"
api_base = "http://localhost:4100/v1"
api_key = "sk-local"

# Add voice providers:
[[providers]]
name = "elevenlabs"
type = "elevenlabs"
api_key = "your-key"

[[providers]]
name = "openai"
type = "openai_compat"
api_base = "https://api.openai.com/v1"
api_key = "sk-..."
```

## How to Add a New LLM Provider

1. Add to `config.toml` under `[[providers]]`
2. Restart: `sudo systemctl restart qorven`
3. The provider auto-registers in the registry
4. Assign to work categories via `POST /v1/routing/assign`

## How to Add a New Frontend Page

1. Create `app/(app)/your-page/page.tsx`
2. Use `'use client'` directive
3. Fetch data via `/api/v1/...` (proxied to backend)
4. Use existing patterns: `cn()` for classes, lucide-react for icons
5. Dark mode: use `bg-zinc-*`, `text-zinc-*`, `border-zinc-*`
6. The page auto-appears in Next.js routing

## How to Add a New Tool

1. Create a struct implementing the `Tool` interface in `internal/tools/`
2. Methods: `Name()`, `Description()`, `Parameters()`, `Execute(ctx, args)`
3. Register in `internal/gateway/gateway.go` → `registerTools()`
4. The tool auto-appears in the agent's tool list

## How to Add a New Channel

1. Create package in `internal/channels/your_channel/`
2. Implement the `Channel` interface: `Type()`, `Start()`, `Stop()`, `Send()`
3. Register in channel manager
4. Add webhook route if needed

## Available Models (via Bedrock/LiteLLM)

| Model | Strengths | Use For |
|-------|-----------|---------|
| deepseek-v3.2 | Good all-round, cheap | Default chat |
| qwen3-235b | Strong reasoning | Complex analysis |
| kimi-k2.5 | Good at Chinese + English | Multilingual |
| nemotron-super-120b | Strong, no tool hints needed | Premium tasks |
| nemotron-nano-30b | Fast, cheap | Quick responses |

## Services & Ports

| Service | Port | Restart Command |
|---------|------|----------------|
| Backend | 4200 | `sudo systemctl restart qorven` |
| Frontend | 3000 | `sudo systemctl restart qorven-web` |
| LiteLLM | 4100 | `sudo systemctl restart litellm` |
| STT (Whisper) | 8881 | `kill PID && python3 scripts/stt-server.py &` |
| PostgreSQL | 5432 | `sudo systemctl restart postgresql` |

## Safe Modification Rules

1. **Always build before restart**: `CGO_ENABLED=0 go build ./...`
2. **Always run tests**: `CGO_ENABLED=0 go test ./... -count=1`
3. **Never modify**: `migrations/` (append only), `go.mod` (use `go get`)
4. **Config changes**: edit `config.toml`, restart backend
5. **Frontend changes**: edit in `qorven-web/`, build with `npx next build`
6. **Database changes**: create new migration file, apply with psql

## Supervisor Protocol

Prime monitors all Qors using 6 intent markers:
- `[STATUS_REQUEST]` — Prime asks "what's your status?"
- `[REVIEW_REQUEST]` — Qor asks "please verify my output"
- `[ACK]` — confirmed, conversation over (TERMINAL)
- `[ESCALATION_NOTICE]` — Prime can't resolve, human needed
- `[AUTO_FIX]` — Prime applies a low-risk fix
- `[HEARTBEAT]` — Qor reports it's alive with metrics

## Memory Hierarchy

| Scope | Visible To | Decay |
|-------|-----------|-------|
| Company | All Qors | Never |
| Team | Team members | Never |
| Prime | All (as context) | Can decay |
| Agent | Only that Qor | Yes |
| Session | Only that conversation | Yes |
