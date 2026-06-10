# app.yaml — the App Manifest

Every Qorven app is declared by a single `app.yaml` at the app root. The runtime (`internal/apps`) reads it on `install_app`.

## Full example

```yaml
slug: my-app            # REQUIRED. Use hyphens, not underscores (todo-app, not todo_app).
display_name: My App
version: 0.1.0
description: What this app does
author: qorven
icon: 📝                # emoji OR a Lucide icon name (e.g. "FileText"). Shown in rail/topbar.
pinned_rail: false      # show in the left rail by default
pinned_topbar: false    # show in the top bar by default

permissions:
  - db_write            # write to the database
  - tool_register       # REQUIRED for tool scripts to execute — without it, tools load with 0 entries

migrations_dir: migrations   # folder of numbered .sql migrations (see the `db` topic)

# Schema-driven settings form, auto-rendered at /apps/{slug}/settings.
# Each setting is passed to tool scripts as env var QORVEN_APP_{UPPER_KEY}.
settings:
  - key: api_key
    label: API Key
    type: secret         # text | secret | number | boolean | select | url
    description: Your service API key
    placeholder: sk-...
    required: true
    help_url: https://example.com/keys
  # select example:
  # - key: model
  #   label: Model
  #   type: select
  #   options:
  #     - { value: gpt-4o, label: GPT-4o }
  #   default: gpt-4o

frontend:
  bundle: ui/frontend/bundle.js   # the plain-JS IIFE (see `scaffold`)
  pages:                          # top-level pages, mounted at /apps/{slug}/{path}
    - id: home
      label: Home
      icon: Home
      path: home
  # agent_tabs:   tabs injected into the agent workspace
  # setting_tabs: tabs injected into Settings

tools:
  - name: add_item
    description: Add an item
    command: tools/add_item.sh    # path relative to app root
    parameters:                   # JSON-schema-ish; what the agent/UI passes
      type: object
      properties:
        name: { type: string, description: "Item name" }
      required: [name]
    # timeout: 30                 # seconds; 0 → 30s default

# hooks:                          # run a command on a platform event
#   - event: agent.message
#     command: hooks/on_message.sh

# data_source:                    # connector-style scheduled data pull into a snapshot
#   enabled: true
#   schedule: "0 9 * * *"         # standard cron
#   tool: fetch_prices
#   result_key: prices

# scope: workspace                # workspace (default) | agent | team
# owner_agent_id / owner_team_id  # set when scope is agent/team
```

## Field reference (from `internal/apps/types.go`)

- **Identity:** `slug` (hyphens), `display_name`, `version`, `description`, `author`, `icon` (emoji or Lucide name), `icon_url`.
- **Pinning:** `pinned_rail`, `pinned_topbar` (booleans) — default placement in rail/top bar.
- **`permissions`:** allowlisted strings; `db_write` and `tool_register` are the common ones. Tools won't run without `tool_register`.
- **`migrations_dir`:** defaults to `migrations`. See the `db` topic for rules.
- **`settings`:** array of `{ key, label, type, description?, placeholder?, required?, default?, options?, help_url? }`. Types: `text | secret | number | boolean | select | url`. Rendered at `/apps/{slug}/settings`; each is injected into tool scripts as `QORVEN_APP_{UPPER_KEY}`.
- **`frontend`:** `bundle` (path; default `frontend/bundle.js`), `pages[]` `{ id, label, icon, path }` (mounted at `/apps/{slug}/{path}`), `agent_tabs[]`, `setting_tabs[]` `{ id, label, icon, order }`.
- **`tools`:** `{ name, description, command, parameters, timeout }`. `command` is a path relative to the app root; args arrive on **stdin as JSON**.
- **`hooks`:** `{ event, command }` — run on a platform event.
- **`data_source`:** `{ enabled, schedule (cron), tool, args, result_key }` — scheduled pull into a snapshot JSONB.
- **`scope`:** `workspace` (default) | `agent` | `team`; set `owner_agent_id`/`owner_team_id` for the latter two.

## What the runtime does with it
- Serves the bundle at `/app-assets/{slug}/bundle.js`.
- Mounts pages at `/apps/{slug}/{path}`.
- Registers tools (callable at `/apps/{slug}/tools/{name}`) — see `scaffold`.
- Applies the app's migrations to the database.
- Renders the settings form and injects settings into tool env.
