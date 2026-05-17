---
name: firecrawl
description: "Self-hosted Firecrawl — deep web crawling, scraping, and search for AI agents. Converts websites into clean markdown."
when_to_use: "When you need to crawl entire websites, extract documentation, scrape JS-rendered pages, or get structured data from complex sites. NOT for single URL reads (use web_fetch) or searching (use web_search)."
allowed_tools: ["exec", "file_write"]
context: inline
---

# Firecrawl Plugin — Self-Hosted Web Crawler

## What This Plugin Does
Firecrawl converts websites into clean, LLM-ready markdown. It handles JavaScript rendering, follows links, and extracts structured content.

## Installation (Self-Hosted)

### Option 1: Docker (Recommended)
```bash
docker run -d --name firecrawl \
  -p 3002:3002 \
  -e FIRECRAWL_PORT=3002 \
  ghcr.io/firecrawl/firecrawl:latest
```

### Option 2: Docker Compose (add to existing stack)
Add this to your `docker-compose.yml`:
```yaml
firecrawl:
  image: ghcr.io/firecrawl/firecrawl:latest
  ports: ["3002:3002"]
  environment:
    - FIRECRAWL_PORT=3002
  restart: unless-stopped
```

### After Installation
Set the environment variable so Qorven uses your self-hosted instance:
```bash
export FIRECRAWL_URL=http://localhost:3002/v1
```

## When to Use Which Tool

| Need | Tool | Cost |
|------|------|------|
| Search the web | `web_search` | Free (Tavily/DDG) |
| Read one URL | `web_fetch` | Free (built-in) |
| Deep crawl a site | `firecrawl` (this plugin) | Free if self-hosted |
| Custom CSS extraction | `scrapling` plugin | Free |
| JS-rendered pages | `firecrawl` scrape mode | Free if self-hosted |

## API Usage (if using hosted API)
Get an API key from https://firecrawl.dev — set `CRAWL4AI_API_TOKEN=fc-xxx`

## Capabilities
- **Scrape**: Single page → clean markdown (handles JS)
- **Crawl**: Follow links → multiple pages as markdown
- **Search**: Search + scrape in one call
- **Extract**: Structured data extraction with schemas
