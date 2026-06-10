# How to build a page

Design-system reference for every canvas page in Qorven. Follow these rules so C3's page sweep passes without rework.

---

## 1. Use `PageShell` for every non-full-bleed page

```tsx
import { PageShell } from '@/components/layouts/page-shell';
```

**Props**

| Prop | Type | Required | Notes |
|---|---|---|---|
| `title` | string | yes | Page heading text |
| `description` | string | — | Subtitle below the heading |
| `actions` | ReactNode | — | Right-aligned buttons in the header |
| `toolbar` | ReactNode | — | Row below the header — tabs, filters, search |
| `children` | ReactNode | yes | Page body |
| `contentClassName` | string | — | Overrides default content padding |

**Real example (tasks page)**

```tsx
<PageShell
  title="Tasks"
  description="Track work across all your agents"
  actions={<button className="… bg-primary …">New task</button>}
  toolbar={<><Filter /><select …/><select …/></>}
>
  {/* page body */}
</PageShell>
```

**Default content area** scrolls (`flex-1 overflow-y-auto`) with padding `px-4 py-4 sm:px-6`.

If your page body manages its own edge padding (flush list, sticky footer, etc.) pass `contentClassName="px-0 py-0 sm:px-0"` to avoid double-padding — the tasks and audit pages do this.

---

## 2. Full-bleed exceptions

These pages intentionally fill 100% height and do **not** use `PageShell`. They have their own internal headers or panes:

- `/qors` — chat, conversation filling the full space
- `/code` — IDE/autonomous dev view
- `/mail` — two-pane layout with its own pane headers
- `/knowledge-graph` — canvas fill with inline stats row

---

## 3. Type scale — no arbitrary `px`

Use named sizes only. **Never `text-[Npx]`.**

| Class | px | When to use |
|---|---|---|
| `text-2xs` | 11 | Dense badges / status only |
| `text-xs` | 12 | Labels, meta — floor for normal UI text |
| `text-2sm` | 13 | Slightly denser secondary text |
| `text-sm` | 14 | Secondary text, table cells |
| `text-base` | 16 | Body copy |
| `text-lg` | 18 | Page header title |
| `text-xl` / `text-2xl` / `text-3xl` | 20+ | Section headings, hero text |

12 px is the visual floor for normal UI. 11 px (`text-2xs`) is reserved for dense badges and status indicators only.

---

## 4. Color tokens — no raw hex

Use semantic utilities. **Never a raw hex (`#…`) in `className` or `style`.**

- Backgrounds: `bg-background`, `bg-card`, `bg-primary`, `bg-muted`, `bg-accent`
- Text: `text-foreground`, `text-muted-foreground`, `text-primary-foreground`
- Borders / rings: `border-border`, `ring-ring`
- Radius: `rounded-sm`, `rounded-md`, `rounded-lg`, `rounded-xl` (all token-derived)

If you need a genuine brand/channel color, define it as a `--channel-*` or `--connector-*` CSS variable in `web/css/config.qorven.css` and reference it — don't inline a hex. All tokens live in that file (light + dark themes, type tiers, `--font-sans` = Inter).

---

## 5. Enforcement

A pre-commit guard (`web/scripts/check-design-tokens.sh`, wired into the repo's pre-commit hook) **fails any commit** that introduces `text-[Npx]` or a raw hex value in staged `web/**/*.tsx|ts` files (outside `web/css/`). Fix violations before committing.

---

## 6. Mobile rules

`PageShell` is responsive out of the box: the header stacks on small screens and content padding tightens (`px-4` mobile → `sm:px-6`). For page content:

- Avoid fixed-width in-canvas sidebars (`w-N shrink-0`) that don't collapse — below `lg` they should become a drawer/sheet or stack vertically.
- Use responsive prefixes (`sm:` / `md:` / `lg:`) for multi-column layouts.

---

## 7. Primitives

Reusable components live in `web/components/ui/*` — Button, Card, Input, Select, Badge, Tabs, and `accordion-menu` for grouped nav. They already consume the design tokens. **Prefer them over hand-rolling** buttons, cards, or inputs from scratch.
