# Changelog

All notable changes to Qorven are documented here.

---

## v0.1.32-alpha — 2026-05-24

### Added
- **Full-featured coding workbench at `/code`** — the editor now has a diff viewer, live preview iframe, slash command palette, and `@filename` file references. Every feature you'd expect from a modern AI coding tool is now built in.
- **Diff viewer** — see exactly what the agent changed, line by line, with syntax-highlighted add/remove display. Switch between Code, Diff, and Preview with the three-way toggle.
- **Slash command palette** — type `/` to open a searchable list of commands: `/new`, `/undo`, `/export`, `/add`, `/plan`, `/diff`, `/init`, `/clear`.
- **@filename file references** — type `@` in chat to fuzzy-search your project files. The selected file's content is automatically included in the message so Prime has the right context.
- **Live preview iframe** — see your running app inside the IDE with desktop, tablet, and mobile device frame options.
- **Deploy panel** — when a build is complete, one-click buttons link to Vercel, Netlify, and GitHub Pages deployments, or download the project as a ZIP.
- **Qorven App Builder mode** — building a Qorven plugin? A dedicated panel shows install status, lets you reload the app after edits, and lets you test individual tools without leaving the IDE.
- **Context panel** — pin any files above the chat input with `/add`. Their content is prepended to every message automatically.
- **Multi-session tabs** — open multiple projects simultaneously. Switching tabs preserves the full state of each session — messages, editor tabs, build log, and diff view all restore instantly.
- **Checkpoints and undo** — the backend creates git-backed restore points before agent writes. `/undo` rolls back the last change without losing history.
- **AGENTS.md initialiser** — `/init` sends Prime to read the codebase and write a project context file. A green confirmation chip appears once the file is written, so Prime knows your project from the very next session.
- **Async agent delegation** — Prime dispatches build work to background agents and stays responsive immediately. Background jobs show a live status chip that turns green on completion.
- **Plan mode** — toggle Plan/Build in the sidebar header. In Plan mode, Prime reasons and proposes changes without writing any files. Flip to Build to execute.
- **Token and cost display** — cumulative token usage appears below the chat input. The counter turns amber above 40K tokens and red above 80K.

### Improved
- **Smarter context compaction** — four-tier model-aware compaction with configurable thresholds reduces context window pressure on long sessions.
- **Intent classification** — agent tools are now gated per request type, so a "what does this do?" question never accidentally triggers file writes.

---

## v0.1.31-alpha — 2026-05-23

### Added
- **Apps now have their own sidebar navigation** — when you open an installed app at `/apps/{slug}/page`, the sidebar switches to that app's pages only, with the app name as the header and a back link to the launcher. Apps behave like self-contained mini-products inside Qorven.
- **Multi-page app support** — apps register named pages with labels via `register({ pages: [{ id, path, label }] })`. The sidebar navigation is driven entirely by the bundle, so page labels work immediately without backend manifest configuration.
- **Scaffold template updated** — new apps built by agents include `displayName`, `label`, and `icon` fields in the `register()` call by default.

---

## v0.1.30-alpha — 2026-05-23

### Fixed
- **App bundles now load in the browser** — the dev server and the auth middleware now correctly forward `/app-assets/*` requests to the backend, so installed app bundles render instead of showing "not registered".
- **Project agents no longer fail when only Gemini is configured** — inception agents assigned a Claude/DeepSeek model by the planner now automatically fall back to the available provider's model instead of failing silently.

---

## v0.1.29-alpha — 2026-05-23

### Fixed
- **App cards are now clickable** — clicking an installed app on the Apps page opens it directly instead of doing nothing.
- **Activity feed shows agent names** — the "Today's activity" panel on the dashboard now shows agent display names instead of raw internal IDs.

---

## v0.1.28-alpha — 2026-05-23

### Fixed
- **Project agents now start autonomously** — agents created from a project brief now run continuously without waiting for supervisor approval on every tool call, and multi-task project plans are approved automatically so work begins immediately.
- **Project task panel now populates** — the code panel's task list now fills in as soon as a project is approved instead of staying empty.
- **Developer agents know how to build apps** — the developer role system prompt now includes the full scaffold → build → install sequence so agents can produce working Qorven apps end-to-end.

---

## v0.1.27-alpha — 2026-05-23

### Fixed
- **Installed apps now appear instantly after build** — after an agent builds and installs a new app, the browser picks it up automatically without requiring a page refresh.
- **App bundles always render correctly** — the scaffold template now generates bundles compatible with the Qorven host SDK; apps built by agents no longer fail to mount due to a JSX runtime mismatch.
- **App sidebar shows the correct app after install** — the app registry now correctly matches installed apps to their URL routes.
- **Bundle cache removed** — rebuilt app bundles are now served immediately; the previous 60-second browser cache could serve a stale bundle after an agent update.
- **`install_app` now validates the build before installing** — if an agent calls `install_app` before running `npm run build`, it receives an actionable error rather than a silent failure at runtime.

---

## v0.1.26-alpha — 2026-05-23

### Fixed
- **Installed apps render the correct content** — the app client now reads slug and path from the live URL when the static shell contains placeholder values, ensuring installed apps always display their actual pages.

---

## v0.1.25-alpha — 2026-05-23

### Fixed
- **Agent error events now display in chat** — when an agent encountered an error mid-stream, the message appeared as an empty bubble. Errors now render as visible text inside the conversation.
- **Installed apps now render correctly** — navigating directly to an app page no longer fell back to the dashboard. The routing layer now correctly serves the app shell for any installed app path.

---

## v0.1.24-alpha — 2026-05-22

### Added
- **Telegram group writer management** — agents can now maintain a whitelist of Telegram users who are permitted to trigger file-write operations. Admins manage the list via `/writers list`, `/writers add @user`, and `/writers remove @user` commands within the group. Permissions persist across restarts.
- **Video analysis tool** — agents can now analyse video files directly: the system extracts a representative frame and sends it to a vision-capable model, returning a structured description. No external service required.
- **Team task orchestration** — the `team_tasks` tool is fully operational: agents can create, list, transition, and complete tasks assigned to teammates. Completing or updating a task automatically wakes any agent whose work was blocked on it.
- **Team messaging** — agents can send direct messages to other agents at runtime via the `team_message` tool, waking the recipient immediately with the provided content.
- **Draft approval notifications** — when an inbound message is held for human review, the requesting channel now receives a formatted notification with message preview and draft ID, so approvers are never left waiting silently.
- **Jira integration** — the Jira connector now executes `create_issue`, `search_issues`, and `get_issue` actions against Jira Cloud, with proper ADF body formatting and Basic auth.
- **Google Sheets integration** — the Google Sheets connector now executes `read_range`, `append_row`, and `update_range` actions against the Sheets API v4.

---

## v0.1.23-alpha — 2026-05-21

### Fixed
- **Background updater no longer overwrites dev builds** — when running a locally-compiled binary (dirty working tree or untagged commit), the auto-updater now skips installation entirely rather than downloading and replacing the binary. Dev sessions stay on the local build.

---

## v0.1.22-alpha — 2026-05-21

### Fixed
- **Qors list no longer flashes a skeleton on every visit** — navigating to Qors always triggered a full loading skeleton even when agents were already cached in the store. The list now refreshes silently in the background on revisit; the skeleton only appears on the very first load.

---

## v0.1.21-alpha — 2026-05-21

### Fixed
- **GUI update now restarts the service correctly** — after swapping the binary the update handler was calling `systemctl restart qorven`, which systemd silently accepts (exit 0) but ignores when the request comes from inside a sandboxed service process (`NoNewPrivileges=yes`). The service now performs a clean process exit instead, which lets systemd's `Restart=always` policy bring up the new binary automatically. The "Restarting…" page now completes as expected.

---

## v0.1.20-alpha — 2026-05-21

### Fixed
- **"Update available" no longer re-appears after a successful update** — the background update checker was comparing raw version strings (`v0.1.19-alpha` vs `0.1.19-alpha`) which never matched, causing the checker to re-install on every 6-hour tick. Both sides are now normalised before comparison.
- **Auto-update now restarts the service correctly** — `triggerSelfRestart` used `cmd.Start()` which returns nil even when `systemctl restart` is blocked by the sandbox, so the service never exited and systemd never launched the new binary. Changed to `cmd.Run()` so failed systemctl falls through to `selfExit()`, which triggers `Restart=always` cleanly.

---

## v0.1.19-alpha — 2026-05-21

### Fixed
- **GUI update no longer fails after an auto-update has run** — `os.Executable()` returns a deleted-inode path (e.g. `qorven.bak`) when the background updater replaced the binary while the process was live. The update handler now resolves the canonical install path (`/opt/qorven/bin/qorven`) directly instead of trusting `os.Executable()`, so clicking Install always targets the correct file regardless of how many times the binary has been swapped.

---

## v0.1.18-alpha — 2026-05-21

### Fixed
- **GUI software update now works reliably on all systemd installs** — the previous self-update mechanism failed with "read-only file system" or "permission denied" on standard Ubuntu/EC2 deployments because the binary lived in `/usr/local/bin/` (owned by root) and `NoNewPrivileges=yes` blocked all escalation paths. The binary is now installed in `/opt/qorven/bin/` which is owned by the `qorven` service user, so the service can atomically rename the new binary into place without sudo, nsenter, or sandbox escaping. Existing installs are migrated automatically on first startup — no manual steps required.
- **CLI `qorven update` preserves ownership** — after a CLI-triggered update the binary is chowned back to `qorven:qorven` so subsequent GUI updates continue to work without needing sudo.
- **Windows update provisioning** — `qorven update` and the GUI update handler now stop the Windows service before swapping the binary (Windows locks open executables) and restart it after. NSSM-managed installs update cleanly.

---

## v0.1.17-alpha — 2026-05-21

### Fixed
- **No more full-page reloads when navigating to Qors** — six internal links across the header breadcrumb, context panel, right panel, and dashboard pages used plain HTML anchors instead of the Next.js router, forcing a full browser reload on every click. All converted to client-side navigation.
- **Icons are thicker and more legible** — rail, top bar, and status bar icons rendered at the default stroke weight (2) which appears thin at small sizes. Now rendered at 2.5 across all three bars.
- **Status bar text is fully readable** — chip labels were rendered at 50–75% opacity, making them difficult to read in ambient light. Now at full muted-foreground opacity.
- **Code breadcrumb is bold when no project is open** — "Code" in the header breadcrumb was not bold, inconsistent with every other section's active label.
- **Uptime displays as HH:MM:SS** — was showing `1h 5m 3s`; now shows `01:05:03` with zero-padded fields.

### Changed
- **README no longer contains a hardcoded version number** — the release badge at the top tracks the current version automatically. The stability statement is rewritten to be factual and stable rather than release-specific.

---

## v0.1.16-alpha — 2026-05-21

### Fixed
- **Software Update no longer shows a false "update available"** — the version shown in Settings matched the installed version but the comparison was failing due to a `v` prefix mismatch (`v0.1.16-alpha` vs `0.1.16-alpha`). Both sides are now normalised before comparing.
- **One-click update now works reliably on systemd installs** — the binary swap tries three escalation paths: a transient systemd-run unit, nsenter into the host mount namespace, then a direct write. Any of the three succeeds on a standard Ubuntu/EC2 install. The previous fix worked in some configurations but failed when PrivateTmp=yes isolated /tmp from the transient unit. The new staging path (/run) is visible across all mount namespaces, making the swap reliable regardless of how the service is sandboxed.

---

## v0.1.15-alpha — 2026-05-21

### Added
- **New project landing on /code** — the Code section now opens with a centred canvas letting you choose between Vibe, Spec, and Ship modes before starting, with a prominent chat input and quick-start suggestions. Once you submit your first message the workspace transitions into the full plan and execution view.
- **Project brief sidebar** — active and past projects are listed in the sidebar under a dedicated Inception tab, with live status indicators showing where each project is in the pipeline.
- **Spec, team, and task approval flow** — after describing a project Prime presents the architecture spec, proposed team, and task breakdown in three sequential steps, each requiring your approval before continuing.
- **Execution canvas** — once approved, the canvas shows real-time task completion and budget usage with a live task feed and per-agent progress bars.
- **Budget and timeline controls on the project canvas** — set your budget cap and deadline with one click before starting; Prime sizes the team and selects model tiers accordingly.

### Fixed
- **Project list no longer creates a second sidebar** — the project navigator is integrated into the existing Code sidebar, consistent with the rest of the layout.

---

## v0.1.14-alpha — 2026-05-21

### Fixed
- **System status chips now render** — RAM, disk, cost, and active agent chips in the status bar were blank after login. Now display correctly on every page load.
- **Status bar version is always current** — the version chip no longer showed a stale value after a restart; it now reflects the running version at all times.
- **No full-page reload when navigating to Chat or Qors** — clicking those items in the sidebar no longer triggered a hard browser navigation.
- **Changelog popup shows release notes** — the in-app changelog lightbox was showing a spinner instead of the release notes.
- **Disconnect indicator no longer flashes on load** — the offline dot in the top bar briefly appeared on every page load before the WebSocket connected; it now only appears after a sustained disconnect.

### Improved
- **Icons are thicker and crisper** — sidebar rail, top bar, and status bar icons now render at a consistent weight that is more legible, especially on high-DPI displays.
- **Design system centralised** — typography scale, icon weight, and toast notifications are now controlled from single shared components. Visual changes apply everywhere at once without touching individual pages.

---

## v0.1.13-alpha — 2026-05-20

### Fixed
- **Installer no longer fails on Ubuntu 22.04 with "docker install: exit status 100"** — the Docker step now removes conflicting Ubuntu-shipped packages (`docker.io`, `docker-compose`, `containerd`) before installing, refreshes the apt cache first, and treats Docker as non-fatal (Qorven works without it — Docker is only needed for the `run_app` container tool). If Docker installation fails the installer continues and shows a manual install hint.

---

## v0.1.12-alpha — 2026-05-20

### Fixed
- **Update works from GUI without manual intervention** — the binary swap no longer uses `systemd-run --scope` (which requires a D-Bus session bus unavailable inside a service unit). Now uses `systemd-run --wait` (transient service, no D-Bus needed) with a direct write fallback.
- **`qorven update` works without sudo on EC2/Ubuntu** — CLI now tries `sudo sh -c "cp && mv"` first (passwordless sudo is the default on Ubuntu cloud images), so most users won't need to prefix with sudo.

---

## v0.1.11-alpha — 2026-05-20

### Fixed
- **Version shows in status bar** — the version chip at the bottom of every page was blank in production. The embedded frontend fetched `/api/health/detailed` but the Go binary only served `/health/detailed`. Both paths now work.
- **Update works without sudo** — `qorven update` and the Settings UI update button now try three escalation paths in order: direct write (already root), passwordless sudo (standard EC2/Ubuntu setup), then systemd-run scope (service sandbox escape). Most installs will succeed without any manual `sudo`.

---

## v0.1.10-alpha — 2026-05-20

### Fixed
- **Update works on systemd installs** — clicking "Install" in Settings or running `qorven update` no longer fails with "backup failed: read-only file system". The binary swap now escapes the systemd sandbox (`ProtectSystem=full`) by delegating the file operation to a transient `systemd-run --scope` process. Falls back to a direct write on non-systemd systems.

---

## v0.1.9-alpha — 2026-05-20

### Added
- **Port 8486 — Qorven's own port** — default bind is now `0.0.0.0:8486` instead of `4200/80`. Single port serves the API, WebSocket, and the embedded UI together. No nginx required for local, Tailscale, or private-network deploys.
- **Port picker in installer** — the TUI wizard now asks which port to use (default `8486`), probes availability before writing config, and warns if the port is already in use so you can override or choose another.
- **Port picker in Settings → Network** — the Network settings page shows the current listen port, lets you check whether any other port is free, and gives you the one-line config snippet to apply the change.
- **nginx is now opt-in** — the installer no longer sets up nginx by default. A new step asks `Set up nginx? [y/N]` — only installs it if you say yes.
- **Port-check API** — `GET /v1/admin/system/check-port?port=N` returns whether a port is available on the host machine.

### Changed
- **Old `api_listen` / `web_listen` config fields automatically migrated** — if your `config.toml` still has the old split-port layout, Qorven derives the new `listen` field from `api_listen`'s port on startup. No manual edit required.
- **install.sh detects existing installs** — running the install script on a machine that already has Qorven offers an update or reinstall choice instead of overwriting blindly.

---

## v0.1.8-alpha — 2026-05-20

### Fixed
- **Update now works from the UI** — clicking "Install" in Settings → Software Update no longer fails with a read-only filesystem error on systemd installs. Existing installs are patched automatically on next startup.
- **CLI update finds the latest version** — `qorven update` was picking up an older release instead of the newest. Now correctly finds the most recent release including pre-releases.
- **Version chip shows clean version** — local dev builds showed extra build info in the status bar version chip. Release builds now always show a clean version number.

---

## v0.1.7-alpha — 2026-05-20

### Added
- **One-click update from status bar** — click the version chip to open the changelog. If a newer version is available a green banner appears with an "Update now" button that installs and restarts automatically. A green dot on the chip signals an update is waiting.
- **Auto-update on startup** — installed instances check for a newer release 30 seconds after boot and every 6 hours, applying updates without manual steps.
- **Install telemetry** — the installer records platform, OS, distro, arch, cloud provider, CPU, and RAM anonymously to track adoption.

### Fixed
- **Qors page sidebar** — `/qors` was rendering its own custom left panel, looking different from every other page. Now uses the standard sidebar.
- **Chat page spinner** — opening a Qor's chat showed a full-screen spinner until both agent and session loaded. UI now renders immediately.
- **Version blank on fresh install** — binaries built before today's release workflow fix showed no version in the status bar. Auto-update replaces these on first boot.
- **Release workflow never triggered** — tag pattern mismatch (`qorven-v*` vs `v*`) meant CI never built release binaries automatically. Fixed.
- **Uptime ticks every second** — status bar uptime now counts up live instead of jumping every 10 seconds.

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
