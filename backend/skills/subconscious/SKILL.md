---
name: subconscious
description: "Self-improvement loop — agents analyze their own performance, debate improvements, and write learnings back into memory. Runs on cron schedule."
when_to_use: "Automatically triggered by cron. Analyzes recent agent runs, generates improvement ideas, debates them via council, writes winning direction back into agent memory."
allowed_tools: ["memory_search", "web_search", "file_write", "file_read"]
context: inline
---

# Subconscious Agent — Self-Improvement Loop

## What This Does
The subconscious runs periodically (default: every 6 hours) and:
1. **Inspects** — reads recent agent runs, failures, and user feedback
2. **Ideates** — generates 3-5 improvement candidates
3. **Debates** — challenges each idea against hard objections (uses council)
4. **Synthesizes** — picks the winning direction
5. **Persists** — writes the learning back into agent memory
6. **Reports** — sends summary to supervisor (Prime)

## The Loop

```
Recent Runs → Evidence Gathering → Ideation (3-5 ideas)
    → Debate/Critique (council) → Synthesis (1 winner)
    → Write to Memory → Next run starts smarter
```

## Evidence Sources
- Recent session messages (last 24h)
- Tool call success/failure rates
- User feedback (thumbs up/down)
- Escalation history
- Memory staleness

## What Gets Written Back
- **Winning direction** → saved as agent memory (explicit certainty)
- **Rejected paths** → saved as agent memory (abductive certainty, low weight)
- **Improvement backlog** → saved as prime memory
- **Run summary** → saved to audit log

## Configuration
- `schedule`: cron expression (default: `0 */6 * * *` — every 6 hours)
- `max_ideas`: number of candidates to generate (default: 5)
- `debate_rounds`: number of challenge/defense rounds (default: 2)
- `auto_apply`: whether to apply improvements without approval (default: false)
- `model_ideation`: model for generating ideas (default: cheap/fast)
- `model_debate`: model for critique (default: premium)
