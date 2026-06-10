# Typography

## Type scale

Use named sizes only — **never `text-[Npx]`**. The two custom sizes (`text-2sm`, `text-2xs`) are defined in `@theme` in `web/css/config.qorven.css`; the rest are Tailwind defaults.

| Class | px | Use |
|-------|----|-----|
| `text-2xs` | 11 | Dense badges / status only |
| `text-xs` | 12 | Labels, meta — **floor for normal UI text** |
| `text-2sm` | 13 | Slightly denser secondary text |
| `text-sm` | 14 | Secondary text, table cells, default body in chrome |
| `text-base` | 16 | Body copy |
| `text-lg` | 18 | Larger emphasis |
| `text-xl` | 20 | **Page header title** (`CanvasHeader` uses `text-xl font-semibold`) |
| `text-2xl` / `text-3xl` | 24 / 30 | Section headings, hero |

12px is the visual floor for normal UI text. 11px (`text-2xs`) is reserved for dense badges and status indicators.

Custom size definitions (config.qorven.css):
```css
--text-2sm: 0.8125rem;   /* 13px */
--text-2xs: 0.6875rem;   /* 11px */
```

## Fonts

Wired in `web/app/layout.tsx` via `next/font/google`:

- **Inter** — UI sans. Loaded with weights 400/500/600/700, exposed as `--font-inter`, and set as `--font-sans` in `config.qorven.css`. `<body>` gets `inter.className`. **Do not define `font-family` anywhere else.**
- **JetBrains Mono** — code/mono, exposed as `--font-mono`.

The fallback stacks live in `config.qorven.css` (`--font-sans`, `--font-mono`). The theme provider can swap `--font-sans` at runtime (Settings → Appearance → font), and `fontScale` sets the root `font-size` (0.8–1.2 → 80%–120%).

Body rendering uses `-webkit-font-smoothing: antialiased` (`styles.css`) — the Qorven standard. **No thin weights** — headings/titles are `font-semibold` (600) or heavier.

## Icons

Lucide icons render at **1.5px stroke** (set in `styles.css` on `.lucide, [data-lucide]`; `config.qorven.css` also sets `svg.lucide { stroke-width: 2.25 }` as a base — the `styles.css` rule is the effective one for line icons). Brand/gradient SVGs shipped inside cards keep their own stroke.

## Enforcement

`web/scripts/check-design-tokens.sh` (wired into pre-commit) **fails any commit** that introduces, in staged `web/**/*.tsx|ts` (outside `web/css/`, `theme-provider.tsx`, `appearance-settings.tsx`):

- an arbitrary px font size: `text-[Npx]`
- a raw 6-digit hex: `#rrggbb`

The guard scans the **whole staged file**, so editing a file with pre-existing violations forces fixing them — map `text-[10px]`→`text-2xs`, `text-[12px]`→`text-xs`, `text-[13px]`→`text-2sm`, etc.
