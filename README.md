# Qorven

[![Release](https://img.shields.io/github/v/release/QorvenAI/qorven?include_prereleases&label=release)](https://github.com/QorvenAI/qorven/releases)
[![License: FSL-1.1-ALv2](https://img.shields.io/badge/license-FSL--1.1--ALv2-blue)](./LICENSE)
[![Go](https://img.shields.io/badge/go-1.26-00ADD8)](./backend/go.mod)
[![CI](https://github.com/QorvenAI/qorven/actions/workflows/backend.yml/badge.svg)](https://github.com/QorvenAI/qorven/actions/workflows/backend.yml)

**Open-source, self-hosted AI workspace. A team of agents that works while you do.**

For developers, teams, and businesses that want powerful AI automation without sending data to a third-party cloud. One binary. Your server. Full control.

> **Status: v0.1.0-alpha** — running well on single-node Linux deployments. APIs and config schema may change before v1.0. Not yet recommended for critical workloads without a backup strategy.

---

## Quick start

```bash
curl -fsSL https://get.qorven.ai | sudo bash
```

Requires a Linux server (amd64 or arm64) with 2 GB RAM and PostgreSQL. The installer sets up the database, downloads the binary, runs migrations, and opens the setup wizard in your browser — no config file editing required.

**Docs:** [docs.qorven.ai](https://docs.qorven.ai) &nbsp;·&nbsp; **Discussions:** [GitHub Discussions](https://github.com/QorvenAI/qorven/discussions)

---

## What's included

| | |
|---|---|
| **Multi-agent team** | Prime (coordinator) + Developer, Researcher, Writer, and Email agents out of the box. Create unlimited custom agents with their own identity, model, and tool access. |
| **120+ built-in tools** | Web search, file ops, code execution, email send/receive, terminal, shipment tracking, social media, web scraping, and more. |
| **20+ channel integrations** | Telegram, WhatsApp, Email (IMAP/SMTP), Slack, Discord, Teams, LINE, Webchat, Webhook, GitHub, DingTalk, WeCom, Zalo, and more. |
| **Scheduling & automation** | Cron-based scheduled tasks, inbound message routing rules, and daily AI briefings. |
| **Memory** | Agents remember preferences, past conversations, and project context across sessions using pgvector. |
| **Approval workflows** | Human-in-the-loop gates for sensitive agent actions — approve or reject from any connected channel. |
| **App platform** | Install Go binary connectors from disk. Agents can scaffold and install new connectors at runtime. |
| **Web dashboard** | Chat, Code IDE, Mail, Sessions, Channels, Models Hub, Agents, Approvals, Settings, Audit log. |
| **CLI & TUI** | `qorven chat` — full terminal interface with markdown rendering and slash commands. |

---

## Bring your own model

Qorven works with every major AI provider. Use your own API keys — zero markup on tokens.

**OpenAI · Anthropic · Google Gemini · DeepSeek · Groq · Mistral · xAI · Cerebras · Ollama · OpenRouter · any OpenAI-compatible endpoint**

The smart router picks the right model tier per task automatically, or pin a specific model per agent.

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
