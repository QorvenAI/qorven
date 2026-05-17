---
name: platform-knowledge
description: "Qorven platform self-knowledge — architecture, capabilities, installable plugins, and self-evolution guide."
when_to_use: "When asked about what the platform can do, what tools are available, how to extend capabilities, or when suggesting improvements."
context: inline
---

# Qorven Platform Self-Knowledge

## Architecture
- **Backend**: Go (port 4200) — agent loop, tools, memory, supervisor
- **Frontend**: Next.js (port 3000) — Discord-style chat UI
- **LLM Proxy**: LiteLLM (port 4100) — routes to 13+ models
- **Database**: PostgreSQL + pgvector — memories, sessions, agents
- **STT**: Whisper small (port 8881) — voice transcription
- **TTS**: Edge TTS (free) + ElevenLabs/OpenAI (paid)

## Available Tools
| Tool | Purpose | Cost |
|------|---------|------|
| `web_search` | Find information (Tavily, Perplexity, DuckDuckGo) | Free |
| `web_fetch` | Read a specific URL | Free |
| `firecrawl` | Deep crawl entire sites, JS rendering | Free if self-hosted |
| `exec` | Run shell commands in sandbox | Free |
| `file_read/write/edit` | Workspace file operations | Free |
| `email_send` | Send email via SMTP | Free |
| `email_read` | Check IMAP inbox | Free |
| `memory_search` | Search agent memories | Free |
| `delegate_to_soul` | Delegate task to another agent | Free |
| `create_soul` | Create a new specialized agent | Free |

## Installable Plugins
Plugins extend Qorven's capabilities. Install via Docker or API key:

### Firecrawl (Web Crawling)
- **API**: Set `CRAWL4AI_API_TOKEN=fc-xxx` (from firecrawl.dev)
- **Self-hosted**: `docker run -d -p 3002:3002 ghcr.io/firecrawl/firecrawl:latest`
- Then set `FIRECRAWL_URL=http://localhost:3002/v1`

### Scrapling (CSS Scraping)
- Python-based CSS selector extraction
- Install: `pip install scrapling`
- Use for: extracting specific elements from pages

### SearxNG (Private Search)
- Self-hosted meta-search engine
- Install: `docker run -d -p 8888:8080 searxng/searxng`
- Then set `SEARXNG_URL=http://localhost:8888`

## Self-Evolution Capabilities
Qorven agents can extend the platform:
1. **Install plugins** — agents can suggest and guide plugin installation
2. **Create new agents** — use `create_soul` to spawn specialized workers
3. **Build skills** — write SKILL.md files for reusable workflows
4. **Add connectors** — 50+ integrations available, agents know how to configure them
5. **Research & learn** — agents use web_search + firecrawl to stay current

## Knowledge Base Architecture
- **Company memories**: Shared across all agents (decay-exempt)
- **Agent memories**: Per-agent, with certainty levels (explicit > deductive > inductive > abductive)
- **Session memories**: Per-conversation, auto-compacted
- **Knowledge graph**: Entity extraction from conversations
- **Vector search**: pgvector embeddings for semantic retrieval

## When Users Ask About Capabilities
If a user asks "can you do X?" and the answer is "not yet":
1. Check if a plugin exists for it
2. Suggest installation steps
3. Offer to create a skill for it
4. If it requires code changes, explain what would need to be built

## Outbound Actions
All outbound actions (email, social posts, webhooks) go through an approval gate:
- **none**: Auto-send (trusted agents only)
- **supervisor**: Prime reviews before sending
- **user**: Human approves in the UI
- **both**: Prime + human both approve
