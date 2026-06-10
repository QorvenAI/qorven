# UI System (for app builders)

Your app's UI must follow Qorven's design system so it looks native. Full human reference: `docs/ui/` in the repo. Essentials for an app bundle:

## Use the host's components
The UI bundle gets React + a component library on `window.__QorvenUI` (see `scaffold` for the full list: `Button`, `Card`, `Input`, `Table`, `Tabs`, `Dialog`, `Badge`, `Select`, `Textarea`, `Switch`, `icons`, `cn`, …). **Use these — don't ship your own component library or invent names.** They already consume the design tokens, so they theme automatically (light/dark, all presets).

## Color — tokens, never raw hex
Style with the CSS variables, so the app follows the user's theme:
- Surfaces: `var(--background)`, `var(--card)`, `var(--muted)`, `var(--accent)`, `var(--popover)`
- Text: `var(--foreground)`, `var(--muted-foreground)`, `var(--primary-foreground)`
- Accent/edges: `var(--primary)`, `var(--border)`, `var(--ring)`
- Radius: `var(--radius)` (and `--radius-sm/md/lg/xl`)

Never hardcode a hex like `#5271ff` — use `var(--primary)`. Genuine third-party brand colors are the only exception and belong in core CSS as `--channel-*`/`--connector-*`, not inline.

## Type scale — no arbitrary px
Sizes: `text-2xs` (11) badges/status only · `text-xs` (12, the floor for normal text) · `text-2sm` (13) · `text-sm` (14) · `text-base` (16) · `text-lg` (18) · `text-xl`+ (headings). Never `text-[Npx]`. Headings are semibold or heavier — no thin weights.

## Layout conventions
- Pad page content (~20px); use flex/grid with consistent gaps (8/12/16px).
- Inside the main app: every titled page uses the standard header pattern (`CanvasHeader`/`PageShell` in core); in an app bundle, keep a simple titled container and let the host chrome frame it.
- Don't build a second in-canvas sidebar — navigation belongs to Qorven's rail/sidebar; your app provides pages, the host lists them.

## Where to read more
Call `get_builder_knowledge('ui')` returns this. For the deep reference (tokens table, theme presets, sidebar architecture, component catalog) the repo has `docs/ui/{README,tokens,theme-presets,typography,layout,sidebar,components}.md`.
