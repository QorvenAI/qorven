# Qorven — Feature Reference

Machine-readable capability index. Read this when a user asks "can Qorven do X?" or "where does Y live?". Every entry cites a file path — grep to verify.

When this drifts from reality, regenerate by scanning the live registries. Section anchors are stable; link with `#section-name`.

---

## architecture

- Single Go binary + PostgreSQL 15+ with pgvector.
- Gateway is `chi` router in `internal/gateway/gateway.go`.
- Agent loop is `internal/agent/loop.go` — think → act → observe, streaming end-to-end.
- State: agents, sessions, messages, tools, memories in Postgres. Vectors in pgvector.
- Deploy: `qorven start`. No microservices, no sidecars, no Python/Node runtime needed.

---

## tools

Registered in `internal/tools/*`. Exposed to the LLM via function-calling. Return `*Result`. Tools marked **sandbox** route through a container when `sandbox_manager` is configured.

### file ops

| Tool | Purpose | Sandbox |
|---|---|---|
| `read_file` | Read file contents, optional line range | yes |
| `write_file` | Create or overwrite, auto-creates parent dirs | yes |
| `edit` | Find-and-replace by exact text match, returns diff | yes |
| `list_files` | Directory listing with metadata | yes |
| `glob` | Find files by pattern (`**/*.go`) — ripgrep fast path | yes |
| `grep` | Regex search over file contents — ripgrep fast path | yes |
| `apply_patch` | Atomic multi-file unified-diff patch with rollback | yes |
| `undo` | Revert last file edit (per-session history) | yes |
| `diagnostics` | Run language build/lint (`go build`, `tsc`, `cargo check`) | yes |
| `read_image` | Describe an image via vision LLM | no |
| `read_document` | Extract text from PDF/DOCX/XLSX | yes |

### code navigation & project

| Tool | Purpose |
|---|---|
| `lsp_bridge` | LSP — go-to-definition, references, hover, symbols |
| `project_manager` | Manage code projects, scan tests + build configs |
| `workspace_builder` | Create isolated dev workspaces |
| `self_knowledge` | Query Qorven's own codebase (packages, schema, git log) |
| `self_patch` | Propose changes on a git branch, auto-revert on build failure |
| `self_improve` | Analyze codebase, suggest improvements, defer to `self_patch` |

### execution & spawning

| Tool | Purpose |
|---|---|
| `exec` | Shell command, optional `background=true` |
| `spawn` | Start a child agent with a directive |
| `delegate` | Delegate a task to another agent |

Background `exec` children are tracked and SIGTERMed on shutdown (see [resilience](#resilience)).

### web scraping & research

See [web_scraping](#web_scraping) for the engine stack underneath these tools.

| Tool | Purpose | Auth |
|---|---|---|
| `web_search` | Multi-provider web search with citations | provider-specific |
| `web_fetch` | Fetch URL, HTML→markdown, 5-layer anti-bot | none |
| `qor_crawl` | Deep-crawl + structured extraction | none |
| `scrape` | Extract tables, prices, specs | none |
| `research` | Decompose question into parallel sub-queries, synthesize | provider-specific |
| `qor_browse` | Autonomous browser agent (headless) | none |
| `browse_and_act` | Navigate, fill forms, click, extract | none |
| `browser` | Interactive browsing with snapshot capture | none |

### communication

| Tool | Purpose |
|---|---|
| `email_send` | Send via configured SMTP |
| `email_read` | Poll inbox, flag suspicious replies |
| `message` | DM another user in the workspace |
| `team_message` | Post to a team room |
| `create_image` | Generate image from text |

### social (post + monitor)

See [social_platforms](#social_platforms) for the full platform list.

| Tool | Purpose |
|---|---|
| `social_publish` | Post to 11 platforms (Twitter/X, LinkedIn, Facebook, Instagram, Threads, TikTok, YouTube, Reddit, Mastodon, Bluesky, Pinterest) |
| `social_monitor` | Watch mentions/replies across GitHub, Reddit, HackerNews, Twitter, GitHub Trends |

### memory & knowledge graph

| Tool | Purpose |
|---|---|
| `memory_search` | Keyword + semantic search over agent memories |
| `memory_get` | Fetch a memory by ID with metadata |
| `knowledge_graph_search` | Query KG by entity, relationship, or semantic match |

### rooms & teams

| Tool | Purpose |
|---|---|
| `room_post` | Post to a room |
| `room_list` | List accessible rooms |
| `room_decide` | Cast a vote in a room poll |
| `room_assign` | Assign a task within a room |
| `join_room` / `leave_room` | Session management |
| `team_tasks` | Create, list, update team tasks |

### github

All require a connected GitHub credential (`/v1/connections`).

| Tool | Purpose |
|---|---|
| `gh_repo_info` | Repo metadata |
| `gh_list_issues` | List issues/PRs with filters |
| `gh_read_issue` | Issue/PR detail + comments |
| `gh_create_issue` | New issue with labels + assignee |
| `gh_create_branch` | New branch |
| `gh_push_file` | Commit + push a file |
| `gh_open_pr` | Open a PR |
| `gh_post_comment` | Comment on issue/PR |
| `gh_list_pr_checks` | CI status for a PR |
| `gh_merge_pr` | Merge (squash/rebase) |
| `gh_create_repo` | Create a repo |
| `gh_task_register` | Link GitHub issue to Qorven task |

### travel

| Tool | Purpose |
|---|---|
| `flight_search` | Google Flights lookup — `from`, `to`, `date` |

(Hotels, cars, trains: not present today.)

### qorven platform

| Tool | Purpose |
|---|---|
| `qorven_lint` | Lint Qorven config files |
| `qorven_report` | Generate system-state reports |
| `qorven_wiki` | Query the project wiki |
| `qorven_download` | Download from Qorven storage |
| `qorven_fly` | Deploy to Fly.io |

### skills

| Tool | Purpose |
|---|---|
| `skill_search` | Find installed skills by keyword |
| `list_agents` | List agents visible to this one |

### custom tools

Users define tools at runtime via the Custom Tool Store (`internal/tools/custom_store.go`). Validated against a JSON schema before registration.

### tool safety

- **Credential scrubbing** — regex-based masking of API keys, bearer tokens, connection strings, AWS creds, env var patterns (`internal/tools/scrubber`).
- **Tool budget** — per-result + per-turn byte caps (`internal/agent/tool_budget.go`).
- **Loop guard** — detects repeat-call loops, hallucinated action claims (`internal/agent/loop_guard.go`).
- **Permission hook** — dangerous calls route through approval gate.
- **Tool-call JSON validation** — truncated streaming args rejected, not executed with empty map (`internal/providers/toolcall_parse.go`).
- **Quota enforcement** — workspace write limits, file size caps (`internal/tools/security.go`).

---

## web_scraping

The stack under `web_fetch` / `qor_crawl` / `scrape`. Five layers of anti-bot, with HTTP→browser escalation when a site blocks.

| Layer | File | Purpose |
|---|---|---|
| **Engine router** | `internal/scraper/engine_router.go` | URL/MIME → engine map (http / browser / pdf / document). Overrides via `~/.qorven/engine-domains.json`. |
| **TLS/JA3 spoofing** | `internal/scraper/antibot.go` | Chrome/Firefox cipher shuffling via `req` + `utls` |
| **HTTP/2 frame matching** | `internal/scraper/antibot.go` | SETTINGS + PRIORITY frames that look like real browsers |
| **Header/fingerprint rotation** | `internal/scraper/fingerprint.go` | Consistent UA + sec-ch-ua + accept-language profiles |
| **Stealth headless** | `internal/scraper/stealth.go` | go-rod + `go-rod/stealth` patches (navigator.webdriver, plugins, canvas) |
| **Proxy rotation** | `internal/scraper/stealth.go:ProxyRotator` | Round-robin proxy pool with request jitter |

Escalation: HTTP attempt → 403/429 → retry with stealth headless → proxy rotation.

Not present: Cloudflare JS-challenge solver, captcha solving.

---

## browser_automation

Under `internal/qor/browser/`. Based on **go-rod** (Chrome DevTools Protocol). Used by `qor_browse`, `browse_and_act`, and `browser` tools.

| File | Capability |
|---|---|
| `browser.go` | Chromium launcher + lifecycle |
| `page.go` | Navigation, wait-for, URL control |
| `actions.go` | Click, fill, submit, keyboard, scroll |
| `snapshot.go` | Screenshot + accessibility-tree DOM capture |
| `tabs.go` | Multi-tab management |
| `remote.go` | Attach to remote Chrome via CDP URL |
| `tool.go` | Tool wrapper exposed to the LLM |

Autonomous page understanding: JS eval + accessibility-tree parsing for "click the Submit button" style natural-language actions.

---

## research_engine

Deep-search / decomposition lives under `internal/qor/deepsearch/`.

- **SearchGraph** — parent→children query tree. Root is the user's question; children are LLM-decomposed sub-queries.
- **DecomposeFunc** — LLM call that breaks a question into narrower parallel sub-queries.
- **SearchFunc** — per-sub-query executor (plugs into the web search or knowledge graph).
- **SynthesizeFunc** — LLM call that merges sub-results into a cited final answer.

Exposed via the `research` tool.

---

## llm_providers

Native adapters in `internal/providers/`. Each implements `Provider` with `Chat` and `ChatStream`.

| Provider | File | Native? | Streaming | Tool use | Thinking |
|---|---|---|---|---|---|
| OpenAI | `openai.go` | native | yes | yes | reasoning_effort |
| Anthropic | `anthropic.go` | native | yes | yes | extended_thinking |
| AWS Bedrock | `bedrock.go` | native (Converse API) | yes | yes | varies by model |
| Google Gemini | `gemini.go` | native | yes | yes | thinking |
| DashScope | `dashscope.go` | native (Qwen) | yes | yes | no |
| OpenAI-compatible | `openai.go` (base URL) | compat | yes | yes | varies |

OpenAI-compat covers: Groq, DeepSeek, Mistral, xAI (Grok), Azure OpenAI, Anyscale, Together, Fireworks, OpenRouter, LocalAI, LM Studio, Ollama, vLLM, LiteLLM proxy.

### model catalog

`model_registry.go` + `models_catalog.json` hold ~1,700 model entries. Per model: input/output token limits, $/1M input, $/1M output, tool-use support, vision support, reasoning support. Consumed by `smart_router.go` for cost/latency-aware routing.

### provider features

- **Streaming** — all providers
- **Prompt caching** — Anthropic `ephemeral` cache control on stable system prompts
- **Prefill** — Anthropic/OpenAI assistant prefill for guided output
- **Failover** — adjacent-model fallback on 5xx
- **Credential pooling** — multiple API keys per provider, round-robin with rate-limit awareness
- **Batch API** — OpenAI + Anthropic batch endpoints
- **Tool-call parse-error surfacing** — truncated JSON args flagged, not silently executed

Not present: dynamic model hot-swap (llama-swap-style GPU-resident model rotation).

---

## channels

Each channel is `internal/channels/<name>/`. Every channel implements `Channel` with `Start`, `Stop`, `Send`, `Receive`.

| Channel | Inbound | Outbound | Media |
|---|---|---|---|
| Telegram | bot API polling | bot API | photo, video, doc, voice |
| Email (SMTP/IMAP) | IMAP folder polling | SMTP | attachments |
| Slack | Events API / RTM | web API | files, images |
| Discord | gateway WS | REST API | files, embeds |
| WhatsApp | Cloud API webhook | Cloud API | photo, audio, doc |
| Teams (Microsoft) | Bot Framework webhook | Graph API | yes |
| Webchat (built-in) | authenticated WebSocket | WebSocket | yes |
| SMS (Twilio) | webhook | REST API | text only |
| Webhook (generic) | HTTP POST (HMAC) | HTTP POST | yes |
| GitHub (as channel) | webhook | REST API | attachments |
| Matrix | HTTP long-poll | client-server API | yes |
| Signal | stored-message polling | native API | no |
| iMessage | polling (local node) | native API | yes |
| Mattermost | incoming webhook | REST API | yes |
| LINE | webhook | messaging API | yes |
| Feishu / Lark | webhook + polling | open API | yes |
| DingTalk | webhook (robot code) | open API | yes |
| WeChat Work | verified webhook | open API | yes |
| Zalo | polling | open API | yes |
| Facebook Messenger | webhook | Graph API | yes |

### channel orchestration

`internal/channels/` top-level:

| File | Purpose |
|---|---|
| `channel.go` | `Channel` interface + registry |
| `dispatch.go` | Message → channel routing |
| `events.go` | Unified inbound-event schema |
| `ratelimit.go` | Per-user / per-group rate limits |
| `retry.go` | Backoff + redelivery |
| `approval.go` | State machine for human-approved outbound actions |
| `media_utils.go` | Unified media handling across channels |

---

## social_platforms

Under `internal/qor/social/`.

### post-capable

Tool: `social_publish`. File: `publish.go`.

- Twitter / X
- LinkedIn
- Facebook
- Instagram
- Threads
- TikTok
- YouTube
- Reddit
- Mastodon
- Bluesky
- Pinterest

### read-only monitoring

Tool: `social_monitor`. File: `reader.go`. Pluggable via `PlatformReader` interface.

- GitHub (repo search + trending)
- Reddit
- Hacker News
- Twitter / X
- GitHub Trends

---

## voice

DB-driven at runtime (`voice_providers` table). UI form schemas in `voice_catalog.json`.

### tts drivers

| Driver | Type | Notes |
|---|---|---|
| OpenAI | cloud native | `tts-1`, `tts-1-hd`, `gpt-4o-mini-tts` |
| ElevenLabs | cloud native | streaming, voice cloning |
| Cartesia | cloud native | `sonic-3`, ~90ms latency |
| Microsoft Edge TTS | free local | `edge-tts` CLI |
| Piper | local | CPU-only, ~20-60MB voices |
| Kokoro | local | 82M params, Apache 2.0 |
| Ollama | local | any Ollama voice model |
| HuggingFace Inference | cloud | bring-your-own |
| OpenAI-compatible | proxy | Groq, LocalAI, LMNT |

### stt drivers

| Driver | Type | Notes |
|---|---|---|
| OpenAI Whisper | cloud | `whisper-1`, `gpt-4o-transcribe` |
| Groq Whisper | cloud | whisper-large-v3-turbo |
| Deepgram | cloud | streaming, nova-3 |
| AssemblyAI | cloud | streaming, diarization hook, sentiment |
| faster-whisper | local | whisper.cpp successor |
| Moonshine | local | 26-245M params, CPU-optimized |
| Ollama | local | whisper via Ollama |
| HuggingFace Inference | cloud | bring-your-own |
| OpenAI-compatible | proxy | any `/v1/audio/transcriptions` |

### realtime (full-duplex)

| Driver | Transport | Model |
|---|---|---|
| OpenAI Realtime | WebSocket or WebRTC | `gpt-4o-realtime-preview` |
| Gemini Live | WebSocket | Gemini realtime |

### telephony

| Driver | Purpose |
|---|---|
| Twilio | inbound/outbound PSTN call via `internal/voice/twilio.go` |
| Plivo | inbound/outbound PSTN call via `internal/voice/plivo.go` |

### webrtc peer voice

- SDP offer/answer + ICE signaling at `/ws/voice/webrtc`
- Falls back to WebSocket audio when full WebRTC unavailable
- **Not present**: screen share, voice cloning tool, voice-activity-detection

---

## search_providers

Generic multi-provider search for the `web_search` tool. Files under `internal/search/providers.go`.

- Brave Search
- Exa.ai (neural)
- Jina AI reader
- Kagi
- Google (via search pipeline)
- Tavily
- DuckDuckGo
- SearXNG
- Serper
- Perplexity
- Bing

Each is a named provider in the registry; the smart router picks one based on cost/quality/availability.

---

## gateway_routes

Route registration in `internal/gateway/gateway.go:registerRoutes`. Most require Bearer token; exceptions noted.

### health & runtime

| Path | Purpose |
|---|---|
| `GET /health` | Legacy (compat) |
| `GET /health/detailed` | Full subsystem health |
| `GET /livez` | Process-alive probe (k8s, no deps) |
| `GET /readyz` | Ready-to-serve (DB + voice checked) |
| `GET /__qorven_runtime` | Port discovery for web client |
| `GET /metrics` | Prometheus metrics |
| `GET /openapi.json` | OpenAPI 3.0 spec |

### auth

| Path | Purpose |
|---|---|
| `POST /auth/login` | Password login (10/min/IP) |
| `POST /auth/setup` | First-time admin setup |
| `GET /auth/setup-check` | Has setup completed? |
| `POST /auth/logout` | Revoke token |
| `POST /auth/refresh` | Refresh JWT |
| `GET /auth/me` | Current user |
| `POST /auth/change-password` | Self-serve password change |
| `POST /auth/api-keys` | Mint API key |
| `POST /auth/forgot-password` | Magic-link reset (3/min/IP) |
| `POST /auth/reset-password` | Consume reset token |

### v1 api (token required)

| Prefix | Purpose |
|---|---|
| `/v1/agents/*` | Agent CRUD, system prompt, tool profiles, avatar |
| `/v1/sessions/*` | Session CRUD, history, message append |
| `/v1/chat` | Streaming or non-streaming chat completion |
| `/v1/models/*` | Model list, pricing, selected-model |
| `/v1/providers/*` | Provider config + test |
| `/v1/skills/*` | Skill search, install |
| `/v1/memory/*` | Memory search, get, forget |
| `/v1/knowledge-graph/*` | KG queries, entity CRUD, export (JSON / GraphML / Cypher / Obsidian) |
| `/v1/rooms/*` | Room CRUD, join, post, decide |
| `/v1/tasks/*` | Task queue |
| `/v1/projects/*` | Code project registry |
| `/v1/workflows/*` | Workflow def + execution |
| `/v1/voice/*` | Voice provider catalog, TTS, STT, config |
| `/v1/approvals/*` | Outbound-action approval queue |
| `/v1/audit/*` | Audit log queries |
| `/v1/admin/*` | Admin-only (metrics, config) |
| `/v1/connections/*` | OAuth connections |
| `/v1/setup/finalize` | Setup wizard completion (HMAC) |
| `/v1/billing/*` | Usage + cost + budget |

### websockets

| Path | Purpose |
|---|---|
| `GET /ws` | JSON-RPC agent messaging + tool streaming |
| `GET /ws/realtime` | Hub broadcast (activity, approvals, room messages) |
| `GET /ws/voice` | Agent voice chat (STT → agent → TTS) |
| `GET /ws/voice/webrtc` | WebRTC peer voice signaling |
| `GET /ws/voice/realtime` | Proxy to OpenAI/Gemini realtime WS |

Auth: `?token=<jwt>`. Server pings every 20s with 10s timeout.

### inbound webhooks

| Path | Purpose |
|---|---|
| `POST /v1/webhooks/mail/inbound` | Inbound email (HMAC) |
| `POST /v1/webhooks/github/{id}` | GitHub events (secret-verified) |
| `POST /v1/webhooks/{channel}/inbound` | Per-channel webhook |

### a2a protocol

`/a2a/*` — Agent-to-Agent discovery + task endpoints for cross-instance federation.

---

## agent_loop_mechanisms

| Mechanism | File | Purpose |
|---|---|---|
| Core loop | `internal/agent/loop.go` | Think → act → observe with streaming |
| Parallel tools | `loop_parallel.go` | Read-only tools run concurrently |
| Tool budget | `tool_budget.go` | Per-result + per-turn byte caps |
| Loop guard | `loop_guard.go` | Detects repeat-call loops, hallucinated actions |
| Compactor | `compactor.go` | Iterative context summarization |
| Dreamer | `../memory/dreamer.go` | Background memory consolidation |
| Learning loop | `learning_loop.go` | Post-task retrospective → memory/skills |
| QOROS | `qoros.go` | Quiet operational reasoning + specialist spawning |
| Crystallizer | `../skills/` | Auto-create skills from success patterns |
| Skill evolver | `skill_evolver.go` | Self-improving skill synthesis |
| Plan hook | `plan_hook.go` | Plan mode — stepwise reasoning with approvals |
| Permission hook | `permission_hook.go` | Route dangerous calls through approval gate |
| Prime delegation | `prime_delegation.go` | Chief-of-Staff → specialists |
| Heartbeat | `../heartbeat/` | Per-agent liveness pings |
| MCP user | `loop_mcp_user.go` | Load MCP tools with per-user credentials |
| Input guards | `input_guard.go` | Sanitize input before LLM |
| Media handling | `loop_media.go` / `loop_media_dedup.go` | Images/PDFs with dedup across turns |
| Prompt cache | `prompt_cache.go` | Reuse cached system prompts (Anthropic) |
| Tool guidance | `tool_guidance.go` | Hints on when to use which tool |
| Tool timing | `tool_timing.go` | Slow-tool detection + user notification |
| Tool result truncation | `tool_result_truncation.go` | Size cap with `truncated=true` |
| Self patch | `self_patch.go` | Mid-loop self-correction on build failure |
| Background model | `loop.go` | Cheap model for titles/tags/intent |
| Smart router | `../providers/smart_router.go` | Cheapest/fastest model per request |
| Empty-response recovery | `loop.go` | Substitute apology on empty LLM reply (every iteration) |

---

## memory_and_knowledge

| Component | File | Purpose |
|---|---|---|
| Typed memories | `internal/memory/backend_pg.go` | Facts with certainty (0–1), auto-expire |
| Embeddings | `embedding.go` | pgvector semantic search |
| Knowledge graph | `graph.go` / `knowledge_graph.go` | Entity + relationship graph |
| Entity extractor | `entity_extractor.go` | LLM-driven extraction |
| Hierarchy | `hierarchy.go` | Company → team → agent inheritance |
| Working memory | `working.go` | Short-term, LRU |
| Certainty framework | `certainty.go` | Evidence-based confidence |
| Chunker | `chunker.go` | Semantic chunking |
| Snapshot | `snapshot.go` | Per-session memory snapshots |
| Digest | `digest.go` | Auto-compression + summarization |
| Curator | `curated.go` | High-value fact list |
| Taxonomy | `taxonomy.go` | Hierarchical classification |

### knowledge graph analytics

Under `internal/knowledgegraph/`:

- **God-node / PageRank proxy** — degree centrality in `analysis.go`
- **SimpleLeiden** — BFS-based community detection (simplified Leiden)
- **Betweenness-centrality proxy** — cross-community bridge scoring (`surpriseScore`)
- **Cohesion** — intra-community edge density (`CohesionScore`)
- **Semantic similarity** — `similarity.go`
- **Diff** — change tracking between graph snapshots (`diff.go`)

### export formats

- JSON
- GraphML (XML)
- Neo4j Cypher
- Obsidian markdown with wiki-links (`wiki.go`)

---

## supporting_subsystems

| Subsystem | Path | Purpose |
|---|---|---|
| Billing | `internal/billing` | Cost + token tracking, budget enforcement (`CheckBudget`, `EnforceBudget`), per-agent/session cost summary |
| Vault | `internal/vault` | Encrypted credential storage |
| OAuth | `internal/oauth` | Multi-provider OAuth flows |
| Audit | `internal/audit` | Structured log of tool calls, decisions, approvals |
| Permissions | `internal/permissions` | RBAC + ABAC, Chief/Director/Specialist roles with per-action permissions |
| Approvals | `internal/approvals` + `channels/approval.go` | Outbound-action approval gate with state machine |
| Plugins (WASM) | `internal/plugins/wasm` | wazero runtime, 64-page memory cap, 2s CPU cap, no net/fs, JSON stdio contract |
| Scenarios | `internal/scenario` | Multi-agent simulation with personas, multi-round conversations, LLM-driven |
| Workflows | `internal/workflow` | DAG executor, variable interpolation, tool steps, conditional branching |
| Cron | `internal/cron` | Scheduled background tasks, time triggers |
| Sandbox | `internal/sandbox` | Container/VM isolation for tools |
| Sessions | `internal/session` | User/agent conversation store with message history |
| MCP | `internal/mcp` | Model Context Protocol server + client |
| Supervisor | `internal/supervisor` | Inter-agent protocol + oversight |
| Realtime hub | `internal/realtime` | WebSocket broadcast |
| Skills | `internal/skills` | Skill store, installer, evolver |
| Connectors | `internal/connectors` | Manifest-driven SaaS integrations (Salesforce, HubSpot, Notion, Linear, Jira, GitHub, etc.) |
| Calendar | `internal/calendar` | Event management + scheduling |
| Drive | `internal/drive` | File storage |
| Mail | `internal/mail` | Email threading + context preservation |
| RAG | `internal/rag` | Retrieval-augmented generation |
| Search | `internal/search` | Full-text search over memories, KG, docs |
| QOR browser | `internal/qor/browser` | Headless browser automation |
| QOR social | `internal/qor/social` | Social posting + monitoring |
| QOR deepsearch | `internal/qor/deepsearch` | Graph-decomposed parallel research |
| Coworker vault | `internal/qor/coworker/vault.go` | Per-agent secrets compartment |
| Notifications | `internal/notifications` | DM, email, in-app |
| Tasks | `internal/tasks` | Task queue with lifecycle |
| Service accounts | `internal/serviceaccounts` | API service account management |
| TLS | `internal/tls` | ACME autocert (Let's Encrypt) |
| A2A | `internal/a2a` | Agent-to-agent federation |
| Flight | `internal/flight/flights` | Google Flights integration |

---

## connectors

Manifest-driven third-party integrations under `internal/connectors/`. Each connector declares: ID, name, icon, category (developer/workplace/data/commerce/infra/productivity), auth schema (api_key / oauth2 / basic / none), actions, triggers.

Categories present: developer tools, workplace/CRM, data warehouses, commerce, infrastructure, productivity. Connector list is seeded from `seed.go` (Salesforce, HubSpot, Notion, Linear, Jira, GitHub, and more).

---

## observability

Under `internal/tracing/`.

- **OpenTelemetry** — tracer initialized with stdout exporter; OTLP pluggable for production
- **Spans** — `agent.run`, `tool.*`, `llm.chat` with structured attributes
- **Token metrics** — input/output counts recorded per span
- **Snapshot worker** — aggregates trace-level metrics (per-agent/per-channel, per-provider/per-model)
- **Prometheus** — `/metrics` exports counters, gauges, histograms
- **Structured logs** — `slog` throughout, JSON-formatted
- **Audit trail** — separate from logs; tool calls, decisions, approvals land in the audit store

---

## multi_tenancy

Row-level security with tenant-scoped resources.

- **RLS policies** — `tenant_id` column on every multi-tenant table, PostgreSQL RLS policies (migration 040)
- **Permission gate** — `internal/permissions/` enforces RLS on every query
- **Role model** — Chief / Director / Specialist with granular permissions (`tool.execute`, `agent.delegate`, `task.create`, `budget.manage`, etc.)
- **Tenant isolation tests** — `NewIsolatedTenant()` in `testsupport/` confirms data boundaries in integration tests
- **Background sessions** — keyed by `tenantID/sessionID` on both the in-memory map and the on-disk workspace path (`agent/background_session.go:CreateForTenant`)

---

## i18n

Under `internal/i18n/`. Three languages: **English (en)**, **Vietnamese (vi)**, **Chinese (zh)**. Catalog files: `catalog_en.go`, `catalog_vi.go`, `catalog_zh.go`. Locale-aware lookup with prefix fallback (en-US → en). Keys declared in `keys.go`.

---

## cli_and_tui

Under `cmd/`. Available subcommands:

`chat`, `agents`, `sessions`, `auth`, `config`, `doctor`, `migrate`, `init`, `workflows`, `tasks`, `teams`, `rooms`, `tools`, `providers`, `memory`, `usage`, `vault`, `cron`, `status`, `channels`, `update`, `backup`, `debug`, `setup`, `research`, `scan`, `read`, `logs`, `gateway`, `mcp`, `start`.

### TUI (cmd/tui/)

- Bubble Tea framework
- Sidebar + chat viewport + code view
- Markdown rendering with syntax highlighting
- Streaming tool-call rendering (`stream.go`)
- Slash-command popup
- Model + agent picker
- Code-view with language-aware highlighting (`code_view.go`)

---

## developer_experience

- **`qorven doctor`** — comprehensive health check (version, config, DB, providers, agents, tools, channels, workspace, external deps, gateway)
- **`qorven migrate`** — DB schema migrations CLI
- **`qorven setup`** — guided first-run config
- **`make dev`** — run without building
- **`make dev-watch`** — hot-reload + auto-restart via `air` (see `.air.toml`)
- **`scripts/dev-up.sh`** — port-check preflight with `--kill` / `--check-only` flags
- **Migration idempotency** — `IF (NOT) EXISTS` guards throughout `backend/migrations/`

---

## testing_infrastructure

- **217+ `*_test.go` files** across `backend/internal/`
- **Test helpers** — `internal/testutil/`, `internal/testsupport/`
- **Fixtures** — `internal/plugins/wasm/testdata/`
- **Integration tests** — `integration_test.go` / `hard_test.go` files in permissions, channels, workflow, scenarios
- **Resilience unit tests** — `internal/gateway/resilience_test.go`, `internal/providers/toolcall_parse_test.go`, `internal/config/atomic_test.go`
- **Deep E2E** — `internal/gateway/deep_e2e_test.go`, `diamond_*_test.go`, `hard_stress_test.go`

---

## plugins

WASM plugins under `internal/plugins/wasm/`.

- **Runtime** — wazero (pure Go, no CGO)
- **Sandbox** — 64 WASM pages (4 MiB) memory cap, 2s CPU timeout, no network, no filesystem
- **Contract** — JSON stdin → plugin → JSON stdout; isolated per invocation
- **Hooks** — plugins register custom tools that appear in the agent's tool list
- **Test fixtures** — example WASM plugins in `testdata/`

---

## migrations

Paired `*.up.sql` / `*.down.sql` files in `backend/migrations/`, numbered 001+. `IF NOT EXISTS` on CREATE, `IF EXISTS` on DROP where applicable.

Major phases:
- **001** — initial schema (agents, sessions, messages, tools)
- **004** — multi-agent architecture
- **006** — knowledge graph tables
- **021** — RAG embeddings (pgvector)
- **022** — skill marketplace
- **025** — core1 systems (large foundational update)
- **027** — team tasks
- **032** — agent dreaming
- **037** — plan graph for step-by-step reasoning (largest)
- **038** — multi-tenant foundations
- **040** — RLS tenant isolation
- **043** — plugins table
- **048** — auth hardening
- **049+** — voice providers

---

## resilience

See `internal/gateway/resilience.go` + `docs/ops/resilience.md`.

| Feature | Where | Purpose |
|---|---|---|
| Port probe | `resilience.go:bindListener` | Walk configured port → +10 before failing |
| Runtime JSON | `~/.qorven/runtime.json` | Web client discovers actual bound port |
| `/livez` | `resilience.go:handleLivez` | Cheap liveness — no DB |
| `/readyz` | `resilience.go:handleReadyz` | Readiness — DB + voice checked |
| `/__qorven_runtime` | `resilience.go:handleRuntimeInfo` | Port discovery endpoint |
| Graceful shutdown | `gateway.go:installShutdownHandler` | 10s drain on SIGTERM, 2nd signal = hard exit |
| WS heartbeat | `resilience.go:runWSHeartbeat` | 20s ping / 10s timeout / shared-context teardown |
| Background process cleanup | `tools/exec.go:ShutdownBackgroundProcesses` | SIGTERM detached children on shutdown |
| Atomic config writes | `config/atomic.go:AtomicWriteFile` | Temp + fsync + rename, no partial files |
| Tool-call JSON validation | `providers/toolcall_parse.go` | Truncated args rejected, not executed empty |
| Empty-response recovery | `agent/loop.go` | Never return blank reply to user |
| Tenant-scoped sessions | `agent/background_session.go:CreateForTenant` | TenantID in key + workspace path |
| Frontend reconnect backoff | `web/lib/websocket.ts` | Exponential backoff + jitter, cap 30s |
| REST retry on ECONNREFUSED | `web/lib/api.ts:fetchWithRetry` | GET/HEAD retry 3× with backoff |
| Reconnect banner | `web/components/reconnect-banner.tsx` | Amber banner >5s disconnect, green flash on recover |

---

## frontend_surfaces

Next.js app under `web/`.

| Surface | Path | Purpose |
|---|---|---|
| Dashboard | `web/app/(app)/page.tsx` | Landing + activity feed |
| Chat | `web/app/(app)/chat/*` | Agent chat with streaming |
| Agents (Qors) | `web/app/(app)/qors/*` | Per-agent detail, chat, config |
| Rooms | `web/app/(app)/rooms/*` | Multi-agent rooms |
| Code IDE | `web/app/(app)/code/page.tsx` | Monaco + chat |
| Mail | `web/app/(app)/mail/page.tsx` | Email inbox |
| Tasks | `web/app/(app)/tasks/page.tsx` | Task queue |
| Teams | `web/app/(app)/teams/page.tsx` | Team management |
| Calendar / Schedule | `web/app/(app)/schedule/page.tsx` | Events |
| Drive | `web/app/(app)/drive/*` | Files |
| Models Hub | `web/app/(app)/models-hub/page.tsx` | Provider + model config |
| Voice tester | `web/app/(app)/voice/page.tsx` | TTS + STT playground |
| Settings | `web/app/(app)/settings/page.tsx` | Services, voice, notifications |
| Setup wizard | `web/app/setup/page.tsx` | 11-step first-run config |
| Terminal | `web/app/(app)/terminal/page.tsx` | tmux-backed terminal |
| Approvals | `web/app/(app)/approvals/page.tsx` | Outbound-action queue |
| Heartbeat | `web/app/(app)/heartbeat/page.tsx` | Per-agent liveness + WS status |

Shared chrome: left rail + sidebar + header + right panel + bottom drawer + status bar (`web/components/layouts/qorven/*`). Prime voice widget floats at sidebar bottom when Services → Voice is enabled.

---

## not_implemented

Things a user might expect but are **not** currently in the code. Say no when asked — don't invent a workaround unless the user asks for one.

- **Cloudflare JS-challenge solver** (anti-bot stops at TLS/header/stealth layers)
- **Captcha solving** (hCaptcha / reCAPTCHA)
- **Weather API tool**
- **Hotel / car / train search** (flights only)
- **Dynamic model hot-swap** (llama-swap-style GPU-resident rotation)
- **Screen share / remote desktop** (voice-only WebRTC today)
- **Voice cloning tool**
- **Voice activity detection**
- **Explicit prompt-injection defense** (credential scrubbing exists; no dedicated PI classifier)
- **PII filter** (credentials are redacted; no structured PII detection)

---

## discovery

To answer "does Qorven do X?", grep in order:

1. **Tool** — `rg -l 'Register\(.*name.*"X"' backend/internal/tools/`
2. **Provider** — `ls backend/internal/providers/*.go`
3. **Channel** — `ls backend/internal/channels/`
4. **Voice** — `ls backend/internal/voice/`
5. **Scraper / browser** — `ls backend/internal/scraper/ backend/internal/qor/browser/`
6. **Social** — `ls backend/internal/qor/social/`
7. **Connector** — `ls backend/internal/connectors/`
8. **Route** — `rg 'r\.(Get|Post|Put|Delete)\("/X' backend/internal/gateway/`
9. **Subsystem** — `ls backend/internal/`
10. **Migration** — `ls backend/migrations/`

If it's not in those places, it's not in the product.
