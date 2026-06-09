# Changelog

All notable changes to Qorven are documented here.

---

## v0.8.0-alpha — 2026-06-09

The **Operations Fabric** — the layer that makes your agents work together like a real company, and reach you like a colleague would.

### Added
- **Agents reach you the way a person would.** When an agent needs your approval or wants to notify you, it starts in the app — and if you're away, it follows up on Telegram or WhatsApp, then by email, until you respond. Reply on any channel and the rest stop chasing you. Nothing waits silently, and nothing blocks work that can carry on. ([4ed5227](https://github.com/QorvenAI/qorven/commit/4ed5227))
- **One approvals inbox for the whole company.** Every decision that needs you — a budget allocation, and more to come — flows through a single approval object with a clear summary, amount, and context. Approve or reject, and the work that was waiting picks up automatically. Pending approvals never stall the rest of the org. ([569fce3](https://github.com/QorvenAI/qorven/commit/569fce3))
- **Trackable work items.** Work is now durable and visible: who owns it, what it's waiting on, and a full history of every step — so a piece of work survives restarts and you can always see where it stands. ([569fce3](https://github.com/QorvenAI/qorven/commit/569fce3))
- **Departments run themselves — within the limits you set.** Each department has an autonomy policy: proceed automatically within budget, always ask you, or a mix (small things proceed, big ones ask). For a substantial plan, your CFO **projects** whether it fits — reserving room for ongoing daily spend and work already committed — before it proceeds or comes to you. IT/engineering starts cautious (big builds ask); other departments run automatically within budget. ([2afa835](https://github.com/QorvenAI/qorven/commit/2afa835))
- **Instruct any leader by role in a room.** In a room you can now address `@COO`, `@CTO`, `@CFO` and reach the officer holding that role directly. Rooms are also kept calm: there's a cap on automated replies so a conversation can never loop or run up cost. ([0a31880](https://github.com/QorvenAI/qorven/commit/0a31880))
- **Every agent starts knowledgeable.** New leadership roles (CTO, Chief Knowledge Officer, Chief Information Security Officer, Chief Communications Officer) ship with full personas, and a Knowledge Officer now curates the company's knowledge into role-specific briefs that are delivered into each agent's context — filtered to what that agent is cleared to see. New hires are provisioned with their clearance and knowledge access from their first run. ([d793a4e](https://github.com/QorvenAI/qorven/commit/d793a4e))

### Changed
- Pending budget proposals from your CFO now reach you through the same follow-up flow (app → messaging → email) and stop chasing you the moment you decide. ([8c669af](https://github.com/QorvenAI/qorven/commit/8c669af))

---

## v0.7.0-alpha — 2026-06-09

### Added
- **CFO runs your budget.** Your CFO agent can now propose budget allocations across departments, projects, and agents. You set how much autonomy it has — **ask me every time**, **act within a threshold** (small moves apply instantly, larger ones come to you), or **full authority**. Every change is validated against your hierarchy caps so nothing can over-spend. ([a20b675](https://github.com/QorvenAI/qorven/commit/a20b675), [3012d20](https://github.com/QorvenAI/qorven/commit/3012d20))
- **A dedicated Budgets page.** Review and approve the CFO's proposals line by line, set the CFO's authority level, and see your effective-available reconciliation in one place. Pending approvals never stop work — agents keep running while you decide. ([c325740](https://github.com/QorvenAI/qorven/commit/c325740))
- **Smart model assignment by role.** New agents are now given the right model for their job automatically: your CTO gets the best coding model, your CFO the best at math, and other officers the best all-round model — chosen only from the models you've enabled, and sized to the agent's budget (workers get efficient models, never premium). You can still change any agent's model yourself. ([405e8bd](https://github.com/QorvenAI/qorven/commit/405e8bd), [9f556f6](https://github.com/QorvenAI/qorven/commit/9f556f6))

### Changed
- Budget changes are now restricted to your CFO — other agents can no longer alter budgets. ([1ab878b](https://github.com/QorvenAI/qorven/commit/1ab878b))

### Fixed
- The model catalog now loads correctly — a data formatting issue had left it empty, which weakened automatic model selection across the platform. ([405e8bd](https://github.com/QorvenAI/qorven/commit/405e8bd))

---

## v0.6.0-alpha — 2026-06-09

### Added
- **Budget governance — keep your AI spend under control.** A complete, corporate-style budget system runs across your whole workspace. Set an **overall budget** for the company, allocate it down to **departments, projects, and individual agents**, and even cap a single **task**. Every level enforces its limit, and spend rolls up the chain. ([d8…](https://github.com/QorvenAI/qorven/commit/c002b69))
- **Every model call is now metered and capped.** All AI usage — your agents, their background work, memory upkeep, research, everything — is recorded to a precise, bank-grade ledger and checked against budgets before it runs. When a budget is hit, work pauses cleanly instead of running up a surprise bill. ([205c591](https://github.com/QorvenAI/qorven/commit/205c591), [25a9225](https://github.com/QorvenAI/qorven/commit/25a9225))
- **Fund your budget your way.** Declare a **prepaid fixed amount** that depletes as you spend (the default — matches how most people top up provider credits) or a **monthly recurring** budget that resets each cycle. ([f246962](https://github.com/QorvenAI/qorven/commit/f246962))
- **Effective-available reconciliation.** Qorven reconciles the budget you declared against what your connected provider keys actually allow, and shows your true spendable amount — with a clear warning when your declared budget is bigger than your keys can fund. View it on the **Provider Spend** page. ([4f5818a](https://github.com/QorvenAI/qorven/commit/4f5818a), [4e04c26](https://github.com/QorvenAI/qorven/commit/4e04c26))
- **Departments and projects.** Each C-officer heads a department; workers inherit their manager's. Budgets can be **carved** from a parent pool (drawing it down) or granted as a **fresh** additive pool for a specific project. ([bbe7a55](https://github.com/QorvenAI/qorven/commit/bbe7a55), [e369a6c](https://github.com/QorvenAI/qorven/commit/e369a6c))
- **Per-key funding controls.** Declare each provider key as prepaid (with a loaded balance), postpaid, quota, or free — and set a usage window (e.g. "N requests per 5 hours") for subscription/OAuth keys. ([25973f0](https://github.com/QorvenAI/qorven/commit/25973f0), [722acb2](https://github.com/QorvenAI/qorven/commit/722acb2))
- **No more stalling on rate limits.** When a provider key hits its declared window or returns a rate-limit error, Qorven automatically routes around it to another key so your agents keep working. ([5247fd3](https://github.com/QorvenAI/qorven/commit/5247fd3), [dcee0f9](https://github.com/QorvenAI/qorven/commit/dcee0f9))

### Changed
- Spend is now tracked in exact micro-dollar precision across every usage path, so cost reporting and budget enforcement always agree. ([205c591](https://github.com/QorvenAI/qorven/commit/205c591))

---

## v0.5.6-alpha — 2026-06-08

### Fixed
- **Voice now starts reliably** — fixed the voice activity detector failing to load its model, which previously threw an error whenever a chat or voice surface opened. Voice input works on first try again. ([7e68176](https://github.com/QorvenAI/qorven/commit/7e68176))

---

## v0.5.5-alpha — 2026-06-08

### Added
- **Worker view for your team** — agents now follow a clear org model: you chat with your COO and C-officers, while the specialists working under them get a dedicated read-only monitor instead of a chat box. Open a worker to see what it's doing right now, its task list and history, its profile and skills, and the external services it can use — with a one-click "Message its manager" shortcut. ([dbd7b2f](https://github.com/QorvenAI/qorven/commit/dbd7b2f), [ea6d1e8](https://github.com/QorvenAI/qorven/commit/ea6d1e8))
- **Review and comment on a worker's tasks** — every task in a worker's monitor has a comment thread, so you can leave feedback or a review directly on the work. ([fd12552](https://github.com/QorvenAI/qorven/commit/fd12552))
- **Create workers with a manager** — the New Qor dialog now lets you create a specialist and choose which C-officer it reports to. ([bd28c8c](https://github.com/QorvenAI/qorven/commit/bd28c8c))
- **Workers keep their job tools** — specialists can still use external services like GitHub, Zoho, and other connectors to get work done; only direct chat and communication channels (Telegram/WhatsApp/email) are reserved for your C-officers. ([4d9d5a2](https://github.com/QorvenAI/qorven/commit/4d9d5a2))

### Changed
- The chat picker and voice switcher now list only your COO and C-officers — the executives you actually talk to. ([e8a243d](https://github.com/QorvenAI/qorven/commit/e8a243d), [2b337b5](https://github.com/QorvenAI/qorven/commit/2b337b5))
- Direct chat with a specialist worker is now blocked across every entry point — opening one takes you to its monitor instead. ([723e51c](https://github.com/QorvenAI/qorven/commit/723e51c), [d55a8eb](https://github.com/QorvenAI/qorven/commit/d55a8eb), [4da1387](https://github.com/QorvenAI/qorven/commit/4da1387))

---

## v0.5.4-alpha — 2026-06-08

### Added
- **Conversational setup** — first-run setup is now a guided chat with Prime instead of a form. Prime introduces itself, walks you through your account, workspace, AI provider, and channels one question at a time, and lets you pick which model powers your agents. ([e1b3561](https://github.com/QorvenAI/qorven/commit/e1b3561))
- **Org hierarchy with C-suite roles** — your workspace now has a clear structure: you are CEO, Prime is your COO (the agent you chat with), and dedicated C-officers run each area. Opening **/code** sets up your CTO; opening **/social** sets up your CMO. Each is created on first visit with a lightweight "Meet your CTO/CMO" card. ([a40b276](https://github.com/QorvenAI/qorven/commit/a40b276), [2c3bcb5](https://github.com/QorvenAI/qorven/commit/2c3bcb5))
- **Redesigned agent cards** — agents now appear as clean profile cards across the org chart, the Qors directory, and the sidebar: avatar, name, designation, model, and department. Hovering a card reveals capabilities, direct-report count, and usage. ([f2e87b5](https://github.com/QorvenAI/qorven/commit/f2e87b5), [d58cc25](https://github.com/QorvenAI/qorven/commit/d58cc25))
- **Model picker in setup** — after connecting a provider, choose exactly which model your COO should use from the provider's available models. ([297d2d8](https://github.com/QorvenAI/qorven/commit/297d2d8))

### Changed
- Agents no longer show "Offline" status dots — they are available 24/7. A subtle pulse appears on an agent's avatar only while it is actively working. ([124c500](https://github.com/QorvenAI/qorven/commit/124c500))

### Fixed
- Voice activity detection now loads reliably — switched to the Silero v5 model, which is compatible with the current ONNX runtime. ([0a249c2](https://github.com/QorvenAI/qorven/commit/0a249c2))
- Factory reset no longer fails on the vector database extension; the installer now grants the database user the required privilege. ([086f5e0](https://github.com/QorvenAI/qorven/commit/086f5e0), [79cca45](https://github.com/QorvenAI/qorven/commit/79cca45))
- Fixed the setup ↔ dashboard redirect loop and the page flash before redirecting to login or setup. ([46c43c8](https://github.com/QorvenAI/qorven/commit/46c43c8), [7ae9ed0](https://github.com/QorvenAI/qorven/commit/7ae9ed0))
- Provider connection in setup now shows the correct API-key format per provider and lets you switch providers if a key fails. ([e3abfc2](https://github.com/QorvenAI/qorven/commit/e3abfc2))

---

## v0.5.3-alpha — 2026-06-06

### Added
- **/undo command in chat** — type `/undo` to remove the last message pair (user + assistant) from any session. `/undo 2` removes the last 2 pairs. The agent treats the removed messages as if they never happened. Supports up to 50 pairs per undo. ([b660ec3](https://github.com/QorvenAI/qorven/commit/b660ec3))

---

## v0.5.2-alpha — 2026-06-06

### Fixed
- Factory reset no longer fails with a migration error — stale pool connections are now flushed after schema drop before re-running migrations. ([cf025fe](https://github.com/QorvenAI/qorven/commit/cf025fe))
- Redirect loop on login resolved — the `next` param is now decoded before validation and redirecting back to `/login` is explicitly blocked. ([cf025fe](https://github.com/QorvenAI/qorven/commit/cf025fe))
- Cookie max-age is now a flat 7 days instead of calculated from JWT expiry, preventing intermittent logouts caused by clock skew on first write. ([cf025fe](https://github.com/QorvenAI/qorven/commit/cf025fe))
- Session expiry buffer in middleware increased to 60 seconds to avoid false logouts during slow page navigation. ([cf025fe](https://github.com/QorvenAI/qorven/commit/cf025fe))

---

## v0.5.1-alpha — 2026-06-06

### Added
- **Password visibility toggle on reset** — new password and confirm password fields on the reset-password page now have show/hide eye icons, matching the login page. ([9b01276](https://github.com/QorvenAI/qorven/commit/9b01276))
- **Org chart — pure SVG canvas** — pan, drag, zoom, fit-to-screen, gradient avatars, live status dots, and model names. No third-party chart library. ([f93b097](https://github.com/QorvenAI/qorven/commit/f93b097))
- **Simplified password reset** — no username required. Qorven auto-resolves the admin account. Telegram-first with SSH fallback via `qorven reset-password`. ([e93a774](https://github.com/QorvenAI/qorven/commit/e93a774))

### Fixed
- Login and forgot-password pages now fit correctly on short screens (700px height) — no scrollbars, no clipped content. ([9b01276](https://github.com/QorvenAI/qorven/commit/9b01276))
- Login page right panel content compressed smartly — all features and stats visible without overflow on any viewport. ([9b01276](https://github.com/QorvenAI/qorven/commit/9b01276))
- Theme colour no longer flashes from default to custom on page load — applied before first paint via inline script. ([ed90cf7](https://github.com/QorvenAI/qorven/commit/ed90cf7))
- Expired or invalid API key errors from Telegram agents now report "Authentication error" instead of the generic "something went wrong" message. ([9b01276](https://github.com/QorvenAI/qorven/commit/9b01276))
- Sessions expired at the server now redirect to login before the page renders — middleware validates JWT expiry, not just cookie presence. ([389edf0](https://github.com/QorvenAI/qorven/commit/389edf0))
- JWT session lifetime extended to 7 days; WS 401 triggers logout instead of infinite retry. ([118a06f](https://github.com/QorvenAI/qorven/commit/118a06f))
- Duplicate WebSocket connections removed — dashboard data and task count badge now share a single connection via Zustand store. ([d1a840d](https://github.com/QorvenAI/qorven/commit/d1a840d))
- Security: rate limiter fixed (was permanently blocking after burst), voice WS authenticated, OTP removed from server logs, sessions revoked on password change, XFF header only trusted from private IPs, open redirect prevented on login. ([868cec0](https://github.com/QorvenAI/qorven/commit/868cec0))

---

## v0.5.0-alpha — 2026-06-03

### Added
- **Customisable bento dashboard** — the home page is now a fully editable operations centre. Drag, resize, and rearrange widgets freely. Layouts are saved per user. ([d0d3098](https://github.com/QorvenAI/qorven/commit/d0d3098))
- **AI widget builder** — click "Ask AI" and describe what you want in plain language. The COO agent generates a widget configuration and adds it to your dashboard instantly.
- **11 built-in widget types** — Metric card, Line/Area chart, Bar chart, Donut chart, Activity feed, Agent status grid, Tasks & approvals, Heatmap, Progress gauge, External data, Mini chat.
- **Live dashboard data** — all widgets update in real-time via WebSocket. No page refresh needed.
- **Provider budget types** — API keys now support four budget modes: Prepaid (depletes balance), Postpaid (monthly cap with auto-reset), Quota (OAuth/subscription token limits), Free (local models). ([58b0159](https://github.com/QorvenAI/qorven/commit/58b0159))
- **Per-key spend tracking** — every LLM call is attributed to the specific API key that made it. Spend counters update in real-time on provider_keys.
- **Qors page redesign** — agent cards now show model name, current spend, and capability pills (Web, Memory, Delegate). Uniform card sizes, hover-reveal actions, no empty space.
- **OAuth token store** — Claude Code, GitHub Copilot, and Gemini OAuth flows fully implemented with PKCE support and auto-refresh.

### Fixed
- Dashboard background refresh no longer clears content to skeleton state — data updates silently without blanking the page.
- VAD ONNX model load errors fixed — sidebar agent rows no longer each mount their own useMicVAD instance. Single shared instance in AgentVoicePill.
- Agent bar now collapses correctly when sidebar is collapsed.

---

## v0.4.0-alpha — 2026-06-01

### Added
- **Non-blocking chat input** — you can now send messages while an agent is actively working. Messages sent mid-run are queued and injected into the conversation at the next tool boundary, so the agent incorporates them without losing its current task context ([ca05213](https://github.com/QorvenAI/qorven/commit/ca05213), [330c995](https://github.com/QorvenAI/qorven/commit/330c995), [30203dd](https://github.com/QorvenAI/qorven/commit/30203dd))
- **Queue indicator on send button** — when a message will be queued (agent is busy), the send button turns amber with a `+` badge; hovering shows a tooltip explaining the message will be added at the next step boundary ([30203dd](https://github.com/QorvenAI/qorven/commit/30203dd))
- **Session status API** — new `GET /v1/sessions/{id}/status` endpoint returns whether a session has an active run, the current phase (thinking/tool execution), the active tool name, and iteration count ([af11825](https://github.com/QorvenAI/qorven/commit/af11825))
- **Inject message API** — new `POST /v1/sessions/{id}/inject` endpoint for programmatically injecting messages into a running agent session ([af11825](https://github.com/QorvenAI/qorven/commit/af11825))

### Improved
- **Status bar** — now shows CPU%, RAM%, and disk usage as percentages; live date+time clock; active task, session, and pending approval counts; clicking opens the System page ([a249405](https://github.com/QorvenAI/qorven/commit/a249405))
- **Injected messages persisted to session history** — messages queued mid-run are saved to the conversation log and survive page reload ([e796644](https://github.com/QorvenAI/qorven/commit/e796644))
- **Agent run cancellation wired through** — stopping a run via the API now correctly cancels the underlying execution context ([4e2b2e0](https://github.com/QorvenAI/qorven/commit/4e2b2e0))

---

## v0.3.9-alpha — 2026-05-31

### Changed
- **CLI simplified to ops-only** — removed TUI and all commands that mirrored the web UI (agents, chat, sessions, channels, rooms, tasks, workflows, auth, providers, memory, MCP, and 20+ others). The CLI now handles only system operations: `start`, `stop`, `restart`, `status`, `install`, `update`, `uninstall`, `logs`, `backup`, `restore`, `migrate`, `doctor`, `config`, `init`, `setup`, `version` ([4f846b1](https://github.com/QorvenAI/qorven/commit/4f846b1))
- **Web UI is the single interface for all agent work** — agents, chat, channels, workflows, and everything else is managed at `http://localhost:8486`

### Fixed
- **Auth redirect before page renders** — added `middleware.ts` so unauthenticated requests redirect to login at the Edge, before any page content loads ([3f14c00](https://github.com/QorvenAI/qorven/commit/3f14c00))
- **Terminal WebSocket auth** — WS connections now use `wsAuth` middleware which accepts `?token=` query param; browsers cannot send custom headers on WebSocket upgrade ([b0a997c](https://github.com/QorvenAI/qorven/commit/b0a997c))
- **Terminal connects to backend directly** — `wsBase()` uses `NEXT_PUBLIC_API_URL` in dev so WebSocket connections bypass the Next.js proxy (which cannot proxy WS upgrades) ([c01ff1d](https://github.com/QorvenAI/qorven/commit/c01ff1d))
- **Terminal escape sequences stripped** — raw ANSI/OSC codes no longer show as garbage in the simple text renderer ([c01ff1d](https://github.com/QorvenAI/qorven/commit/c01ff1d))
- **Sidebar restored on terminal page** — terminal page no longer hides the sidebar ([c01ff1d](https://github.com/QorvenAI/qorven/commit/c01ff1d))

---

## v0.3.8-alpha — 2026-05-30

### Added
- **Distill-inspired sidebar** — agents now show color-coded avatars with live pulse rings, activity subtitles ("Thinking…", last action), and per-agent voice buttons on hover ([bcaf999](https://github.com/QorvenAI/qorven/commit/bcaf999))
- **Per-agent voice sessions** — click the headphone icon next to any agent to start a voice session; switching agents automatically stops the previous session; no conflict possible ([bcaf999](https://github.com/QorvenAI/qorven/commit/bcaf999))
- **Hub rows in sidebar** — hubs appear below the agent list with stacked member avatar piles showing which agents are inside ([bcaf999](https://github.com/QorvenAI/qorven/commit/bcaf999))
- **Agent detail profile** — navigating to an agent's chat now shows a mini-profile sidebar: avatar, status ring, tokens today, recent conversations ([bcaf999](https://github.com/QorvenAI/qorven/commit/bcaf999))
- **RoomsSidebar** — the Hubs section has its own sidebar with hub list and live member panel showing agent pulse rings ([bcaf999](https://github.com/QorvenAI/qorven/commit/bcaf999))
- **Hub chat avatars** — messages in hub chats now show the sending agent's avatar and name ([bcaf999](https://github.com/QorvenAI/qorven/commit/bcaf999))
- **Chat canvas agent identity** — the empty chat state now shows the agent's avatar and a personal greeting ([bcaf999](https://github.com/QorvenAI/qorven/commit/bcaf999))

### Improved
- Voice sessions now managed globally — one active session at a time, conflict-free across all sidebar rows and the chat toolbar voice button

---

## v0.3.7-alpha — 2026-05-30

### Added
- **Hubs** — group workspaces (real-time multi-agent collaboration rooms) now appear in the main navigation rail between Channels and Org Chart ([7a0f7d1](https://github.com/QorvenAI/qorven/commit/7a0f7d1))

### Improved
- **Work Log** — filter dropdowns show plain English ("Anyone", "All areas", "All activity"); actions display as "Created", "Updated", "Ran tool" instead of raw API verbs; resource names translated to user-facing labels ([7a0f7d1](https://github.com/QorvenAI/qorven/commit/7a0f7d1))
- **Approvals** — "Block" button renamed to "Reject" on email approval cards ([7a0f7d1](https://github.com/QorvenAI/qorven/commit/7a0f7d1))
- **Models Hub** — "No keys" badge renamed to "Add API key"; "Model Alias Lookup" renamed to "Model shortcuts"; provider summary and inline descriptions use plain language ([7a0f7d1](https://github.com/QorvenAI/qorven/commit/7a0f7d1))
- **Chat** — silent failure when no AI provider is configured now shows a clear toast: "No AI provider connected. Go to Models Hub to add one." ([7a0f7d1](https://github.com/QorvenAI/qorven/commit/7a0f7d1))
- Internal term "Qors" replaced with "agents" across Tasks, Skills, Workflows, Training, and the right panel ([7a0f7d1](https://github.com/QorvenAI/qorven/commit/7a0f7d1))

---

## v0.3.6-alpha — 2026-05-30

### Added
- **macOS installer** — `curl -fsSL https://get.qorven.ai | sudo bash` now works on macOS (Intel and Apple Silicon). Homebrew installs PostgreSQL and pgvector; a launchd service handles auto-start on boot ([b8ed03d](https://github.com/QorvenAI/qorven/commit/b8ed03d))
- **Windows installer** — EDB silent MSI replaces the previous winget approach; pgvector installed from pre-built binary — no Visual Studio required ([b8ed03d](https://github.com/QorvenAI/qorven/commit/b8ed03d))

### Improved
- **Installer reliability** — handles 25 real-world edge cases: existing PostgreSQL (any version, any auth), multi-cluster service names, RHEL/Fedora/Amazon Linux, Apple Silicon Homebrew prefix, postgres.app socket paths, Windows 32-bit detection, port 5432 pre-check, NSSM fallback to sc.exe ([20177bb](https://github.com/QorvenAI/qorven/commit/20177bb))
- **Re-run safety** — re-running the installer on an existing deployment now preserves the encryption key and gateway token; previously this would silently destroy all stored credentials ([20177bb](https://github.com/QorvenAI/qorven/commit/20177bb))

---

## v0.3.5-alpha — 2026-05-28

### Added
- **Deploy persistence** — deployments survive server restarts and are queryable via API history ([f9c6afe](https://github.com/QorvenAI/qorven/commit/f9c6afe))

### Improved
- **Inline completions** — 300ms debounce prevents excessive API calls during rapid typing ([f9c6afe](https://github.com/QorvenAI/qorven/commit/f9c6afe))
- **Completion quality** — suggestions stop at logical boundaries (blank lines, closing braces, new definitions) instead of a hard line cap ([f9c6afe](https://github.com/QorvenAI/qorven/commit/f9c6afe))

### Fixed
- Subsequent file edits no longer trigger false stale-file warnings ([ce7e967](https://github.com/QorvenAI/qorven/commit/ce7e967))
- Read-before-edit guard no longer blocks write operations ([07797c8](https://github.com/QorvenAI/qorven/commit/07797c8))
- Migration compatibility with existing database schemas ([5cca81c](https://github.com/QorvenAI/qorven/commit/5cca81c), [69ab8b8](https://github.com/QorvenAI/qorven/commit/69ab8b8))

---

## v0.3.4-alpha — 2026-05-28

### Added
- **Steering files** — projects can define `.qorven/RULES.md` and `.qorven/context/*.md` files that are automatically injected into every agent session for project-specific coding standards and context
- **Live preview** — auto-detects project framework (Next.js, Vite, CRA, Go, Django, Rails, static) and starts a dev server with reverse proxy for real-time preview inside the IDE
- **Event hooks** — `.qorven/hooks.yaml` configuration for 9 lifecycle events (file created/changed/deleted, build started/completed/failed, test passed/failed, agent task completed) with run_command, run_agent, and notify actions
- **Command Center** — real-time background agent job tracking with status (queued/running/completed/failed), progress bars, duration, and cost visibility
- **One-click deploy** — generates framework-specific Dockerfiles for 8 project types, executes real Docker build and run when available, simulated edge deploy as fallback
- **Inline code completions** — AI-powered ghost text suggestions in the editor via fastest available model (haiku/flash), wired through Monaco's inline completion provider
- **Read-before-edit safety** — agents must read a file before editing it; prevents stale writes and ensures agents have current file context

### Changed
- **Deploy panel** — now features a prominent "Deploy to qorven.run" button with live status tracking (building → pushing → live), alongside existing Vercel/Netlify/GitHub Pages options
- **Code editor** — upgraded with inline suggestion support, sub-word smart completions, and AI completion provider registration

---

## v0.3.3-alpha — 2026-05-28

### Added
- **ERP governance engine** — full organizational compliance layer with designation catalog, approval matrix, policy enforcement, segregation of duties, and exception tracking ([58d7f26](https://github.com/QorvenAI/qorven/commit/58d7f26))
- **Designation catalog** — 42 pre-seeded agent positions across 8 departments with skill families, model tiers, tool permissions, and approval scopes ([58d7f26](https://github.com/QorvenAI/qorven/commit/58d7f26))
- **Approval matrix** — configurable rules for budget exceeds, model upgrades, production deploys, and policy overrides with human-required escalation paths ([58d7f26](https://github.com/QorvenAI/qorven/commit/58d7f26))
- **Policy engine** — real-time evaluation of agent actions against configurable policies with deny, warn, throttle, require-approval, and log actions ([58d7f26](https://github.com/QorvenAI/qorven/commit/58d7f26))
- **Segregation of duties** — prevents same agent from performing conflicting actions (e.g. writing code and approving its deploy) ([58d7f26](https://github.com/QorvenAI/qorven/commit/58d7f26))
- **Exception & variance dashboard** — tracks policy violations, cost overruns, and quality failures with severity classification and resolution workflow ([58d7f26](https://github.com/QorvenAI/qorven/commit/58d7f26))
- **Task state machine** — enforced lifecycle transitions (draft → submitted → routed → in_progress → completed) with full audit history ([58d7f26](https://github.com/QorvenAI/qorven/commit/58d7f26))
- **SLA tracking** — define service level targets per metric and measure compliance over time windows ([58d7f26](https://github.com/QorvenAI/qorven/commit/58d7f26))
- **Asset library** — reusable intelligence store for prompts, workflows, templates, and knowledge packs with versioning ([58d7f26](https://github.com/QorvenAI/qorven/commit/58d7f26))
- **Governance dashboard** — 4-tab UI with designation browser, approval decisions, policy event log, and exception resolution ([58d7f26](https://github.com/QorvenAI/qorven/commit/58d7f26))

---

## v0.3.2-alpha — 2026-05-27

### Added
- **Multi-relay social publishing** — connect Outstand, PostForMe, and Buffer as relay providers for social media publishing; add multiple keys per provider and multiple accounts per platform ([a915248](https://github.com/QorvenAI/qorven/commit/a915248))
- **Per-agent per-account social rules** — each agent can have dedicated voice style, content rules, knowledge context, hashtag sets, and posting guidelines for every social account it manages ([a915248](https://github.com/QorvenAI/qorven/commit/a915248))
- **Platform relay matrix** — 19 social platforms mapped to their supported relay providers with availability warnings ([a915248](https://github.com/QorvenAI/qorven/commit/a915248))
- **Pipedream integration relay** — execute connector actions through Pipedream with export guard for secure outbound data flow ([8b6308e](https://github.com/QorvenAI/qorven/commit/8b6308e))
- **Knowledge governance tools** — tiered access control for memory and knowledge with classification levels and PII vault ([8b6308e](https://github.com/QorvenAI/qorven/commit/8b6308e))
- **Integrations settings panel** — new Settings section for managing relay provider keys with multi-key card layout and connection testing ([a915248](https://github.com/QorvenAI/qorven/commit/a915248))

---

## v0.3.1-alpha — 2026-05-27

### Added
- **Memory edit** — individual memory records can now be edited inline in the Memory tab; a PATCH `/v1/memory/{id}` endpoint persists the change
- **Recalled memory panel** — the agent chat sidebar now shows which memories were retrieved for the current conversation turn

### Changed
- **Build plan file enforcement** — when the user edits the file list on the build approval screen, each pending `agent_task` node's instruction is updated to enforce that exact file set before execution begins

### Fixed
- **Provider wizard Step 1/2 validation driven by auth profile fields** — Step 1 now validates required extra fields from the auth profile; Step 2 correctly skips key entry for providers that use non-API-key auth (e.g. Bedrock)
- **Help text banner in key input** — `KeyPoolSheet` now renders `help_text` from auth profiles as a banner above the key entry form when present
- **OAuth status refresh on provider list load** — OAuth connection statuses are now derived from the actual provider list's `oauth_provider` field rather than a hardcoded list, and are refreshed every time providers load
- **OAuth popup error feedback** — if the OAuth authorization popup is blocked by the browser, or the user closes it before completing authorization, a descriptive toast is shown immediately

---

## v0.2.9-alpha — 2026-05-27

### Fixed
- **Workflow partial updates no longer corrupt records** — `PATCH /v1/workflows/{id}` now reads the existing workflow before applying the patch, preventing NULL constraint violations when only some fields are supplied
- **Memory save now reaches the database** — `POST /v1/memory/save` was passing the literal string `"default"` as the tenant ID instead of the actual tenant UUID, causing every agent memory write to fail with an invalid UUID parse error
- **Social integrations UNIQUE constraint** — added missing UNIQUE index on `(agent_id, platform, account_id)` to match the ON CONFLICT clause used by the social publishing store (migration 020)
- **`provider_configs` table now present** — migration 021 creates the table used by search providers, OAuth app config, and integration settings; its absence caused 500 errors on those endpoints

---

## v0.2.8-alpha — 2026-05-27

### Fixed
- **Channel list no longer silently drops rows** — channels with a NULL `last_error` were being excluded from list results due to a scan error; all channels now appear correctly

---

## v0.2.7-alpha — 2026-05-27

### Fixed
- **`GET /v1/channels/{id}` returns channel data** — a NULL scan on the nullable `last_error` column caused the endpoint to return 404 on every request; fixed with a pointer scan
- **Memory access controller compile fix** — `memory.SearchResult` was missing the `Classification` field referenced by the clearance filter

---

## v0.2.6-alpha — 2026-05-26

### Fixed
- **Invalid agent_id no longer crashes session creation** — `POST /v1/sessions` with a non-UUID `agent_id` (e.g. emoji, garbage text) now returns 400 instead of 500; DB errors on that path are also sanitized to avoid leaking SQL details

---

## v0.2.5-alpha — 2026-05-26

### Fixed
- **Self-update on private repositories** — the in-app Update button now works correctly; previously it failed to download assets from private GitHub repos because it relied on `browser_download_url` which is empty for private repos

---

## v0.2.4-alpha — 2026-05-26

### Fixed
- **Setup redirect loop** — after completing setup, authenticated users are now correctly redirected to the app instead of staying on the setup page
- **API key creation** — `POST /v1/user/api-keys` now works correctly (was returning 405)
- **Password change** — `POST /v1/user/password` is now reachable (was returning 404)

---

## v0.2.3-alpha — 2026-05-26

### Added
- **Autonomous execution mode** — agents can now run continuously for 1-2+ hours on complex tasks; the system checkpoints progress and self-continues past iteration limits automatically, matching the sustained execution capability of tools like Claude Code
- **Long-running session control** — new API endpoints to list and cancel autonomous sessions; code project sessions (`/code`) auto-enable autonomous mode with 2500-iteration capacity across 50 checkpoint cycles

### Fixed
- **A2A multi-turn conversations** — subsequent messages in an A2A task now actually invoke the agent and return the response; previously `handleSendMessage` appended the message but returned immediately without running the agent, leaving tasks stuck in "working" state

---

## v0.2.2-alpha — 2026-05-26

### Added
- **Live Gateway metrics stream** — Gateway Admin page now has a real-time Live Throughput card showing requests, tokens in/out, total spend, and cache hit rate; updates every 2 seconds via SSE
- **Carrier integration scaffolding** — `build_carrier` tool auto-generates ready-to-compile Go connector code for any shipping carrier; `list_carriers` shows pre-configured specs for FedEx, UPS, DHL, USPS, Aramex, and Maersk; agents guide users through the full build-and-install flow

---

## v0.2.1-alpha — 2026-05-26

### Added
- **CFO accounting system** — bank-grade cost reconciliation that verifies internal calculations match provider pricing to 100% accuracy; spend forecasting with anomaly detection; budget raise requests between agents
- **Fleet status tools** — executive agents can query live fleet health, per-agent spend, and session counts in real time
- **Hierarchical task delegation** — L1/L2 agents delegate subtasks down the org hierarchy; automatic synthesis rollup when all subtasks complete
- **Live dashboard refresh** — home dashboard auto-refreshes on agent activity, task completions, and budget warnings via WebSocket
- **Audit log enhancements** — billing and budget events tracked; improved audit detail views
- **Smart router improvements** — better model selection for tiered routing across providers

---

## v0.2.0-alpha — 2026-05-25

### Added
- **Social Media Library** — full social publishing platform with support for 12 platforms: Twitter/X, LinkedIn, Facebook, Instagram, YouTube, TikTok, Reddit, Discord, Slack, Dev.to, Medium, WordPress, Google My Business, Nostr, and Telegram Bot
- **Social analytics** — per-post metrics capture with background worker; analytics dashboard with reach, engagement, and growth charts
- **Social content sets** — reusable post templates that can be loaded into the composer with one click
- **Social team comments** — internal comment threads on post cards for team review before publishing
- **Per-integration settings** — posting hours, active days, display nickname, group, and pause toggle per social account
- **Outgoing webhooks for social events** — N8N/Make/Zapier-compatible webhook delivery on post publish, fail, and schedule events
- **GitHub Copilot provider** — connect via OAuth; supports GPT-4o, o3, Claude Sonnet 4, Gemini 2.5 Pro, and more
- **Google Vertex AI provider** — OAuth connect with full Gemini model list
- **send_file tool** — agents can deliver workspace files to users as one-click download notifications; works in both host and sandbox execution environments
- **Shell output widget** — exec tool results render as styled terminal cards with command header, exit code badge, and scrollable output; applies to both host and Docker sandbox paths
- **Fallback model** — per-agent fallback model picker in the configuration panel; automatically used when the primary model is unavailable
- **Cortex synthesis** — async post-turn knowledge extraction stores key facts and decisions as typed memories after each conversation turn
- **Usage page** — tokens and budget columns added to per-agent usage table; budget progress bars; total token and call count stat cards

### Fixed
- **Channel setup guides** — Discord and Slack guides now include all required setup steps
- **Social posting windows** — posts respect per-integration active hours and days configuration
- **Nostr signing** — Ed25519 event signing wired correctly for Nostr posts

---

## v0.1.50-alpha — 2026-05-25

### Added
- **App icons** — apps can declare an emoji or icon name in their manifest; shown on app cards and in navigation
- **Pin apps to left rail** — pin any app to the left sidebar for one-click access from anywhere
- **Pin apps to top bar** — pin apps as icon shortcuts in the header bar
- **App settings page** — schema-driven settings form at `/apps/{slug}/settings`; fields defined in `app.yaml` render automatically with no custom UI needed; values are injected as env vars into tool scripts
- **Migration 002** — new columns on the `apps` table: `icon`, `pinned_rail`, `rail_order`, `pinned_topbar`, `topbar_order`, `settings_schema`

---

## v0.1.49-alpha — 2026-05-25

### Fixed
- **App builder: jq string ID comparisons** — tool script template now explicitly warns against `tonumber` coercions when comparing IDs in jq. IDs are stored as strings; using `($id | tonumber)` caused silent no-op matches on toggle/delete operations.

---

## v0.1.48-alpha — 2026-05-25

### Fixed
- **App builder no longer stalls mid-build** — the agent loop was cutting off tool execution after 5 consecutive tool-only iterations and forcing a text reply. Building an app requires 10+ consecutive file writes, so Prime was getting interrupted before writing `bundle.js`. Code tasks now allow 20 consecutive tool iterations before the search-discipline guard fires.

---

## v0.1.47-alpha — 2026-05-25

### Fixed
- **App builder: explicit npm prohibition** — Gemini was reverting to npm even with the plain-JS bundle pattern in the prompt. Added a prominent warning block at the top of the build instructions: "Node.js and npm are NOT installed — do NOT run npm install or npm run build."

---

## v0.1.46-alpha — 2026-05-25

### Fixed
- **CI test failure** — `TestWriteRuntimeInfo_Roundtrip` was failing because the test expected `~/.qorven/runtime.json` but `config.DataDir()` now resolves to the XDG path (`~/.local/share/qorven`) on Linux. Test now pins `QORVEN_DATA_DIR` so it is not sensitive to path resolution strategy.

---

## v0.1.45-alpha — 2026-05-25

### Fixed
- **App builder no longer requires npm** — agent instructions now write the UI as a plain JavaScript IIFE bundle directly, with no Vite/npm build step. Production installs don't have Node.js installed, so the previous npm-based build always failed. The bundle pattern uses `React.createElement` and `window.__QorvenUI` components available from the host page.

---

## v0.1.44-alpha — 2026-05-25

### Fixed
- **App builder tools not executed with Gemini models** — Gemini 2.5 Flash outputs tool calls using `print(default_api.func_name(...))` Python-style syntax in `tool_code` blocks instead of structured function call responses. The narrated tool call rescue parser now handles this pattern, including multi-line triple-quoted string arguments (file contents). App builds using Gemini now execute write_file, exec, and install_app correctly.

---

## v0.1.43-alpha — 2026-05-25

### Fixed
- **App builder used wrong directory on production installs** — the agent's app-building instructions contained hardcoded `~/.qorven/apps` paths. On production installs where `QORVEN_DATA_DIR=/var/lib/qorven`, all `write_file` and `exec` steps were pointing at the wrong location. The paths now resolve from the configured data directory automatically.
- **Removed dev-only template paths from agent instructions** — steps telling the agent to `cat` template files from the source tree (`backend/cmd/scaffold/templates/...`) don't exist on installed binaries. Replaced with inline guidance.

---

## v0.1.42-alpha — 2026-05-25

### Fixed
- **App builder permission error on fresh installs** — the installer was not setting `QORVEN_DATA_DIR` in the systemd unit, so the service resolved its data directory to the home directory of the service user (which does not exist on production installs). All file writes from agents — creating apps, writing code files, running shell tools — now correctly go to `/var/lib/qorven`.

---

## v0.1.41-alpha — 2026-05-25

### Fixed
- **Installer Tailscale auth URL was never shown** — the installer was reading the wrong step result (nginx's output instead of Tailscale's), so the authorization URL never appeared and the screen fell through to showing a LAN IP instead. Fixed.
- **Port and nginx screens removed from installer** — port and nginx are now configurable from Settings in the UI after install. The install flow is: Welcome → access mode → installing → done.
- **Done screen shows both local IP and Tailscale IP** — after install completes, both addresses are shown clearly so you can open whichever is reachable.

---

## v0.1.40-alpha — 2026-05-25

### Improved
- **Data directory is now configurable and platform-aware** — Qorven resolves its data directory via the `QORVEN_DATA_DIR` environment variable. If unset, it automatically falls back to the legacy `~/.qorven` path (for existing installs) or the correct platform default: `~/.local/share/qorven` on Linux, `~/Library/Application Support/Qorven` on macOS, `%APPDATA%\Qorven` on Windows. No migration required for existing users.
- **Agent file and exec tools respect the configured data directory** — agents writing app files or running shell commands now use the resolved data path, eliminating permission errors when the service runs under a dedicated system user.

---

## v0.1.37-alpha — 2026-05-24

### Fixed
- **Providers with key-pool keys work correctly after restart** — API keys stored via Models Hub were not being loaded on startup, causing agents to 401 until the key was manually re-verified. Keys are now backfilled from the key pool at startup and after every verify.
- **OpenAI no longer rejects tools with free-form parameters** — tools like `execute_action` that accept an open key-value `params` object were being sent with strict mode enabled, causing a schema validation error. These tools are now correctly excluded from strict mode.

---

## v0.1.36-alpha — 2026-05-24

### Fixed
- **New provider keys take effect immediately** — verifying an API key now reloads the provider registry on the spot. Previously the key was only picked up after a server restart.

---

## v0.1.35-alpha — 2026-05-24

### Fixed
- **Agents no longer loop when a model returns an empty response** — if the model produces no text after using tools, the loop retries up to twice then moves on instead of spinning through all available iterations.
- **Agent self-correction now works for installed apps** — agents are told the exact file path layout for user-built apps (`~/.qorven/apps/{slug}/`) so they can locate, read, and fix app scripts without being given the path manually.

---

## v0.1.34-alpha — 2026-05-24

### Fixed
- **App Builder agents now reliably complete tasks** — ticket lifecycle tools (complete, comment, file-track) were being filtered out by the intent routing layer before reaching the agent, so agents could never mark tasks done. They now survive the filter and agents consistently close out their tasks and unblock the next step in the chain.

---

## v0.1.33-alpha — 2026-05-24

### Fixed
- **App Builder tasks now unblock and run in sequence** — when an agent completes a task, the next task in the chain is now correctly queued. Previously, tasks blocked by a dependency would stay stuck in `todo` indefinitely even after the blocker finished.
- **Intake wizard no longer repeats the tech stack question** — typing `skip` now correctly advances past the question without re-asking it.
- **Task cards are always clickable** — every task in the Build panel expands on click to show its status, agent progress, and changed files. Previously, tasks with no output yet showed nothing on click.

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
