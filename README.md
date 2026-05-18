# Qorven

[![Release](https://img.shields.io/github/v/release/QorvenAI/qorven?include_prereleases&label=release)](https://github.com/QorvenAI/qorven/releases)
[![License: FSL-1.1-ALv2](https://img.shields.io/badge/license-FSL--1.1--ALv2-blue)](./LICENSE)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8)](./backend/go.mod)
[![CI](https://github.com/QorvenAI/qorven/actions/workflows/backend.yml/badge.svg)](https://github.com/QorvenAI/qorven/actions/workflows/backend.yml)

**Open-source, self-hosted AI workspace. A team of agents that works while you do.**

For developers, teams, and businesses that want powerful AI automation without sending data to a third-party cloud. One binary. Your server. Full control.

> **Status: v0.1.3-alpha** — running well on single-node Linux deployments. APIs and config schema may change before v1.0. Not yet recommended for critical workloads without a backup strategy.

---

## Quick start

**Linux / macOS (amd64 or arm64):**
```bash
curl -fsSL https://get.qorven.ai | sudo bash
```

**Windows (PowerShell — run as Administrator):**
```powershell
iwr -useb https://get.qorven.ai/install.ps1 | iex
```

The installer sets up PostgreSQL, downloads the binary, runs migrations, and opens the setup wizard in your browser — no config file editing required. Requires 2 GB RAM.

**Docs:** [docs.qorven.ai](https://docs.qorven.ai) &nbsp;·&nbsp; **Discussions:** [GitHub Discussions](https://github.com/QorvenAI/qorven/discussions)

---

## What's included

### Agents & teams

| | |
|---|---|
| **Multi-agent team** | Prime (coordinator) + Developer, Researcher, Writer, and Email agents out of the box. Create unlimited custom agents with their own identity, model, tools, and memory. |
| **Soul system** | Each agent has a rich identity bundle — system prompt, capabilities, behavior rules — with priority layering. Agents can generate souls for sub-agents. |
| **Agent runtime** | Heartbeat probes, pause/resume/wakeup controls, scheduled dreaming (reflection), goals tracking, and escalation-to-human when stuck. |
| **Approval workflows** | Human-in-the-loop gates for sensitive agent actions. Approve or reject from any connected channel. |

### Tools (120+)

| | |
|---|---|
| **Code & terminal** | Shell exec, file ops, `apply_patch`, glob/grep, go/ts/cargo diagnostics, git, GitHub API. Full in-browser IDE with file explorer and terminal. |
| **Web & research** | Multi-source search, full-page reader with 5-layer anti-bot stack (TLS/JA3, HTTP/2, header profiles, headless browser, proxy rotation), PDF reader, structured scraping. |
| **Email** | Send, receive, and reply via any SMTP/IMAP account. Header injection protection, HTML-to-text conversion. |
| **Social media** | Publish to Twitter/X, LinkedIn, Facebook, Instagram, Threads, TikTok, YouTube, Bluesky, Mastodon, and Pinterest. Schedule posts, autopost from RSS/webhook, content calendar. |
| **Image generation** | DALL-E, Stability AI, FLUX. Size and quality options with provider fallback chains. |
| **Video generation** | FAL, Seedance, Runway, Kling. Async polling with 5–10 second clip support. |
| **Voice & audio** | Speech-to-text (AssemblyAI, Deepgram, OpenAI, Moonshine, HuggingFace, Cartesia, LiveKit, Ollama). Text-to-speech (ElevenLabs, OpenAI, Piper, HuggingFace, Cartesia, Ollama, Plivo, Twilio). Real-time voice via WebRTC and OpenAI Realtime API. |
| **Business ops** | Shipment tracking (DHL, FedEx, SF Express, YTO, STO). Bilingual PDF quote and invoice generation. |
| **Data & SQL** | SQL query tool with read-only transaction enforcement. Spreadsheet ops, document reader. |
| **Connectors** | 50 pre-seeded integration schemas: Google Workspace, Notion, HubSpot, Airtable, Stripe, Shopify, Salesforce, Linear, Jira, Zendesk, Mailchimp, SendGrid, Twilio, Zoom, and more. |

### Channels (20+)

Telegram · WhatsApp (Cloud API) · Email (IMAP/SMTP) · Slack · Discord · Teams · LINE · DingTalk · WeCom · Feishu · Zalo · Webchat (WebSocket) · Webhook · GitHub · Signal · Matrix · Mattermost · iMessage · SMS

Each channel has inbound routing rules, keyword triggers, approval gates, and reply queues.

### Memory & intelligence

| | |
|---|---|
| **Persistent memory** | pgvector semantic search with BM25 full-text fallback. Hierarchical scopes: workspace → team → agent. |
| **Knowledge graph** | Entity extraction, vector similarity, relationship indexing, PageRank centrality. |
| **REM dreaming** | Scheduled memory consolidation — agents reflect on past conversations and surface key insights. |
| **Deep search** | Multi-source research graphs with source deduplication and cross-referencing. |
| **Daily briefings** | Per-agent scheduled briefings delivered to any channel. Include connector snapshots, pending tasks, and prioritised items. |

### Social media management

Qorven includes a full social media management layer — similar to Postiz or Buffer, but agent-driven and self-hosted:

- **10-platform publisher** — Twitter/X, LinkedIn, Facebook, Instagram, Threads, TikTok, YouTube, Bluesky, Mastodon, Pinterest
- **Scheduling** — ISO datetime scheduling with automatic publish at the scheduled time
- **Content calendar** — month view with scheduled, published, and draft posts
- **AutoPost rules** — cron-driven posting from RSS feeds or webhooks
- **Trend monitoring** — Twitter, YouTube, Reddit, HackerNews trend signals
- **Topic monitoring** — continuous tracking with change detection and agent notifications
- **Human approval gate** — agent-drafted posts go to `/outbound` for review before publishing

### Automation

| | |
|---|---|
| **Cron scheduler** | Per-agent cron schedules with DB-backed deduplication and human-readable display. |
| **Inbound rules engine** | Classify messages, auto-reply, route to agents, or generate draft replies for human review — on every channel. |
| **Workflow engine** | Multi-step automation with branching and error handling. |
| **Scenario engine** | Multi-agent simulations and stakeholder analysis. Generate personas from seed text, run multi-round deliberations. |
| **Goals system** | Agent and workspace-scoped goal tracking with 6 built-in templates (research, marketing, monitoring, standup, support, code review). |

### App platform & self-extension

| | |
|---|---|
| **App SDK** | Install any binary as an agent tool using a simple stdin/stdout protocol. Hot-loaded at runtime. |
| **Self-extending connectors** | Agents research an API, write a Go connector, compile it, install it, and use it — in one conversation. No developer. No restart. |
| **Self-building dashboards** | 15+ widget types (stat card, chart, kanban, timeline, data table, feed). Agents build and populate dashboards from live connector data. |

---

## Bring your own model

Qorven works with every major AI provider. Use your own API keys — zero markup on tokens.

**OpenAI · Anthropic · Google Gemini · DeepSeek · Groq · Mistral · xAI · Cerebras · Ollama · OpenRouter · any OpenAI-compatible endpoint**

The smart router picks the right model tier per task automatically, or pin a specific model per agent.

---

## Web dashboard

| Route | Purpose |
|---|---|
| `/chat` | Streaming chat with tool call display and session history |
| `/code` | In-browser IDE with terminal, file explorer, and diagnostics |
| `/social` | Social media composer, scheduler, content calendar |
| `/channels` | Channel management with connection status and QR flows |
| `/models-hub` | Provider key management, model registry browser |
| `/qors` | Agent profiles with Memory, Skills, Metrics, Schedules, Mail, Permissions tabs |
| `/mail` | Agent-managed inbox |
| `/approvals` | Pending agent action approvals |
| `/calendar` | Combined events + scheduled social posts |
| `/sessions`, `/memories`, `/knowledge-graph`, `/audit`, `/settings` | Operations and history |

---

## CLI & TUI

```bash
qorven start                # run the server
qorven chat                 # terminal chat with markdown rendering and / slash commands
qorven install              # full-screen BubbleTea TUI installer
qorven migrate up/down      # database migrations
qorven agents list/get/...  # agent management
qorven auth login/logout    # local API authentication
```

---

## Development

```bash
git clone https://github.com/QorvenAI/qorven.git
cd qorven
cp .env.example .env          # set QORVEN_DB_PASSWORD
docker compose up -d          # starts Postgres

# Backend (hot reload via air)
cd backend && make dev-watch

# Frontend (separate terminal)
cd web && pnpm install && pnpm dev
```

The web UI runs at `http://localhost:3000`. The API runs at `http://localhost:4200`.

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for full setup, code style, and PR flow.

---

## Roadmap

See [`ROADMAP.md`](./ROADMAP.md) for what's planned toward v1.0.

---

## Sponsor Qorven

Qorven is free and open source. Sponsorship funds core development, integrations, documentation, and long-term maintenance.

- **GitHub Sponsors:** [github.com/sponsors/QorvenAI](https://github.com/sponsors/QorvenAI)
- **Ko-fi:** [ko-fi.com/qorvenai](https://ko-fi.com/qorvenai)
- **Razorpay** (Indian supporters): [qorven.ai/sponsor](https://qorven.ai/sponsor)

Enterprise sponsors receive logo placement and priority issue handling. [Contact us](mailto:hello@qorven.ai).

---

## Acknowledgements

Built on excellent open-source foundations:

[PostgreSQL](https://www.postgresql.org/) · [pgvector](https://github.com/pgvector/pgvector) · [Next.js](https://nextjs.org/) · [Tailwind CSS](https://tailwindcss.com/) · [Chi](https://github.com/go-chi/chi) · [pgx](https://github.com/jackc/pgx) · [Cobra](https://github.com/spf13/cobra) · [Bubbletea](https://github.com/charmbracelet/bubbletea) · [Lucide](https://lucide.dev/) · [Zustand](https://github.com/pmndrs/zustand)

Full dependency list: [`backend/go.mod`](./backend/go.mod) · [`web/package.json`](./web/package.json)

---

## Community

- **Docs:** [docs.qorven.ai](https://docs.qorven.ai)
- **Bug reports & feature requests:** [GitHub Issues](https://github.com/QorvenAI/qorven/issues)
- **Questions & ideas:** [GitHub Discussions](https://github.com/QorvenAI/qorven/discussions)
- **Security vulnerabilities:** see [`SECURITY.md`](./SECURITY.md) — do not open public issues

---

## License

[FSL-1.1-ALv2](./LICENSE) — free to use, modify, and self-host. Converts to Apache 2.0 two years after each release date.
