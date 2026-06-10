# Layout

The app shell is assembled in **`web/components/layouts/qorven/layout.tsx`** and styled by **`web/css/layout.qorven.css`** (everything scoped under the `.qorven` root class). Individual pages use **`PageShell`** + **`CanvasHeader`**.

## The shell regions (left → right, top → bottom)

```
┌──────┬───────────────┬──────────────────────────────┐
│      │               │  Header (fixed)               │  ← breadcrumb · global search · panel icons
│ Rail │  Sidebar      ├──────────────────────────────┤
│ 60px │  280px        │  Toolbar (optional, 44px)     │
│      │  (contextual  ├──────────────────────────────┤
│      │   + pinned    │                              │
│      │   zone +      │  main-canvas (scrolls)       │
│      │   COO dock)   │   page content (PageShell)   │
│      │               │                              │
└──────┴───────────────┴──────────────────────────────┘
                          Status bar (24px, fixed bottom)
```

Mounted in `layout.tsx`: `<Rail/>`, `<Sidebar/>`, mobile scrim, `.wrapper` (Header + Toolbar + `<main class="main-canvas">`), `<ContextPanel/>`, `<RightPanel/>`, `<BottomDrawer/>`, `<AgentVoicePill/>` (the COO dock), `<StatusBar/>`, and the mobile bottom bar.

## Layout CSS variables (`.qorven`)

| Var | Value | Meaning |
|-----|-------|---------|
| `--rail-width` | 60px (0 on mobile) | Icon rail width |
| `--sidebar-width` | 280px (0 collapsed/mobile) | Active sidebar width |
| `--sidebar-default-width` | 280px | Full width used by the mobile drawer |
| `--header-height` | 44px | Header height |
| `--context-panel-width` | 320px | Right context panel |
| `--nav-width` | rail + sidebar | Combined left offset for wrapper/header |
| `--status-bar-height` | 24px (set on root in layout.tsx) | Bottom status bar |
| `--agent-pill-height` | 56px (set in layout.tsx) | COO dock height (reserved in the sidebar column) |
| `--toolbar-height` | 0 / 44px (`.has-toolbar`) | Optional toolbar row |

State classes toggled on `.qorven`: `sidebar-collapse`, `context-panel-open`, `right-panel-open`, `has-toolbar`, `bottom-drawer-open`, `layout-initialized` (enables transitions after first paint), `no-transition` (one-frame transition kill, used around `/code`).

## PageShell — every non-full-bleed page

`web/components/layouts/page-shell.tsx`:

```tsx
<PageShell title="Tasks" description="…" actions={<Btn/>} toolbar={<Filters/>}>
  <YourContent/>
</PageShell>
```

| Prop | Notes |
|------|-------|
| `title` (req) | rendered by `CanvasHeader` |
| `description` | subtitle |
| `actions` | right-aligned header buttons |
| `toolbar` | row below header (`border-b`, `px-4 py-2.5 sm:px-6`) for tabs/filters/search |
| `children` (req) | body |
| `contentClassName` | override the default content padding (`flex-1 overflow-y-auto px-4 py-4 sm:px-6`) — pass `px-0 py-0 sm:px-0` for flush lists |

## CanvasHeader

`web/components/layouts/canvas-header.tsx` — `title` (`text-xl font-semibold`), optional `description` (`text-sm text-muted-foreground`), optional `actions` (right). **Every titled page uses this** (directly or via PageShell). Never inline an `<h1>`/custom flex header.

## Full-bleed pages

Pages that fill 100% height skip `PageShell` and wrap content in `.full-bleed` (which cancels the canvas padding/centering): `/qors` (chat), `/code` (IDE), `/mail` (two-pane), `/knowledge-graph`, `/hubs/[id]` (hub detail). The canvas centers normal pages at `max-width: 90rem`.

## Header (top bar)

`web/components/layouts/qorven/header.tsx`, fixed, spans from the rail to the right edge:
- **Left:** mobile hamburger (`< lg`) / sidebar-collapse toggle (`lg+`) + breadcrumb (or the soul/code context).
- **Center:** the **global search** trigger — a "Search agents, hubs, anything… ⌘K" pill (icon-only `< md`) that opens the command palette (see [components.md](components.md#command-palette)).
- **Right:** pinned app icons, chat/terminal/notifications/activity panel toggles, connection status, right-panel toggle.

## Panels

- **Context panel** (`--context-panel-width`, 320px) and **Right panel** (320px, chat/notifications/activity) push the wrapper on `lg+`, overlay on smaller.
- **Bottom drawer** sits above the status bar.
- **Status bar** (`StatusBar`, 24px) is pinned bottom; the canvas always reserves its height.

## Mobile

- `< lg`: rail width → 0; the sidebar becomes an **off-canvas drawer** (`translateX(-100%)`, slides in when `[data-mobile-nav='open']`) with a scrim behind it (z-index: drawer 50, scrim 40). Opened by the header hamburger.
- `≤ 640px`: rail hidden; a **bottom navigation bar** (`.mobile-bottom-bar`, 56px) shows. The wrapper goes full-width with bottom padding for the bar.
