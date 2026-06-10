# Building on Qorven — Overview

Qorven is a **platform**. You (an agent) build apps on top of it, like WordPress plugins/themes. Qorven ships the runtime, the UI system, and the SDK — you build the app. We never ship example apps; we give the wrapper, users build anything.

## What an "app" is

A Qorven app installs into `/apps/{slug}` and can add: **pages** (UI under `/apps/{slug}/{path}`), **tools** (server-side scripts the agent and UI can call), **settings** (a schema-driven form), and its **own database tables** (scoped migrations). It's declared by a single `app.yaml` manifest. This is the WordPress-plugin analogy: the app extends Qorven without modifying core.

## Internal vs external apps

- **Internal app** — renders inside the authenticated Qorven UI (the common case). "A todo app for my own use", "a stock dashboard", "a CRM for our office". Build these with the app platform (see `scaffold` + `app-manifest`).
- **External-facing app** — serves a public audience (e.g. a customer booking page) AND connects back to internal data/services + an internal admin view. Requires the external app contract + tunnel exposure. See `external` (in progress).

When a user gives a plain request, infer which kind it is — don't ask them about internals.

## The build flow (internal app)

1. `mkdir` the app dirs under the apps directory.
2. Write `app.yaml` (the manifest — see `app-manifest`).
3. Write DB migration(s) (see `db`).
4. Write tool scripts (server-side, args via stdin).
5. Write the UI bundle as a **plain-JS IIFE** (no npm/build — see `scaffold`).
6. `install_app` with the app directory path.

> **Do NOT call `scaffold_app`** — it creates a Go-Wasm plugin, which is the wrong format for normal apps. Use `write_file` + `exec` to create the files directly, then `install_app`. (`scaffold_app` exists for advanced Wasm plugins only.)

## Retrieve more detail

Call `get_builder_knowledge(topic)` for:
- `app-manifest` — the full `app.yaml` schema.
- `scaffold` — the exact directory layout, the IIFE UI bundle pattern, tool scripts, install.
- `ui` — the design system (tokens, components, type scale) your UI must follow.
- `db` — migration rules, tenant/user scoping, data access.
- `external` — external-facing apps + tunnel (in progress).

## Other self-build paths (already in your prompt)

- **Dashboards** — JSON block config POSTed to `/v1/dashboards` (no app needed) for quick KPI/chart views.
- **`workspace_builder`** — spin up a whole agent team + dashboard from a template (crm/support/devops/…).
- Use an app (this guide) when the user needs real pages, persisted data, and tools — not just a dashboard view.
