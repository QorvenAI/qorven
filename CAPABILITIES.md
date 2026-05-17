# What Qorven can do

This is the end-user overview. For the AI-readable feature index (file paths, registries), see [`backend/FEATURES.md`](./backend/FEATURES.md). For the resilience design, see [`docs/ops/resilience.md`](./docs/ops/resilience.md). For building new UI cards, see [`docs/ui/cards.md`](./docs/ui/cards.md).

---

## talk to your agent

- **Chat** across 20 channels — web UI, Telegram, Slack, Discord, WhatsApp, email, SMS, Teams, Matrix, Signal, iMessage, LINE, Feishu, DingTalk, WeChat Work, Zalo, Facebook Messenger, Webchat, Webhook, GitHub.
- **Voice** — push-to-talk microphone in any chat input. Prime voice widget on the sidebar for hands-free. TTS + STT across 18 providers (OpenAI, ElevenLabs, Cartesia, Deepgram, AssemblyAI, Groq, Kokoro, Piper, faster-whisper, Moonshine, Ollama, HuggingFace, Microsoft Edge, and any OpenAI-compatible service). Full-duplex realtime via OpenAI Realtime or Gemini Live.
- **Voice Activity Detection** — tunable thresholds + barge-in so the agent stops talking when you do.

## let the agent do things

- **File system** — read, write, edit, patch, search (ripgrep-fast glob/grep), diff, undo, diagnostics (`go build`, `tsc`, `cargo check`).
- **Shell** — run commands, background processes, sandboxed or host-bound.
- **Web scraping** — 5-layer anti-bot stack (TLS/JA3, HTTP/2, header profiles, go-rod stealth, proxy rotation) with automatic escalation from HTTP to headless browser.
- **Browser automation** — coordinate-based click/type/scroll/screenshot/JS eval via 8 primitives, plus a `computer_use` one-call do-and-see wrapper. Vision-driven `browse_and_act` auto-selects screenshot mode on vision-capable models (Claude 3+/4, GPT-4o/5, Gemini 1.5+, Nova Pro, Pixtral), falls back to accessibility-tree mode on text-only models. Stale-session auto-recovery so a closed tab doesn't wedge the loop.
- **Cloud storage** — 70+ backends via rclone (S3, GCS, Azure, Dropbox, Google Drive, OneDrive, Backblaze, SFTP, WebDAV, more). List / read / write / copy / sync. Credentials stay in rclone.conf; Qorven never writes them.
- **Databases** — talk to your SQL (Postgres + SQLite). `sql_connections` lists configured DBs, `sql_schema` shows structure, `sql_query` runs the query. Writes require `confirm="YES-WRITE"`; connections can be marked read-only for extra safety. Parameterised to prevent injection.
- **Cross-platform messaging** — send DMs, post to rooms, draft emails, monitor social mentions.
- **GitHub** — read/create issues + PRs, open repos, push files, merge PRs, check CI status.

## understand things

- **Research** — decomposes a question into parallel sub-queries, runs them, synthesises a citation-backed answer.
- **Codebase digest** — pack a repo into one LLM-ready markdown blob that respects `.gitignore` and skips binaries.
- **Read PDFs, DOCX, text** — layout-preserving extraction with heading detection.
- **Read images, audio, video** — via vision / STT providers.
- **Knowledge graph** — entity + relationship extraction, PageRank-style centrality, Leiden community detection, exports to JSON / GraphML / Cypher / Obsidian markdown.
- **Memory** — typed facts with certainty scores, pgvector semantic search, hierarchical (company → team → agent), working memory for short-term events.

## get information

- **Weather** — current + 7-day forecast for any location, no API key needed (Open-Meteo).
- **Flights / hotels / cars / trains** — no paid APIs; pure scraping via the anti-bot stack.
- **Social monitoring** — GitHub, Reddit, Hacker News, Twitter, GitHub Trends.

## rich chat rendering

Answers that carry structured data render as cards, not walls of text:

- **Weather card** — current + forecast, animated
- **Flight / hotel / car / transit cards** — comparison rows with prices, booking links, stops/amenities
- **Sports score card** — live badge, team logos, venue
- **SQL result card** — sortable client-side, truncation-aware
- **Screenshot card** — fullscreen expand + download, for both browser captures and generated images
- **Browser step card** — per-step replay of automation runs (action icon + thoughts + thumbnail)
- **Artifact preview** — HTML / SVG / Mermaid rendered inline with fullscreen toggle
- **Rich cards for YouTube, GitHub repos, link previews, diffs, terminals, JSON, callouts, quotes, stats, maps, timelines, tasks, calendar events, files, agents, progress bars**

Inline `[1]` `[2]` citation chips that scroll to the source on click. Streaming caret that follows the LLM's tokens. Tool-call expandables with favicon source pills for web results. Thinking block with elapsed-time label.

## automate

- **Cron jobs** — agents can schedule themselves: "send me the morning briefing at 9am on weekdays."
- **Workflows** — DAG execution with variable interpolation and conditional branching.
- **Scenarios** (Labs) — multi-agent simulations with personas for "what would each of these three reviewers say about this PR?" style flows. Experimental; see `/labs`.
- **Background sessions** — long-running agent tasks with their own workspace and environment.
- **Learning loop** — per-task retrospectives that feed back into memory.

## build your own agents

- **Custom tools** — define a tool at runtime via the tool store, schema-validated.
- **WASM plugins** — sandboxed 4 MiB / 2s tools loaded on demand.
- **Skills** — 17 built-in prompt templates (summarise, rewrite, code review, pros-cons, decision journal, write tests, debug, commit messages, extract structured data, research, post-mortem, deploy checklist, plan-before-act, and more). All editable on disk; the gateway only overwrites if you haven't touched them.
- **Prime delegation** — a Chief-of-Staff agent routes tasks to specialists; you see the plan graph in the chat.
- **Spawn** — agents can spawn child agents mid-task.

## privacy + safety

All opt-in, all per-tenant, all off by default unless noted:

- **PII redaction** — mask emails, phones, SSNs, credit cards (Luhn-validated), IBANs (mod-97-validated), IPs in user messages and tool output before the model sees them
- **Prompt-injection defense** — 13 attack-pattern rules across 5 categories. Four policies: off / warn / block / strict.
- **Credential scrubbing** — tool output is scanned for API keys, bearer tokens, connection strings, AWS creds before reaching the model
- **Sandbox routing** — tools can run inside a container (Docker / WASM) when configured
- **Approval gates** — dangerous tool calls (destructive exec, outbound email/post) pause for user approval
- **Role-based permissions** — Chief / Director / Specialist roles with fine-grained permissions
- **Multi-tenancy** — PostgreSQL row-level security, tenant-scoped background sessions

## resilience

- **Port probe** — the backend walks `+0..+10` before giving up on a bound port
- **Runtime discovery** — web client finds the backend port automatically via `/__qorven_runtime`, no env edits needed
- **Graceful shutdown** — SIGTERM drains in-flight streams, second signal force-exits
- **WebSocket heartbeat** — 20s ping detects dead connections long before TCP keepalive (2h default)
- **Exponential backoff reconnect** — frontend reconnects with jitter, max 30s, unlimited retries
- **Reconnect banner** — amber "reconnecting" indicator after 5s of disconnect; green flash on recovery
- **Atomic config writes** — config never truncated mid-crash
- **Tool-call JSON validation** — truncated streaming args rejected, not executed with empty map
- **Background subprocess cleanup** — `exec` tool children SIGTERMed on gateway shutdown

See [docs/ops/resilience.md](./docs/ops/resilience.md) for the full story.

## connect to your world

- **Connectors** — manifest-driven SaaS integrations: Salesforce, HubSpot, Notion, Linear, Jira, GitHub, and more via OAuth.
- **MCP** — Model Context Protocol server + client; consume external MCP servers and expose Qorven's own tools as MCP.
- **Cross-instance federation** — A2A protocol lets agents on different Qorven deployments discover and delegate to each other.

## deployment

- **Single binary** + PostgreSQL 15 with pgvector. No microservices, no sidecars.
- **`qorven start`** — walks configured port → +10 on conflict.
- **Docker Compose** — restart: unless-stopped, /livez healthchecks, web depends on backend.
- **Systemd** — unit file at `backend/scripts/qorven.service` with `Restart=on-failure`, hardening defaults.
- **`air` hot-reload** — `make dev-watch` for development.
- **`scripts/dev-up.sh`** — port-check preflight with `--kill` / `--check-only`.

---

When this drifts, regenerate from `backend/FEATURES.md` or search the live registries per that file's `#discovery` section.
