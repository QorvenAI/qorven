# Qorven UI System

The complete reference for Qorven's design system — tokens, theming, typography, layout, sidebar, and components. Everything here is verified against the code as of 2026-06-10.

> **Building a page?** Start with [`web/components/layouts/README.md`](../../web/components/layouts/README.md) ("How to build a page") — it's the quick rulebook. This folder is the deeper reference behind it.

## Contents

| Doc | What's in it |
|-----|-------------|
| [tokens.md](tokens.md) | Color/radius/status CSS variables, `@theme inline` mapping, dark mode, channel/connector brand colors. How to use a token; never raw hex. |
| [theme-presets.md](theme-presets.md) | The 6 full-palette presets (violet/slate/ocean/sand/midnight/graphite), the neutral-surface + vivid-accent model, the brand-color override, and how to add a preset. |
| [typography.md](typography.md) | Type scale (`text-2xs`…`text-3xl`), the 12px floor, the Inter/JetBrains font wiring, icon stroke, and the design-token guard. |
| [layout.md](layout.md) | The `.qorven` layout shell, layout CSS variables, `PageShell` + `CanvasHeader`, full-bleed exceptions, header (with global search), toolbar, panels, status bar, mobile drawer/bottom-bar. |
| [sidebar.md](sidebar.md) | Rail → contextual sidebar → pinned zone architecture, `useActiveRail`, the section→component switch, `SidebarLayout`, sidebar primitives, the pinned Hubs/Recent-chats zone, hub rows. |
| [components.md](components.md) | The `qr-*` CSS component classes, `web/components/ui/*` primitives, settings primitives, soul avatars, badges, command palette. |
| [cards.md](cards.md) | (pre-existing) Card patterns. |

## The non-negotiable rules

1. **No raw hex** in `className`/`style`. Use semantic tokens (`bg-primary`, `text-muted-foreground`, `border-border`). Genuine brand colors go in `web/css/config.qorven.css` as `--channel-*`/`--connector-*`.
2. **No arbitrary font sizes** (`text-[Npx]`). Use the named type scale. 12px (`text-xs`) is the floor for normal UI text; 11px (`text-2xs`) is for dense badges/status only.
3. **Every titled page uses `CanvasHeader`** (usually via `PageShell`). Never inline an `<h1>` or a custom flex header.
4. **Nav lives in the sidebar**, never as a second `w-N shrink-0 border-r` column inside the canvas.
5. Both rules 1 & 2 are enforced by a **pre-commit guard** (`web/scripts/check-design-tokens.sh`).

## Where the system lives (file map)

| Concern | File(s) |
|---------|---------|
| Color/radius/status tokens, theme presets, type-scale vars, `@theme` | `web/css/config.qorven.css` |
| `qr-*` component classes, scrollbars, global base styles | `web/css/styles.css` |
| Layout shell CSS (`.qorven` vars, rail/sidebar/header/wrapper, mobile) | `web/css/layout.qorven.css` |
| Theme state (presets, brand color, font, radius, density) | `web/lib/theme-provider.tsx` |
| Page skeleton | `web/components/layouts/page-shell.tsx`, `canvas-header.tsx` |
| App shell (rail + sidebar + header + panels) | `web/components/layouts/qorven/layout.tsx` |
| Rail (section switcher) | `web/components/layouts/qorven/rail.tsx` |
| Route→section mapping | `web/hooks/use-active-rail.ts` |
| Contextual sidebar + switch | `web/components/layouts/qorven/sidebar.tsx` |
| Pinned Hubs + Recent chats | `web/components/layouts/qorven/sidebar-pinned.tsx` |
| Sidebar slot contract + primitives | `web/components/sidebar/sidebar-layout.tsx`, `sidebar-primitives.tsx` |
| Per-section sidebars | `web/components/sidebar/*-sidebar.tsx`, `web/components/layouts/qorven/sidebar-code.tsx` |
| Reusable UI primitives | `web/components/ui/*` |
| Settings form primitives | `web/components/settings/sections/primitives.tsx` |
| Fonts + root `<html>`/ThemeProvider wiring | `web/app/layout.tsx` |
| Quick page-building rulebook | `web/components/layouts/README.md` |

## Terminology note

The hub feature uses the word **"Hub"** everywhere in the UI. The web route is **`/hubs`** (legacy `/rooms` redirects to it). The backend API is still `/v1/rooms` and internal identifiers use `room*` — that's intentional; only the user-facing word and the web route are "Hub"/`/hubs`.
