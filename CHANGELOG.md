# Changelog

All notable changes to Qorven are documented here.

---

## v0.1.6-alpha — 2026-05-20

### Added
- **Status bar live stats** — bottom bar now shows real system RAM, disk usage, token counts (today), monthly spend, and a live Active Qors chip. Dot goes green when a Qor is running, amber when thinking, grey when all idle.
- **Auto-reload on backend upgrade** — frontend detects a version change via the `X-Qorven-Version` header on every 10s stats poll and reloads automatically. No more stale UI after a deploy.
- **nginx WS proxy** — WebSocket connections now route correctly through port 80 regardless of client origin (localhost, Tailscale, or custom domain).

### Fixed
- **RAM showing MB instead of GB** — status bar was reading Go process heap; now reads `/proc/meminfo` for actual system memory.
- **Disk showing 7.7 GB (tmpfs)** — `go run` compiles to `/tmp`; fixed by always statting `/` directly.
- **`air` / `pkill` hanging on shutdown** — `MessageBus.Close()` closed channels while consumer goroutines were blocked in bare receives, causing an infinite zero-value spin loop. Fixed with a dedicated `closed` channel that all selects honour.
- **Windows CI: postgresql.conf not found** — installer test now probes three known data dir paths before editing; treats missing conf as non-fatal since TCP may already be enabled.

---

## v0.1.5-alpha — 2026-05-20

### Changed
- **UI** — Standardised page headers across all canvas views (~51 pages now use a consistent title/description layout).

---

## v0.1.4-alpha — 2026-05-19

### Added
- **One-click OTA updates** — Settings → System → Install now downloads the new binary,
  verifies SHA256, atomically swaps it, patches the systemd unit (`Restart=always`),
  and restarts the service automatically. The UI shows a reconnection spinner and
  reloads the page when the server is back — no manual `systemctl restart` needed.

### Fixed
- **Cron deletion race** — deleting a schedule from the Schedules tab now disables
  the job first, then deletes, eliminating a 30-second window where the runner
  could pick up the row in a concurrent tick.
- **Room-mention schedules never fired** — cron jobs created by @mentioning an agent
  in a room were missing `next_run_at`, so they only started executing after the next
  server restart. Now set correctly on creation.
- **Windows installer: git clone NativeCommandError** — `git clone 2>&1 | Out-Null`
  threw `NativeCommandError` in PowerShell 5.1 when git wrote progress to stderr on
  success. Fixed by capturing output into a variable and checking `$LASTEXITCODE`.

---

## v0.1.3-alpha — 2026-05-17

### Fixes & hardening

#### Security
- **Email header injection** — `To`, `Subject`, `From`, and `In-Reply-To` headers are now sanitized to strip CR/LF before being written into raw MIME messages in both the email tool and the email channel
- **Zip slip in updater** — archive entry names are cleaned before `filepath.Join`; absolute paths and `../` prefixes are rejected; absolute symlink targets are blocked
- **URL scheme check** — `data:` and `vbscript:` are now blocked alongside `javascript:` in the HTML-to-Markdown link converter
- **SQL read-only enforcement** — `sql_query` tool now wraps read queries in a `READ ONLY` transaction so write-bearing CTEs (e.g. `WITH ins AS (INSERT ...) SELECT ...`) are rejected at the database level

#### Test reliability
- Fixed flaky `TestBridgeProcess_Send` — gorilla/websocket requires serialized writes; added `writeMu` to `BridgeProcess` to prevent concurrent-write panics under load
- Fixed `TestAdversarial_XSS_DisplayName` key collision — loop now uses an atomic counter instead of a millisecond timestamp
- Fixed `TenantScopeMiddleware` tests — `defer db.Close()` replaced with `t.Cleanup` to prevent pool closing before deployment-config cleanup runs; `deployment_config` writes now use the bypass pool (restricted `qorven_app` role has no write access to that table)
- Fixed CI connection exhaustion — `MinConns` reduced from 2 to 0; connections are created on demand, preventing Postgres `max_connections` limit from being hit under parallel test runs

#### Cleanup
- Removed unused `backend/ui/` scaffold (bootstrapped create-next-app, never wired into the build or served)

---

## v0.1.0-alpha — 2026-05-17

### Initial public release

This is the first open-source release of Qorven.

#### Agent platform
- Multi-agent runtime: Prime coordinator + Developer, Researcher, Writer, and Email agents out of the box
- Soul system: rich identity bundles (system prompt + capabilities + behaviour rules) with priority layering
- Setup wizard collects admin account, assistant persona, communication style, language, and first AI provider
- Sub-agent soul generation: agents can write identity prompts for newly created Qors
- Agent dreaming (scheduled reflection), heartbeat probes, and QorOS runtime controls (pause/resume/wakeup)
- Hierarchical memory store backed by pgvector with BM25 full-text search and recency fallback
- Cron job scheduler: per-agent schedules, DB-backed deduplication, human-readable display
- Tool permission system: per-agent profiles with auto-approve / ask-first / blocked policy tiers

#### Channels
- Telegram, WhatsApp (Cloud API), Email (IMAP/SMTP), Slack, Discord, Teams, LINE, Webchat, Webhook
- DingTalk, WeCom, Feishu, Zalo, Facebook, GitHub, SMS, iMessage, Matrix, Mattermost, Signal
- Inbound routing rules, keyword triggers, approval gates, and reply queues

#### Provider support
- Anthropic, OpenAI, Google Gemini, DeepSeek, Groq, Mistral, xAI, Cerebras, Together, Ollama, OpenRouter
- Smart router: complexity-based tier selection (standard / advanced / code)
- Per-provider encrypted API key vault with test-and-verify flow

#### App platform
- Install Go binary connectors from disk via `POST /v1/apps`
- Enable/disable, reload, and uninstall without restarting the server
- Agents can scaffold and install new connectors at runtime through the agent loop

#### Web dashboard
- `/qors` — agent profiles with Memory, Skills, Metrics, Schedules, Mail, Permissions, and Settings tabs
- `/chat` — streaming chat with tool call display and session history
- `/code` — Code IDE with terminal and file explorer
- `/channels` — channel management with connection status and QR flows
- `/models-hub` — provider key management, model registry browser
- `/approvals` — pending agent action approvals
- `/sessions`, `/mail`, `/contacts`, `/org-chart`, `/audit`, `/settings`
- Danger Zone: selective data resets and factory reset with password confirmation

#### CLI & TUI
- `qorven start` — run the server
- `qorven install` — full-screen BubbleTea TUI installer (PostgreSQL setup, config, migrations, systemd)
- `qorven chat` — terminal chat with markdown rendering and `/` slash commands
- `qorven migrate up/down/force` — database migration management
- `qorven auth login/logout/whoami` — local API authentication
- `qorven agents list/get/create/update/delete` — agent management

#### Infrastructure
- Single baseline migration (`001_schema.up.sql`) — fresh installs run one file
- Embedded migrations in the binary; disk migrations override when present
- Systemd service management, structured logging, `/health` and `/health/detailed` endpoints
- Cross-compile targets: linux/amd64 and linux/arm64
- Docker Compose for local development (PostgreSQL + pgvector)
- GitHub Actions: build + test + release

#### Known limitations at v0.1.0
- Single-node only — no HA or multi-node clustering
- Matrix, Signal, Mattermost, iMessage are scaffolded but not fully wired
- No docs site yet
- Frontend has limited automated test coverage

---

> Missing something? [Open an issue](https://github.com/QorvenAI/qorven/issues/new) or [start a Discussion](https://github.com/QorvenAI/qorven/discussions).
