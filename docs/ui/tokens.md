# Design Tokens

All color, radius, and status tokens are CSS variables defined in **`web/css/config.qorven.css`** and mapped into Tailwind via `@theme inline`. Use the Tailwind utility (`bg-primary`, `text-muted-foreground`, …) — never a raw hex.

## How it works

1. `:root { … }` defines the **light** values. Most reference Tailwind's built-in color ramp (`var(--color-zinc-100)` etc.) rather than hand-mixed hex.
2. `.dark { … }` overrides them for dark mode (the `<html>` element has `class="dark"` by default — see `app/layout.tsx`).
3. `@theme inline { --color-*: var(--*) }` exposes each token to Tailwind so `bg-card`, `text-foreground`, `border-border`, etc. all resolve.
4. Theme presets (`[data-theme="…"]`) layer on top — see [theme-presets.md](theme-presets.md).

## Color roles

| Token | Tailwind utility | Role |
|-------|-----------------|------|
| `--background` / `--foreground` | `bg-background` / `text-foreground` | App canvas + default text |
| `--card` / `--card-foreground` | `bg-card` / `text-card-foreground` | Card surfaces |
| `--popover` / `--popover-foreground` | `bg-popover` / … | Menus, dropdowns, palette |
| `--primary` / `--primary-foreground` | `bg-primary` / `text-primary-foreground` | Brand accent (buttons, active state) |
| `--secondary` / `--secondary-foreground` | `bg-secondary` / … | Secondary surfaces |
| `--muted` / `--muted-foreground` | `bg-muted` / `text-muted-foreground` | Subtle bg + secondary text |
| `--accent` / `--accent-foreground` | `bg-accent` / … | Hover/active row background |
| `--destructive` / `--destructive-foreground` | `bg-destructive` / … | Danger actions |
| `--border` | `border-border` | All borders/dividers |
| `--input` | `border-input` | Form-field borders |
| `--ring` | `ring-ring` / `outline-ring` | Focus ring (also the focus-visible outline, see `styles.css`) |
| `--chart-1`…`--chart-5` | — | Chart series colors |

## Radius

`--radius: 0.5rem` (8px) is the base. The scale (in `@theme inline`):

| Token | Value | Utility |
|-------|-------|---------|
| `--radius-sm` | base − 4px | `rounded-sm` |
| `--radius-md` | base − 2px | `rounded-md` |
| `--radius-lg` | base | `rounded-lg` |
| `--radius-xl` | base + 4px (12px) | `rounded-xl` (card radius) |

The theme provider can change `--radius` at runtime (Settings → Appearance → border radius).

## Soul status colors

`--soul-idle/thinking/running/offline/error` → `bg-soul-idle` etc. Used by agent status dots/rings.

## Brand colors (channels & connectors)

Genuine third-party brand colors are the **only** place raw hex is allowed — and only here in CSS, never inline:

- Channels: `--channel-telegram`, `--channel-slack`, `--channel-discord`, `--channel-whatsapp`, `--channel-email`, `--channel-github`, `--channel-teams`, `--channel-sms`, `--channel-webchat`, `--channel-webhook`.
- Connectors: `--connector-notion`, `--connector-stripe`, `--connector-jira`, `--connector-google`, `--connector-gmail`.

Some flip in dark mode (e.g. `--channel-github` → `zinc-50`). Reference them as `var(--channel-slack)` — **if you need a brand color in a component, add a token here, don't inline a hex.**

## Using a token

```tsx
// ✅ semantic utilities
<div className="bg-card text-card-foreground border border-border rounded-xl">
<span className="text-muted-foreground">subtitle</span>
<button className="bg-primary text-primary-foreground">Save</button>

// ❌ never
<div style={{ background: '#161b25' }}>
<div className="text-[#6b7a99]">
```

The pre-commit guard rejects raw 6-digit hex in `web/**/*.tsx|ts` (outside `web/css/` and the two theme files). See [typography.md](typography.md#enforcement).
