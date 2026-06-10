# Building an App — Layout, UI Bundle, Tools, Install

This is the exact, no-deviations procedure for building an internal Qorven app. `{appsDir}` is the resolved apps directory (e.g. `~/.qorven/apps` or `/var/lib/qorven/apps`).

> **Node.js / npm are NOT installed.** Do NOT run `npm install`, `npm run build`, vite, or any node command — they fail. The UI bundle is written as a **plain JavaScript file** directly with `write_file`. No build step, no `package.json`, no `vite.config.ts`.
> **Do NOT call `scaffold_app`** — it emits a Go-Wasm plugin (wrong format). Create files directly, then `install_app`.

## Required directory structure

```
{appsDir}/my-app/
├── app.yaml                      ← manifest (see `app-manifest`)
├── migrations/
│   └── 001_create_tables.up.sql  ← DB schema (see `db`)
├── tools/
│   └── my_tool.sh                ← server-side tool scripts
└── ui/
    └── frontend/
        └── bundle.js             ← plain-JS IIFE — the complete UI, NO build
```

## The 6 steps

1. `exec`: `mkdir -p {appsDir}/{slug}/migrations {appsDir}/{slug}/tools {appsDir}/{slug}/ui/frontend`
2. `write_file`: `{appsDir}/{slug}/app.yaml` (follow the `app-manifest` format exactly)
3. `write_file`: `{appsDir}/{slug}/migrations/001_create_tables.up.sql` (`CREATE TABLE IF NOT EXISTS …`)
4. `write_file`: each tool script in `{appsDir}/{slug}/tools/`; then `exec`: `chmod +x {appsDir}/{slug}/tools/*.sh`
5. `write_file`: `{appsDir}/{slug}/ui/frontend/bundle.js` (the IIFE below — this IS the complete UI)
6. `install_app`: `path={appsDir}/{slug}`

Then tell the user the app is ready (it appears under `/apps/{slug}`).

**Editing an existing app:** `cat` the current `app.yaml`/scripts/bundle, `write_file` the changed files, add a new migration if the schema changed (`002_…up.sql`), then `install_app` again to hot-reload.

## UI bundle — `ui/frontend/bundle.js`

A plain-JS IIFE. The host loads it via a script tag; React and the component library are already on the page. **Copy and adapt this pattern exactly:**

```js
(function() {
  var React = window.__QorvenApp.React;
  var h = React.createElement;
  var useState = React.useState;
  var useEffect = React.useEffect;
  var UI = window.__QorvenUI;          // Button, Card, Input, Text, Table, etc.
  var icons = window.__QorvenUI.icons; // all Lucide icons: icons.Trash2, icons.Plus, …
  var request = window.__QorvenApp.request;

  function MyPage() {
    var s = useState([]); var items = s[0]; var setItems = s[1];
    var inp = useState(''); var input = inp[0]; var setInput = inp[1];

    useEffect(function() { loadItems(); }, []);

    function loadItems() {
      request('/apps/my-app/tools/view_items', {method:'POST', body: JSON.stringify({args:{}})})
        .then(function(r) { if (!r.is_error) setItems(JSON.parse(r.content)); });
    }
    function addItem() {
      if (!input.trim()) return;
      request('/apps/my-app/tools/add_item', {method:'POST', body: JSON.stringify({args:{name: input}})})
        .then(function() { setInput(''); loadItems(); });
    }

    return h('div', {style:{padding:'20px', display:'flex', flexDirection:'column', gap:'16px'}},
      h('div', {style:{display:'flex', gap:'8px'}},
        h(UI.Input, {value: input, onChange: function(e){setInput(e.target.value)}, placeholder: 'Enter text...', style:{flex:1}}),
        h(UI.Button, {onClick: addItem}, 'Add')
      ),
      h('div', {style:{display:'flex', flexDirection:'column', gap:'8px'}},
        items.map(function(item) {
          return h('div', {key: item.id, style:{display:'flex', alignItems:'center', justifyContent:'space-between', padding:'8px 12px', background:'var(--muted)', borderRadius:'6px'}},
            h('span', null, item.name),
            h(UI.Button, {variant:'ghost', onClick: function(){deleteItem(item.id)}, style:{padding:'4px 8px'}}, h(icons.Trash2, {size:16}))
          );
        })
      )
    );
  }

  window.__QorvenApp.register({
    id: 'my-app',
    displayName: 'My App',
    pages: [{ id: 'home', path: 'home', label: 'Home', component: MyPage }]
  });
})();
```

### Bundle rules
- ALWAYS use `React.createElement` (aliased `h`) — **never JSX** in the bundle.
- Use `var` + manual destructure; avoid arrow functions in `useState` calls.
- `request()` returns `{content: string, is_error: boolean}` — parse with `JSON.parse(r.content)`, do NOT call `r.json()`.
- Call tools at `/apps/{slug}/tools/{name}` with `{method:'POST', body: JSON.stringify({args:{…}})}`.
- **Available `window.__QorvenUI` components** (use only these — don't invent names): `Button`, `Card`, `Input`, `Checkbox`, `Badge`, `Avatar`, `Separator`, `Skeleton`, `Select`, `Tabs`, `Dialog`, `Drawer`, `Sheet`, `Popover`, `Tooltip`, `Switch`, `Progress`, `Textarea`, `Label`, `Toggle`, `Table`, `TableBody`, `TableCell`, `TableHead`, `TableHeader`, `TableRow`, `Text`, `Accordion`, `Collapsible`. Plus `icons` (all Lucide) and `cn` (classnames helper). There is NO `List`/`ListItem`/`IconButton` — use plain `<div>` rows + `icons.Trash2` etc.
- For styling, prefer the CSS variables (`var(--muted)`, `var(--primary)`, …) — see the `ui` topic. Don't hardcode brand hex.

## Tool scripts — `tools/*.sh`

Server-side shell scripts. **Args arrive on STDIN as JSON** (not as `$1`). App settings arrive as `QORVEN_APP_{UPPER_KEY}` env vars. The DB DSN is `$QORVEN_DB_DSN`.

```bash
#!/bin/bash
INPUT=$(cat)                          # read JSON args from stdin
NAME=$(echo "$INPUT" | jq -r '.name')
ID=$(echo "$INPUT" | jq -r '.id')
# IDs are strings in JSON — always compare as strings in jq:
#   jq --arg id "$ID" 'map(select(.id != $id))'              ← CORRECT
#   jq --arg id "$ID" 'map(select(.id != ($id|tonumber)))'   ← WRONG (type mismatch)
# Write atomically:  jq '…' data.json > data.json.tmp && mv data.json.tmp data.json
# Query Postgres:    psql "$QORVEN_DB_DSN" -c "INSERT INTO my_app_items(name) VALUES('$NAME')"
```

Make scripts executable (`chmod +x`). The manifest must include `permissions: [tool_register]` or tools load with 0 entries.
