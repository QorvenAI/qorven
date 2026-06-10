# Builder Knowledge Base

How agents build apps/plugins/themes **on top of** Qorven (the WordPress-plugin model). Qorven is the platform; apps extend it.

## How agents get this knowledge

1. **Always-on summary** — a short "Building Apps on Qorven" section is injected into every agent's system prompt (`backend/internal/agent/platform_knowledge.go` → `builderkb.Summary()`). It states the app platform exists, the build flow, the hard rules, and how to get more.
2. **On-demand retrieval** — the `get_builder_knowledge(topic)` tool returns the deep doc for a topic. Topics: `overview`, `app-manifest`, `scaffold`, `ui`, `db`, `external`.

## Canonical source (embedded)

The knowledge docs are **embedded into the Go binary** so they ship and version with it and work in production. They live at:

```
backend/internal/builderkb/docs/
  overview.md      what an app is; internal vs external; the build flow
  app-manifest.md  the full app.yaml schema
  scaffold.md      directory layout + plain-JS IIFE UI bundle + tools + install
  ui.md            the design system app UIs must follow (condensed; see docs/ui/)
  db.md            migrations, tenant/user scoping, data access
  external.md      external-facing apps + tunnel exposure (in progress)
```

`backend/internal/builderkb/kb.go` embeds these (`//go:embed docs/*.md`) and exposes `Topics()`, `Get(topic)`, and `Summary()`.

> Edit the embedded `.md` files to change what agents know — they're the single source of truth. This README is just the pointer.

## Related

- **`docs/ui/`** — the full human UI design-system reference (tokens, theme presets, typography, layout, sidebar, components). The builder `ui` topic is a condensed pointer to it.
- The real app-build protocol is also mirrored in the Prime Coder prompt (`backend/internal/agent/prime_coder.go`) — keep the two in sync when the app format changes.
