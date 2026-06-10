# Sidebar System

Three regions, left → right: the icon **Rail** (section switcher) → the **contextual Sidebar** (the active section's own content) → page content. The sidebar also has an always-present **pinned zone** (Hubs + Recent chats) and the **COO dock** at the very bottom.

## Rail — the section switcher

`web/components/layouts/qorven/rail.tsx`. 60px icon column (`--rail-width`), `hidden lg:flex` (hidden on mobile). Two arrays:

- `primary[]` — 8 core sections: Dashboard (`/`), Chat (`/qors`), Hubs (`/hubs`), Code (`/code`), Email (`/mail`), Channels (`/channels`), Social (`/social`), Drive (`/drive`).
- `bottom[]` — **More** (`/goals`, opens the grouped menu), Models (`/models-hub`), Settings (`/settings`).

Active state comes from `useActiveRail()`.

## useActiveRail — route → section

`web/hooks/use-active-rail.ts`. Maps ~50 routes to a `RailSection` id (most-specific first). `/` → `dashboard`; **unmapped routes → `'home'`** (the "More" icon), so the grouped menu lights up for Organization/Build/Observe/System pages. This same id drives both the rail highlight and the contextual sidebar switch.

## Sidebar — contextual middle + pinned zone

`web/components/layouts/qorven/sidebar.tsx`. Structure:

```
<div class="sidebar" left:var(--rail-width)>
  <SidebarHeader/>                  ← workspace/account dropdown
  <div flex-1 overflow-y-auto>      ← contextual middle (switch on activeRail)
    {contextual}
  </div>
  <SidebarPinned/>                  ← pinned Hubs + Recent chats
</div>
<!-- AgentVoicePill (COO dock) is rendered separately in layout.tsx, fixed bottom -->
```

The contextual switch:

| `activeRail` | Component |
|--------------|-----------|
| `code` | `CodeSidebar` (`layouts/qorven/sidebar-code.tsx`) |
| `sessions` | `MailSidebar` |
| `connectors` | `ChannelsSidebar` |
| `social` | `SocialSidebar` |
| `drive` | `DriveSidebar` |
| `org-chart` / `teams` | `TeamsSidebar` |
| `mcp` | `McpSidebar` |
| `kg` | `KnowledgeSidebar` |
| `models` | `ModelsSidebar` |
| `settings` | `SettingsSidebar` |
| `apps` | `AppsSidebar` |
| default (dashboard, hubs, souls, **more**, unknown) | `SidebarNav` (the grouped accordion of all pages) |

## SidebarLayout — the slot contract

`web/components/sidebar/sidebar-layout.tsx`. Every per-section sidebar uses it:

```tsx
<SidebarLayout
  section2={<FilterRow/>}   // optional 44px sticky search/filter/picker row
  section3={<NavList/>}     // scrollable nav content
/>
```

## Sidebar primitives

`web/components/sidebar/sidebar-primitives.tsx` — use these for consistent section styling:

- **`SidebarGroupTitle`** — uppercase muted section header (`px-3 pt-4 pb-1 · text-2xs font-medium uppercase tracking-wider text-muted-foreground/60`). Every contextual sidebar groups its items under these.
- **`SidebarMenuItem`** — `{ icon, label, badge?, badgeColor?, active?, onClick? }`. Dense row: `h-8.5 gap-2.5 px-2.5 rounded-md`, active = `bg-accent` + medium weight.
- **`SidebarDivider`** / **`SidebarSeparator`** — token-based hairlines.

Search inputs inside sidebars use the `qr-input` class (see [components.md](components.md)).

## Pinned zone — Hubs + Recent chats

`web/components/layouts/qorven/sidebar-pinned.tsx`. Always rendered, pinned above the COO dock. Two collapsible groups with `SidebarGroupTitle`-style headers and a divider between them:

- **Hubs** — all hubs (`+` opens `/hubs`); a `#`-glyph row per hub with a right-aligned member count; clicking routes to `/hubs/[id]`.
- **Recent chats** — recent agents/souls; a `soulGradient` avatar + name; clicking routes to `/qors/[id]`.

**Pinning:** hover a row → a star pins it (backend-synced, per-user, via `pins` API — `sidebar_pins` table). Pinned items **float to the top** of their own list (Hubs / Recent chats) with a filled star — there's no separate "Pinned" group. Each list scrolls within a capped height (`max-h: clamp(108px,18vh,280px)`), so the zone never pushes the contextual content off-screen or under the dock (the sidebar column reserves `--agent-pill-height` on `lg+`).

## COO dock

`web/components/layouts/qorven/agent-voice-pill.tsx` — fixed at the bottom of the sidebar column (its own `fixed`, `left:var(--rail-width)`, `width:var(--sidebar-default-width)`, `height:var(--agent-pill-height)`). The always-present COO voice/chat affordance.

## Adding a section to a sidebar

Use `SidebarLayout` + `SidebarGroupTitle` + `SidebarMenuItem`. Drive/Mail/Social/Tasks/Models/Channels/Teams are the reference implementations. To add a whole new rail section: add to `rail.tsx` `primary`/`bottom`, map its routes in `use-active-rail.ts`, and add a `case` in `sidebar.tsx` pointing at a `*-sidebar.tsx` component (or let it fall to `SidebarNav`).
