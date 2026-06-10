# Components

Three layers: **`qr-*` CSS classes** (framework-agnostic shapes), **`web/components/ui/*`** (React primitives), and **domain components** (settings forms, soul avatars, command palette).

## `qr-*` utility classes

Defined in `web/css/styles.css`. Use these (or the `ui/*` primitives) instead of hand-rolling buttons/inputs/cards. Sizes follow `lg=h-10 (40px) · md=h-8.5 (34px) · sm=h-7 (28px)`; radii cascade from `--radius`.

| Class | What |
|-------|------|
| `qr-btn` + `qr-btn-primary`/`-outline`/`-ghost`/`-destructive` | Button base + variants. Sizes: `qr-btn-lg`/`-sm`/`-xs`, `qr-btn-icon`. md height = 2.125rem, `text-2sm`, weight 500. |
| `qr-input` | Text input — `h-8.5`, `rounded-md`, `border-input`, `shadow-xs`; focus → ring. Used by sidebar searches. |
| `qr-select` | Native select, same shape (keeps native arrow; option bg = card for dark readability). |
| `qr-textarea` | Multiline, vertical resize. |
| `qr-card` + `qr-card-header`/`-title`/`-description`/`-content` | Card with `radius-xl`, `border-border`, `bg-card`. Title is `text-base font-semibold`. |
| `qr-badge` + `-primary`/`-muted`/`-success`/`-warning`/`-destructive` | Pill badges, `text-2xs`. |

Scrollbars: `.scrollbar-none` (hidden), `.scrollbar-thin` (Tailwind), and the `.main-canvas` thin-on-hover treatment — all in `styles.css`.

## `web/components/ui/*` primitives

Token-consuming React components. Prefer these over raw markup. Available (partial): `button`, `card`, `input`, `select`, `textarea`, `badge`, `tabs`, `dialog`, `drawer`, `sheet`, `popover`, `dropdown-menu`, `tooltip`, `command`, `accordion` / `accordion-menu` (grouped nav), `avatar` / `avatar-group`, `checkbox`, `radio-group`, `switch`, `progress`, `pagination`, `breadcrumb`, `kbd`, `scroll-area`, `resizable`, `data-grid*`, `kanban`, `file-upload`, `form`, `label`, `separator`, `alert` / `alert-dialog`, `hover-card`, `input-otp`. (`web/components/ui/index.ts` is the barrel.)

There's also a Qorven-flavored set under `web/components/qor/*` (e.g. `dropdown-menu`, `tooltip`, `command`) used by the chrome.

## Settings form primitives

`web/components/settings/sections/primitives.tsx` — the building blocks for Settings pages:

- **`Card`** — `{ id?, title, description?, headerRight?, children }` — bordered section card.
- **`Row`** — `{ label, hint?, children }` — label-left / control-right form row.
- **`Input`** — `{ value, onChange, type?, placeholder?, readOnly?, suffix?, className? }`.
- **`Btn`** — `{ variant: 'primary'|'ghost'|'danger', loading?, disabled?, onClick, … }`.
- Plus `Toggle`, `SaveBar`, and the `usePrefs()` hook in the same file.

## Soul (agent) avatars

`web/components/soul-card.tsx` exports **`soulGradient(name)`** → a deterministic `from-… to-…` gradient class. Render an avatar as a gradient circle with the first initial; that's the idiom used by Recent chats, the hub message bubbles, the mail/tasks pickers, etc.

```tsx
<div className={cn('flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br text-2xs font-semibold text-white', soulGradient(name))}>
  {name[0]?.toUpperCase()}
</div>
```

Status rings/dots use the `--soul-*` tokens (see [tokens.md](tokens.md)).

## Command palette (global search)

`web/components/modals/command-palette.tsx`, mounted once in `app/layout.tsx`. Open state lives in the **store** (`commandPaletteOpen` / `setCommandPaletteOpen`), so two triggers share it:

- **⌘K / Ctrl-K** (keydown listener in the palette), and
- the **header search pill** (calls `setCommandPaletteOpen(true)`).

It searches in parallel: agents (souls), sessions, tickets, tasks, memories, drive files, and a static page list — grouped with section headers, arrow-key navigable. This is the one global search; per-section sidebar search boxes were removed in favor of it.

## Confirmation modals

Simple fixed-overlay pattern (no extra dependency): `role="alertdialog"` on the inner div, Escape-to-close, overlay-click-to-close. For destructive actions, bind only Escape (let the user Tab to the button), not a global Enter. See `web/components/settings/sections/system-settings.tsx` (`FactoryResetModal`) for the reference.
