# External-Facing Apps & Exposure (in progress)

> **Status:** the internal app platform is fully supported today. External-facing serving (public surfaces + tunnel exposure) is being built (initiative items 5–6). This topic describes the intended shape; verify capability before promising it to a user.

## The two pieces

1. **External-facing app** — an app that serves a public audience (not logged-in Qorven users) AND connects back to internal data/services + an internal admin view. Example shape: a customer appointment/booking page where customers submit, the data lands in internal DB, and an internal admin page (a normal app page) manages it.
2. **Exposure** — Qorven's backend runs on port `8486` and is not reachable externally by default. A tunnel (e.g. Cloudflare Tunnel) will expose ONLY declared public surfaces on `:80/:443`/a custom hostname, keeping the admin backend private.

## Contract (intended)
- An app marks which pages/routes are **public** vs authenticated.
- Public surfaces write through a **controlled bridge API** (never direct DB / never internal-only endpoints).
- Input is validated; rate-limited; SSRF/deny-group protections hold.
- The internal admin view is a normal authenticated app page.

## For now
- Build the **internal** app fully (pages, tools, migrations) using the other topics.
- If a user asks for a public/external surface, tell them external exposure is being finalized and build the internal side first so it's ready to expose.

(This doc will be expanded when items 5–6 land.)
