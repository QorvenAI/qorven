---
name: autoresearch
description: "Automated research pipeline — find, index, and surface insights from papers, docs, and web sources."
when_to_use: "When asked to research a topic deeply, find papers, build a knowledge base, or create a research report with citations."
allowed_tools: ["web_search", "web_fetch", "firecrawl", "memory_search", "file_write", "file_read"]
context: inline
---

# AutoResearch Pipeline

## How to Research Deeply

### Step 1: Decompose the Query
Break the research question into 3-5 sub-queries:
- Core concept definition
- Recent developments (last 6 months)
- Key papers/implementations
- Practical applications
- Open problems

### Step 2: Multi-Source Search
For each sub-query:
1. `web_search` — find relevant pages and papers
2. `web_fetch` — read the most promising results
3. `firecrawl` — if a source has multiple relevant pages, crawl the site
4. Save key findings to memory with `memory_search` context

### Step 3: Synthesize
- Cross-reference findings across sources
- Identify consensus vs. disagreement
- Note recency — prefer 2025-2026 sources
- Cite everything: [1] source.com - Title

### Step 4: Index for Future Use
Save the research summary to agent memory so it can be retrieved later:
- Key facts as explicit memories
- Patterns as inductive memories
- Hypotheses as abductive memories

## Research Quality Checklist
- [ ] Multiple sources consulted (minimum 3)
- [ ] Recent sources prioritized
- [ ] Conflicting viewpoints noted
- [ ] All claims cited
- [ ] Summary saved to memory for future reference

## Paper Discovery
When looking for research papers:
1. Search arxiv.org, semanticscholar.org, paperswithcode.com
2. Use firecrawl to extract paper abstracts and key findings
3. Index papers with: title, authors, date, key contribution, relevance score
