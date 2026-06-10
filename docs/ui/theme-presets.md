# Theme Presets

Qorven ships **6 full-palette themes**, selectable in **Settings → Appearance**. They are defined as `[data-theme="id"]` blocks in `web/css/config.qorven.css` and registered in `web/lib/theme-provider.tsx` (`THEME_PRESETS`).

## The model: neutral surfaces + vivid accent

The core principle (documented in the CSS): each preset keeps **neutral surfaces** (the bg/card/border/muted ramp) and only varies the **accent** (`--primary`/`--ring`), unless the preset is explicitly a *surface* theme. Values come from Tailwind's `--color-*` ramp — no hand-mixed hex. This keeps the UI crisp instead of washed-out/tinted.

## The presets

| id | Name | Accent | Surfaces |
|----|------|--------|----------|
| `violet` | Violet (default) | violet-600/500 | default neutral (zinc) |
| `slate` | Slate | zinc-900 / zinc-100 (monochrome) | neutral; dark variant uses the zinc ramp |
| `ocean` | Ocean | blue-600/500 | dark variant uses the **slate** ramp |
| `sand` | Sand | amber-600/500 | **warm** surfaces (stone ramp), light + dark |
| `midnight` | Midnight | indigo-600/400 | **true-black / OLED** dark surfaces (`#000`) |
| `graphite` | Graphite | teal-600/400 | **pure-neutral gray** surfaces (neutral ramp) |

`violet` and `ocean` only override the accent in light mode (surfaces stay default); their `.dark` blocks set richer surfaces. `slate`/`sand`/`midnight`/`graphite` define their own surface ramps.

## How it's applied

- `theme-provider.tsx` → `applyToDOM()` sets `document.documentElement.dataset.theme = settings.themePreset` (default `'violet'`).
- The CSS cascade: `[data-theme="x"]` (light) and `[data-theme="x"].dark` (dark) blocks sit **after** the base `:root`/`.dark`, so equal-specificity rules win by source order.
- Dark mode itself is still the `.dark` class on `<html>` (set by default in `app/layout.tsx`).

## Brand-color override (on top of a preset)

Users can override just the accent with a custom color (Settings → Appearance → brand color), independent of the preset:

- `ThemeSettings.primaryColor` (hex) + `primaryOklch` (the CSS value).
- **Empty string = no override** → the preset's own `--primary` shows through.
- When set, `applyToDOM` writes `--primary`/`--ring`/`--chart-1` inline on `<html>` (inline wins over the preset). When cleared, those inline props are removed.
- An inline `<script>` in `app/layout.tsx` (`themeScript`) applies a saved `primaryOklch` before React hydrates, to avoid a flash.
- `COLOR_PRESETS` in `theme-provider.tsx` are the quick brand-color swatches (Violet/Blue/Emerald/Rose/Amber/Cyan).

## Persistence

`theme-provider.tsx` stores settings in `localStorage` (`qorven-theme`, instant) **and** POSTs to `/user/preferences` (authoritative, syncs across devices). On mount it loads local first, then reconciles with the backend.

`ThemeSettings` also covers: `fontFamily`, `fontScale` (0.8–1.2), `borderRadius` (0–16px → `--radius`), `density` (compact/default/comfortable → `--density`), `dateFormat`, `timezone`.

## Adding a new preset

1. Add a `[data-theme="myid"]` block (and a `[data-theme="myid"].dark` block) in `config.qorven.css` — set `--primary`/`--ring` at minimum; add surface vars if it's a surface theme. Use `var(--color-*)` ramp values, not raw hex.
2. Add `{ id: 'myid', name: 'My Id', swatch: '#…' }` to `THEME_PRESETS` in `theme-provider.tsx` (`swatch` is the solid square shown in the picker — this is the one place a literal hex is fine, it's preview-only).
3. It appears automatically in Settings → Appearance.
