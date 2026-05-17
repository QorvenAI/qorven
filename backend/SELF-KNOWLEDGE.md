# Qorven Self-Knowledge

This document describes Qorven's own architecture.
The agent MUST read this before making any changes to itself.

## Architecture Overview

- Language: Go 1.26
- Module: github.com/qorvenai/qorven
- Database: PostgreSQL 15 + pgvector
- Gateway: HTTP server on port 4200
- Web UI: Next.js on port 3000
- Config: config.toml + .env

## Key Packages

| Package | Purpose | Key Files |
|---------|---------|----------|
| agent | Agent loop, tools, memory, learning | activity_tracker.go, agent.go, auto_compact.go, background.go |
| autonomy | Cron scheduler, heartbeat runner | cron.go, heartbeat.go |
| billing | Usage tracking, cost calculation | billing.go |
| channels | Email, Telegram, Slack, Discord, WhatsApp | approval.go, binding.go, channel.go, debounce.go |
| config | Config loading, hot reload | agent_config.go, config.go, hot.go, hotreload.go |
| drive | File storage, permissions | store.go |
| engine | Brain engine, cron, heartbeat, QOROS | engine.go |
| gateway | HTTP server, API handlers, channel consumer | builtin_tools.go, callbacks.go, channel_events.go, consumer.go |
| mail | Mail store, identities, routing | agentmail.go, imap_poller.go, router.go, smtp_sender.go |
| memory | Memory store, vector search, knowledge graph | backend.go, backend_pg.go, certainty.go, chunker.go |
| providers | LLM providers (OpenAI, Anthropic, Bedrock, Gemini) | anthropic.go, batch.go, bedrock.go, capabilities.go |
| realtime | WebSocket hub, live updates | hub.go |
| sandbox | Docker sandbox for code execution | docker.go, docker_resolve.go, fsbridge.go, hints.go |
| session | Session store, history, compression | search.go, session.go |
| skills | Skill loader, crystallizer, marketplace | crystallizer.go, learner.go, loader.go, marketplace.go |
| souldesk | Multi-agent delegation, handoff | announce_queue.go, desk.go, router.go, tasks.go |
| tools | All 57 tools (exec, write_file, web_search, etc) | cleanup.go, crawl.go, custom.go, delegate.go |
| voice | TTS, STT, voice pipeline | deepgram.go, elevenlabs.go, format.go, livekit.go |

## Critical Files (DO NOT break these)

- `internal/agent/loop.go` — The main agent loop. Every message goes through here.
- `internal/gateway/gateway.go` — HTTP server, routing, tool registration.
- `internal/gateway/handlers.go` — All API handlers.
- `internal/providers/openai.go` — OpenAI-compatible LLM adapter.
- `internal/providers/bedrock.go` — AWS Bedrock adapter.
- `internal/agent/systemprompt.go` — System prompt builder.
- `internal/config/config.go` — Config struct and loader.
- `cmd/root.go` — CLI entry point.
- `cmd/chat.go` — Chat command.

## How Messages Flow

```
User message → CLI/Telegram/Email/Web
  → gateway.handleAgentChat()
  → agent.Loop.Run()
    → buildSystemPrompt()
    → provider.ChatStream() (LLM call)
    → if tool_calls: execute tools
    → if no tool_calls: return response
    → LearningLoop.RunAfterTask() (background)
  → sendToChannelByType() (reply)
```

## How Tools Are Registered

All tools registered in `gateway.registerTools()`.
Each tool implements: Name(), Description(), Parameters(), Execute().
Tools are in `internal/tools/` or `internal/agent/` (for agent-specific tools).

## How to Safely Make Changes

1. Use `self_knowledge query=build` to verify current build is clean
2. Use `self_knowledge query=file target=path` to read the file you want to change
3. Make the change using `project` tool or `write_file`
4. Use `self_knowledge query=build` to verify the change compiles
5. Use `self_knowledge query=test` to verify tests pass
6. Present the diff for human review
7. NEVER deploy without human approval

## Known Issues

- Bedrock billing: INVALID_PAYMENT_INSTRUMENT on Claude Sonnet 4
- OpenAI key: expired, needs refresh
- Tool narration: some LLMs output tool calls as text (tool_rescue.go handles this)
- Email threading: works but some clients don't show thread correctly
