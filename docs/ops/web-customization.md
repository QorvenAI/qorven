# Customizing the web UI without rebuilding the binary

Qorven ships one binary with the web UI embedded via `go:embed`. You do **not** need to touch Go to customize the UI — the gateway prefers an external `web/` directory over the embedded copy when one is available.

## Resolution order

At boot, the gateway looks for `index.html` in this order and serves the first match:

1. `[server] web_dir = "…"` in `config.toml` (explicit override)
2. `./web/` next to the binary
3. `<exe-dir>/web/`
4. `~/.qorven/web/`
5. **Embedded UI** (baked into the binary at release time)
6. **Nothing** — API-only mode (the frontend 404s; API still works)

## Recipe — ship a custom UI

```bash
# 1. Clone the repo and edit whatever you want under web/.
git clone https://github.com/qorvenai/qorven
cd qorven/web

# 2. Build the static export.
pnpm install
QORVEN_STATIC=1 pnpm build   # → web/out/

# 3. Copy it somewhere the qorven process can read.
sudo mkdir -p /var/lib/qorven/web
sudo cp -r web/out/. /var/lib/qorven/web/
sudo chown -R qorven:qorven /var/lib/qorven/web

# 4. Point config.toml at it.
sudo tee -a /etc/qorven/config.toml <<EOF

[server]
web_dir = "/var/lib/qorven/web"
EOF

# 5. Restart.
sudo systemctl restart qorven
```

No Go, no rebuild. Reverting to the bundled UI is a `web_dir = ""` away.

## Developer mode

If you're iterating on the frontend, skip the static export and run the Next.js dev server alongside the Go backend:

```bash
make dev     # backend on :4200, Next.js dev on :3000
# point your browser at http://localhost:3000 — Next proxies /api/* to :4200
```

The embedded UI is inert in this setup; only the dev server is reached.

## What "custom" actually means

The embedded UI is a vanilla Next.js 16 static export. You can:

- Swap branding (`web/lib/branding.ts`, logos under `web/public/`).
- Add or remove entire pages under `web/app/`.
- Replace components under `web/components/`.
- Change Tailwind theme in `web/css/`.

What you can't change without touching the backend:

- API shape (`/v1/*`, `/auth/*`, `/ws*`). The Go gateway is the contract.
- The `/api` prefix alias (see `internal/gateway/middleware.go`). Custom UIs can still use absolute `/api/v1/...` URLs — that alias exists precisely so external UIs and the embedded UI share one client.

## Warning

If you publish or redistribute a customized UI, follow the licensing notices in `LICENSE` and `NOTICE`. The FSL-1.1-ALv2 license allows you to run, modify, and redistribute under the listed terms.
