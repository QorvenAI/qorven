# AGENTS.md — The Qorven Architecture Map

> **Audience:** AI coding agents (Claude Code, IDE Copilots, custom Anthropic SDK agents) acting on behalf of a developer extending Qorven.
>
> This file is the single source of truth for *where* things live and *which rules are unnegotiable*. If you're about to write code that touches the database, a route, a tool, or auth — read the relevant section first. Guessing causes multi-tenant data leaks.
>
> **Scope:** extending Qorven with custom plugins, tools, frontends, or integrations. For changes to the core itself, read `backend/QORVEN.md`, `backend/CONTRIBUTING.md`, and the Phase-by-Phase commit history.

## 0. Repository layout

```
qorven-mono/
├── AGENTS.md              ← you are here
├── backend/               Go monolith: gateway, orchestrator, stores, tools
│   ├── internal/
│   │   ├── gateway/       HTTP router + middleware chain + handler functions
│   │   ├── store/         Postgres pool + Queryable + WithTenantTx helper
│   │   ├── plans/         Plan-graph store
│   │   ├── approvals/     Approval state machine
│   │   ├── permissions/   Tool-gate permission service
│   │   ├── orchestrator/  Graph runtime + per-tenant sweeper manager
│   │   ├── tools/         Built-in tool registry + destructive manifest
│   │   ├── byom/          Timeouts tunables for BYOM deployments
│   │   ├── serviceaccounts/ Tenant-scoped SA store
│   │   ├── auth/          Users + JWT + API keys
│   │   └── deployment/    single_tenant vs multi_tenant mode flag
│   └── migrations/        Numbered SQL migrations; latest is 042
└── web/                   Next.js frontend (its own AGENTS.md applies there)
```

## 1. The Golden Rules (break one and you leak tenant data)

### 1.1 Every tenant-bound query goes through `WithTenantTx` or a store method that calls `store.FromContext`.

- Multi-tenant mode wraps each authenticated HTTP request in a Postgres transaction scoped to the user's `tenant_id` via `SET LOCAL app.current_tenant_id = <uuid>`. The transaction is stashed on the request context by `TenantScopeMiddleware`.
- Every store method that touches a tenant-bound table (`plans`, `sessions`, `permission_requests`, `wakeup_requests`, `service_accounts`, `plan_nodes`, `plan_edges`, `approvals`, plus legacy-policied `agents`, `tasks`, `memories`, `cron_jobs`, etc.) MUST pull its `Queryable` from the context:

  ```go
  func (s *Store) q(ctx context.Context) store.Queryable {
      return store.FromContext(ctx, s.pool)
  }

  func (s *Store) GetThing(ctx context.Context, id string) (*Thing, error) {
      return s.q(ctx).QueryRow(ctx, `SELECT ... FROM things WHERE id = $1`, id).Scan(...)
  }
  ```

- Direct `s.pool.Query*(...)` calls inside a tenant-bound store are a Phase-4 regression and will be rejected in review. The RLS backstop in Postgres (`FORCE ROW LEVEL SECURITY`) will also deny them silently in multi-tenant, so the bug surfaces as "everything returns empty."
- Background jobs (sweepers, schedulers) that legitimately cross tenants call the store methods from a bypass-pool DB. The pool's `AfterConnect` sets `app.bypass_rls = 'on'`, which the RLS policies honor.

**Key files to read:**
- `backend/internal/store/queryable.go:1` — `Queryable`, `WithTx`, `FromContext`
- `backend/internal/store/db.go:122` — `WithTenantTx`
- `backend/internal/gateway/tenant_scope_middleware.go:41` — `TenantScopeMiddleware`

### 1.2 Every destructive tool goes through `permissions.WrapLazy`.

- A *destructive* tool is one that mutates user-owned state: `exec`, `apply_patch`, `write_file`, `gh_push_file`, `gh_merge_pr`, `undo`. The full list lives in `backend/internal/tools/destructive_manifest.go`.
- CI enforces this via `TestDestructiveManifest_AllWrapped` in `backend/internal/gateway/destructive_manifest_test.go:28`. Add your tool's name to the manifest AND register it wrapped — miss either half and the build goes red.
- Wrap pattern (from `backend/internal/gateway/gateway.go:1289`):

  ```go
  reg.Register(permissions.WrapLazy(
      func() *permissions.Gate { return gw.permissionGate },
      tools.NewMyDestructiveTool(...),
      permissions.GatedToolOptions{
          Reason:      "Writes a file to the user's workspace",
          RequestedBy: "agent",
      },
  ))
  ```

- `WrapLazy` defers the gate lookup until `Execute` time so the wrapped tool can be registered before `ensureProtocolSurfaces` builds the gate. Do not resolve the gate eagerly.

**Key files to read:**
- `backend/internal/permissions/wrap.go:65` — `WrapLazy`
- `backend/internal/permissions/gate.go:17` — `Gate` (the permission service)
- `backend/internal/tools/destructive_manifest.go:1` — the CI-enforced manifest

### 1.3 The DB role must be NOSUPERUSER NOBYPASSRLS in multi-tenant mode.

- `backend/internal/store/db.go:169` exposes `AssertNotSuperuser(ctx)`. The gateway's `ensureProtocolSurfaces` calls it at boot when `deployment.IsMultiTenant()` is true and **panics** if the role has `rolsuper` or `rolbypassrls`. See `backend/internal/gateway/protocol.go:99-105`.
- Superusers bypass every RLS policy regardless of `FORCE`. A decorative RLS boundary is worse than no boundary — it creates false confidence.
- Your local dev DB likely uses a superuser role (the default in docker-compose). That's fine for single-tenant development, but the gateway will refuse to boot in multi-tenant mode until you create a restricted role:

  ```sql
  CREATE ROLE qorven_app LOGIN PASSWORD '...' NOSUPERUSER NOBYPASSRLS;
  GRANT ALL ON ALL TABLES IN SCHEMA public TO qorven_app;
  GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO qorven_app;
  GRANT USAGE ON SCHEMA public TO qorven_app;
  ```

### 1.4 Service account creation: `Add(AddInput{TenantID: ...})` or `AddGlobal(...)`, never "just Add without tenant."

- `serviceaccounts.Store.Add` requires a non-empty `TenantID`. A tenant-bound SA only bypasses `authorize()` for resources in the same tenant.
- `serviceaccounts.Store.AddGlobal` creates a NULL-tenant (cross-tenant) service account. Reserved for operator-installed infra actors (`system`, `orchestrator`, `qoros`). Using `AddGlobal` for a tenant-created SA is a security incident waiting to happen. The API separation is deliberate; treat it as a review gate.

**Key file:** `backend/internal/serviceaccounts/store.go:96` (`Add`) and `:131` (`AddGlobal`).

## 2. The HTTP Request Flow

Every `/v1/*` request passes through this chain — read the exact source before extending:

```
          client HTTP req
               │
               ▼
  ┌───────────────────────────────────┐
  │ chi router                        │
  │   (backend/internal/gateway/     │
  │    routes_v1.go:24)              │
  └──────────────┬────────────────────┘
                 │
         r.Use(AuthMiddlewareV2)
      (auth_handlers.go:178)
                 │
         r.Use(TenantScopeMiddleware)
      (tenant_scope_middleware.go:41)
                 │
                 ▼
  ┌───────────────────────────────────┐
  │ handler (e.g. handleGetPlan)     │
  │   calls gw.plans.GetPlan(ctx, …)  │
  │   store method reads ctx-tx       │
  │   RLS filters rows server-side    │
  └───────────────────────────────────┘
```

### 2.1 AuthMiddlewareV2

- File: `backend/internal/gateway/auth_handlers.go:178`
- Resolution order: JWT cookie → `Authorization: Bearer <jwt>` → `?token=<jwt>` → `qorven_app_token` gateway token → API key (prefix `qk_`) → legacy `api_keys` table lookup → dev-mode admit (single-tenant only) → 401.
- Multi-tenant mode CLOSES the dev-mode admit. A gateway booted with `deployment_mode=multi_tenant` refuses every unauthenticated request regardless of `cfg.Auth.Token`.

### 2.2 TenantScopeMiddleware

- File: `backend/internal/gateway/tenant_scope_middleware.go:41`
- In single-tenant mode: **no-op**. The pool's `AfterConnect` already set `app.bypass_rls = 'on'`, so RLS is inert.
- In multi-tenant mode with an authenticated user: `BeginTx` → `SET LOCAL app.current_tenant_id = '<user.TenantID>'` + `SET LOCAL app.tenant_id = '<user.TenantID>'` (both names, to satisfy pre-Phase-4 policies) → stash the tx on ctx via `store.WithTxHandle` → delegate to the handler.
- End-of-request disposition:
  - Status ≥500 OR ctx cancelled OR `PoisonTx(ctx)` called → rollback.
  - Panic recovered → rollback then re-raise.
  - Otherwise → commit.
- **For SSE/streaming handlers:** call `store.ReleaseTxEarly(ctx)` after your initial tenant-scoped reads so the Postgres connection returns to the pool. Your stream body cannot make tenant-scoped DB calls after release. See `queryable.go:115`.
- **To force rollback after `WriteHeader(200)`:** call `gateway.PoisonTx(ctx)`. Used for "I can't change the status code but this tx absolutely must not commit."

### 2.3 `authorize(ctx, scope)`

- File: `backend/internal/gateway/authorize.go:58`
- Single source of truth for session-bound / plan-bound ops. Handlers call it via thin adapters (`authorizeForPlan`, `authorizeSessionID`). `apicommands.Server.OwnerCheck` is wired to `gw.protocolOwnerCheck` which also routes through `authorize`.
- Rule selection is `deploymentConfig.IsMultiTenant(ctx)`. The two rule sets are documented in the function's header comment — read it before modifying.

## 3. Adding a new `/v1/*` endpoint

1. **Handler function** on `*Gateway`, by convention in the topic-named `*_handlers.go` file (e.g. `plan_handlers.go`, `agent_handlers.go`). Signature: `func (gw *Gateway) handleMyThing(w http.ResponseWriter, r *http.Request)`.
2. **Authorize** session- or plan-bound ops by calling `gw.authorize(r.Context(), AuthScope{SessionID: …})` OR `gw.planAuthorize(r, plan)`. Do not roll your own tenant check — `authorize` is the matrix-tested one.
3. **DB calls** from the handler MUST pass `r.Context()` down so the stores pick up the middleware's tx (`s.q(ctx).Query...`). Anything that calls `gw.db.Pool` directly from a handler is a regression.
4. **Register** in `backend/internal/gateway/routes_v1.go` inside `parent.Route("/v1", ...)`. Anywhere between the `r.Use(gw.AuthMiddlewareV2)` / `r.Use(gw.TenantScopeMiddleware)` block at the top and the closing `})` — the middleware applies to all children automatically.
5. **Mirror** the same handler in `phase2_auth_test.go`'s matrix if it is session- or plan-bound — the auth matrix is our regression guard for the admin/owner/cross-tenant cells.

Example delta against `routes_v1.go:40`:

```go
r.Get("/plans/{id}", gw.handleGetPlan)
r.Get("/plans/{id}/my-new-thing", gw.handleMyNewThing)   // <-- add here
```

## 4. Adding a new built-in tool

1. **Implement** the `tools.Tool` interface:

   ```go
   // backend/internal/tools/types.go:9
   type Tool interface {
       Name() string
       Description() string
       Parameters() map[string]any
       Execute(ctx context.Context, args map[string]any) *Result
   }
   ```

2. **File placement:** `backend/internal/tools/<topic>.go`, following existing conventions (`github.go`, `filesystem.go`, `sandbox.go`).
3. **Register** in the gateway bootstrap (`backend/internal/gateway/gateway.go` around line 1275-1305). For destructive tools wrap with `permissions.WrapLazy` as shown in §1.2.
4. **Destructive manifest:** if the tool mutates user-owned state, add an entry to `backend/internal/tools/destructive_manifest.go`'s `DestructiveTools` map. CI enforces that every manifest entry is wrapped.
5. **Tests:** mirror the existing patterns in `backend/internal/tools/*_test.go`.

## 5. Adding a Wasm/WASI plugin

Qorven's plugin runtime is a WebAssembly (WASI preview1) host backed by [tetratelabs/wazero](https://github.com/tetratelabs/wazero). A plugin is a `.wasm` file that reads JSON from STDIN and writes JSON to STDOUT. The host is in `backend/internal/plugins/wasm/`.

### 5.1 Sandbox guarantees (non-negotiable)

| Boundary | Enforcement |
| --- | --- |
| **No network** | Host only advertises `wasi_snapshot_preview1`. No `wasi_sockets`. A plugin that imports *any* other module fails at `LoadPlugin` before execution — see the structural check in `host.go:187`. |
| **No filesystem** | Host mounts zero preopen directories. Attempts to `fd_open` return EBADF. |
| **Memory cap** | `MaxMemoryPages × 64 KiB`, default 4 MiB. A plugin that grows past this traps with "out of memory" and the orchestrator gets a structured error. |
| **CPU timeout** | Every `Invoke` runs under `InvokeTimeout` (default 2s). Guest traps or is torn down at the next instruction boundary. |
| **State isolation** | Each `Invoke` instantiates a fresh module. Guest globals do NOT persist across calls. Cross-tenant state leaks via module globals are architecturally impossible. |

### 5.2 Guest contract

- Read a JSON payload from **STDIN**.
- Write a JSON reply to **STDOUT**.
- On success, `exit(0)`.
- On error, write a diagnostic to **STDERR** and `exit(1)`. The host captures both streams.
- Guest process is single-shot: when your `main` returns, the instance is torn down.

### 5.3 Writing a plugin (Go)

Any language that can target `GOOS=wasip1 GOARCH=wasm` works. The canonical example is in `backend/internal/plugins/wasm/testdata/echo_plugin.go`. Build with:

```bash
make wasm-testdata                  # rebuilds the sample
# or directly:
GOOS=wasip1 GOARCH=wasm go build -o myplugin.wasm myplugin.go
```

**Size tradeoff — standard Go vs. TinyGo:**

| Toolchain | Typical artifact | Startup | When to pick |
| --- | --- | --- | --- |
| Standard Go (wasip1) | ~3.2 MiB | ~300 ms | Single-tenant, prototyping, full stdlib |
| **TinyGo (wasi-p1)** | **~50 KiB** | ~15 ms | Multi-tenant production — 60× smaller |

Scaffold a TinyGo-ready plugin with `qorven agent init --runtime tinygo`. TinyGo requires a separate toolchain install (`brew install tinygo` / tinygo.org); the emitted `plugin/README.md` walks through it. The Go source is identical between runtimes — switching is a Makefile change, not a rewrite.

Minimal plugin body:

```go
package main

import (
    "encoding/json"
    "io"
    "os"
)

type Req struct {
    Query string `json:"query"`
}

type Reply struct {
    Answer string `json:"answer"`
}

func main() {
    raw, err := io.ReadAll(os.Stdin)
    if err != nil { os.Stderr.WriteString(err.Error()); os.Exit(1) }
    var req Req
    if err := json.Unmarshal(raw, &req); err != nil {
        os.Stderr.WriteString("bad json: " + err.Error()); os.Exit(1)
    }
    // ... do work ...
    reply := Reply{Answer: "hello, " + req.Query}
    out, _ := json.Marshal(reply)
    os.Stdout.Write(out)
}
```

### 5.4 Registering with the gateway

Two paths: **static** (boot-time, for platform-owned plugins) and **dynamic** (runtime upload, for tenants).

#### 5.4.1 Static (platform-owned)

Load once at gateway boot:

```go
host, err := wasm.NewHost(ctx, wasm.Config{})          // one per gateway
_ = host.LoadPlugin(ctx, "my_plugin", wasmBytes)
tool := wasm.NewBridgeTool(host, "my_plugin", wasm.ToolDescriptor{
    Name:        "my_plugin",
    Description: "Answers questions about X",
    Parameters:  map[string]any{ /* JSON schema for args */ },
})
gw.toolReg.Register(permissions.WrapLazy(
    func() *permissions.Gate { return gw.permissionGate },
    tool,
    permissions.GatedToolOptions{Reason: "…", RequestedBy: "agent"},
))
```

#### 5.4.2 Dynamic (tenant-uploaded) — Phase 5.2

Admin users upload `.wasm` binaries at runtime via REST. Rows live in `wasm_plugins` (RLS-scoped by tenant) and the orchestrator's `plugins.Loader` resolves them at plan-run time.

HTTP surface (all under `/v1/wasm-plugins`, admin-only for mutations):

| Method | Path | Auth | Body | Returns |
| --- | --- | --- | --- | --- |
| `POST` | `/v1/wasm-plugins` | Admin | `multipart/form-data` with fields `wasm` (file ≤8 MiB), `name`, `description`, `parameters` | 201 + sanitized plugin metadata (no binary) |
| `GET` | `/v1/wasm-plugins` | Any | — | 200 + `{plugins: […], count}` |
| `DELETE` | `/v1/wasm-plugins/{name}` | Admin | — | 200 on revoke; 404 if already-revoked |

Constraints:
- `name` matches `^[a-z][a-z0-9_]{0,62}$` (DB-level CHECK).
- `parameters` must be valid JSON if present.
- Upload bodies above 8 MiB return `413 payload_too_large`.
- Re-uploading the same name with a new binary *revokes* the previous row and creates a new active one — sha256 history preserved for audit.
- Every uploaded plugin is automatically wrapped in `permissions.WrapLazy` by the `plugins.Loader` — plugins do NOT bypass the permission gate.
- **Reserved names (Phase 6).** Plugin names that collide with a platform-reserved tool are rejected with `400 reserved_name`. The closed list lives in `backend/internal/tools/reserved_manifest.go` and covers every destructive built-in (`exec`, `write_file`, `gh_push_file`, …) plus core orchestration primitives (`room_post`, `memory_search`, `spawn`, …). Enforcement runs at three layers: Store.Upload (write-time rejection), HTTP handler (surfaces the specific error code), Loader (defense-in-depth against historical / smuggled rows). Pick a distinct name — there is no tenant-side override.

Consuming the loader from orchestration code:

```go
loader := registry.NewLoader(store, wasmHost, gateGetter, logger)
tools, err := loader.ToolsForTenant(ctx, user.TenantID)
// register `tools` on a per-run scratch registry passed to the agent loop
```

### 5.5 Observability

Wasm invocations emit Prometheus metrics on `/metrics`:

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `plugins_wasm_invocations_total` | counter | plugin, tenant, outcome | Per Invoke call. `outcome ∈ ok \| timeout \| trap \| exit_nonzero \| truncated_stdout`. `truncated_stdout` fires alongside the primary outcome on the same call. |
| `plugins_wasm_duration_ms_sum` | counter | plugin, tenant | Sum of invocation durations. Divide by count for avg. |
| `plugins_wasm_duration_ms_count` | counter | plugin, tenant | Count of invocations recorded against sum. |
| `plugins_wasm_load_errors_total` | counter | (none) | `LoadPlugin` failures (malformed module, disallowed imports). |

### 5.5 Host configuration knobs

All in `wasm.Config`; sane defaults ship in the package. Raise only when a specific plugin requires more — every relaxation is a security decision.

| Field | Default | When to raise |
| --- | --- | --- |
| `MaxMemoryPages` | 64 (4 MiB) | A plugin parses large documents |
| `InvokeTimeout` | 2s | A plugin intentionally blocks on slow remote work (note: it can't do network, so this usually means "long CPU work") |
| `MaxStdinBytes` | 256 KiB | Payload is larger than that (consider splitting first) |
| `MaxStdoutBytes` | 1 MiB | Response is larger (consider paging) |

## 6. Frontend extensibility (Next.js)

- The canonical first-party web app lives in `web/`. It authenticates against `/auth/login` (POST) to obtain a JWT cookie + response token. The JWT pattern is documented in `web/AGENTS.md`.
- **Canonical custom UI template: `examples/custom-ui-nextjs/`.** It builds standalone (`pnpm build` on the current tree is green) and demonstrates:
  - `lib/auth.ts`: in-memory JWT store (NOT localStorage) + httpOnly cookie bootstrap.
  - `lib/api.ts`: `authFetch()` wrapper that attaches `Authorization: Bearer` + cookies.
  - `lib/events.ts`: typed discriminated union mirroring `backend/internal/api/events/types.go`.
  - `hooks/useQorvenSocket.ts`: WebSocket with exponential-backoff reconnect (1s→30s cap) and 20s ping keep-alives.
  - `hooks/useGraphState.ts`: derives live plan-graph state from the event stream.
  - `components/GraphVisualizer.tsx`: state-colored node rendering with a "running" pulse.
  - Its own `AGENTS.md` with example-local rules.
- **Note on SSE vs WebSocket:** Qorven's first-party real-time transport is WebSocket (`backend/internal/gateway/gateway.go:1512`, `r.Get("/ws/realtime", ...)`). The `text/event-stream` responses in the backend are for LLM streaming (`backend/internal/gateway/handlers.go:457`), not platform-event subscription. Custom frontends that want "graph node started/completed" events subscribe via WebSocket and filter by `event.type`. The type constants live in `backend/internal/api/events/types.go:55-110`; mirror them in TS via `examples/custom-ui-nextjs/lib/events.ts`.
- **Token handoff on WebSocket:** browsers cannot set `Authorization` headers on a WS upgrade. The gateway's `wsAuth` middleware reads `?token=<jwt>` from the query string. The example uses exactly this pattern.

## 7. Database migrations

- File naming: `NNN_<phase>_<topic>.up.sql` + matching `.down.sql`. The numbering is strict — the next migration is `043_*`.
- CI runs `qorven migrate up` from the binary (not raw `psql`). If your migration depends on the order of prior migrations, do not rely on psql idioms that assume any schema state beyond what the previous numbered migration established.
- **RLS policies** for any new tenant-bound table MUST follow the Phase 4 pattern:

  ```sql
  ALTER TABLE my_new_table ENABLE ROW LEVEL SECURITY;
  ALTER TABLE my_new_table FORCE ROW LEVEL SECURITY;
  CREATE POLICY rls_tenant_isolation ON my_new_table
      USING (app_rls_bypass() OR tenant_id = app_current_tenant())
      WITH CHECK (app_rls_bypass() OR tenant_id = app_current_tenant());
  ```

  The `app_rls_bypass()` and `app_current_tenant()` helpers are defined in migration 040. They are the non-negotiable contract: leave out the bypass clause and single-tenant installs break; leave out `FORCE` and a superuser silently bypasses; use a different GUC name and `WithTenantTx` stops scoping correctly.

## 8. Testing rules

- **Single-tenant behavior must stay byte-for-byte identical.** The Phase 3 ruling made this a perpetual constraint. Any test that passes in the current main branch must keep passing when you add a feature.
- **CI gates:** `make verify` (vet + full test suite) + `make build-release-arch` for linux/amd64 and linux/arm64. Both must be green before merge.
- **Destructive manifest test** is the security tripwire — do not rename `TestDestructiveManifest_*` tests in `backend/internal/gateway/destructive_manifest_test.go`.
- **RLS tests** live in `backend/internal/store/rls_test.go` and REQUIRE a restricted role (`qorven_app`). CI provisions it; locally, create it by hand (see §1.3). A missing role `FAIL`s the test — it does not skip.
- **Stress test** (`backend/internal/gateway/multitenant_stress_test.go`) runs 50 tenants × 8 concurrent requests. If your change slows per-request latency meaningfully, the 82-second test budget will also tighten — keep a commit-to-commit eye on its wall-clock.

## 9. The BYOM (Bring Your Own Model) mandate

Users run Qorven on everything from hosted Opus inference down to a CPU-only Llama 8B. Timeouts tuned for the former misclassify the latter as "stuck." Every timeout that matters lives in `backend/internal/byom/timeouts.go` and is overridable via env var:

| Env var                     | Default | What it controls                        |
| --------------------------- | ------- | --------------------------------------- |
| `QORVEN_SUBMIT_TIMEOUT`     | 10m     | Hard ceiling on one agent turn          |
| `QORVEN_PERMISSION_TIMEOUT` | 2m      | Permission-gate auto-deny window        |
| `QORVEN_STALE_PLAN_AFTER`   | 15m     | Sweeper's "abandoned plan" threshold    |
| `QORVEN_SWEEPER_TICK`          | 30s     | Sweeper scan cadence                              |
| `QORVEN_GRAPH_MAX_HOPS`        | 256     | Cycle-detection ceiling per graph run             |
| `QORVEN_TENANT_MAX_CONCURRENT` | 4       | Per-tenant concurrent request cap (0 disables). Phase 7. |
| `QORVEN_TENANT_RATE_LIMIT`     | 30      | Per-tenant req/sec, burst=rate. 0 disables. Phase 7. |

Phase 7 tenant quotas apply to `/v1/commands` and `/v1/wasm-plugins` mutation paths. A tenant at its cap gets `429 {code: "tenant_quota", retry_after: <seconds>}`. Reads are un-quota'd by design.

When you add a new timeout to the orchestrator path, define it in `byom/timeouts.go` with a real env-var override, not as a hardcoded `const … time.Minute`.

## 10. "I want to…" quick-reference

| Task                                                 | Read this first                                                                        |
| ---------------------------------------------------- | -------------------------------------------------------------------------------------- |
| Add a `/v1/*` endpoint                               | §2, §3                                                                                 |
| Add a built-in tool                                  | §1.2, §4                                                                               |
| Add a new tenant-bound table                         | §1.1, §7                                                                               |
| Subscribe to graph events from a custom frontend     | §6; type names in `backend/internal/api/events/types.go:55`                            |
| Boot the gateway in multi-tenant locally             | §1.3 — then set `deployment_mode=multi_tenant` in the `deployment_config` table        |
| Change a production timeout                          | §9                                                                                     |
| Understand why my DB query returns empty             | §1.1 (you bypassed `WithTenantTx`) or §1.3 (your role is NOSUPERUSER but no scope set) |
| Add a legacy-table RLS policy to the restricted set  | Look at migration 042 (`backend/migrations/042_phase5_legacy_rls_unify.up.sql`)         |

## 11. Known Limitations (RC1)

Documenting the system's current boundaries explicitly so operators + AI agents have calibrated expectations. Every item here is a deliberate trade-off, not a bug. Violating them doesn't break security; it just causes confusion or capacity surprises.

### 11.1 Single-binary deployment

- Qorven is designed for one gateway process backed by one Postgres. Horizontal scale (multiple gateway replicas) is **not supported** today.
- The per-tenant sweeper manager (`backend/internal/orchestrator/sweeper_manager.go`) assumes one owner per tenant — running two gateway binaries against the same DB could double-execute a plan after a restart because both sweepers would see the same unconsumed wakeup. Set `QORVEN_REPLICAS=1` to make the gateway's DraftStore guard enforce this intent.
- If you need horizontal scale, you're on the Phase 9+ roadmap. The architectural fix is: Redis for distributed locks + a real leader election for the sweeper. Don't try to bolt it on with just a load balancer.

### 11.2 Process-local rate limits

- `gateway.TenantQuota` (Phase 7) is in-memory per-process. When the gateway restarts, the bucket state resets to full — a tenant that was being throttled gets a clean slate. Acceptable on a single-binary deployment where restarts are rare; a bug under horizontal scale where each replica has its own counter.
- The effective limit for N replicas is `N × DefaultTenantRateLimitPerSecond`. If you care about global caps, that's a Phase 9 problem (Redis-backed limiter).
- Denials are observable via the `tenant_quota_denials_total{tenant,reason}` Prometheus counter on `/metrics`. `reason ∈ "rate" | "concurrency"`.

### 11.3 WebSocket pings are application-level JSON, not RFC 6455 control frames

- The DOM WebSocket API cannot send RFC 6455 ping control frames; it exposes only `send()` for application messages. Qorven's Next.js reference app (`examples/custom-ui-nextjs/hooks/useQorvenSocket.ts`) sends a `{"type":"ping"}` JSON message every 20 seconds as a liveness nudge.
- The server ignores this message. The existence of ANY server traffic (real events or otherwise) resets the client's staleness clock.
- If your custom transport needs true control-frame pings (e.g. Go server talking to a Python client), use a WebSocket library that exposes them directly — you're outside the browser-JS constraint that shaped this design.

### 11.4 `permission_requests.tenant_id` is TEXT, not UUID

- Legacy schema from pre-Phase-4. The column accepts UUID strings and the Phase 4 RLS policy casts it at query time. Rows with the historical literal `'default'` work under single-tenant mode (bypass=on) but fail RLS under multi-tenant.
- The gate's INSERT was updated in Phase 7 to write the caller's tenant when `RequestInput.TenantID` is set. Legacy integration tests still write `'default'` because they bypass `WrapLazy`; they clean up via `session_id`-scoped `t.Cleanup()`.
- Unifying the column type to UUID is in the Phase 9 cleanup backlog. Until then, don't seed `permission_requests` rows by hand with literal strings other than valid UUIDs if you expect them to survive RLS.

### 11.5 TinyGo support is opt-in, not default

- `qorven agent init` defaults to `--runtime go` (standard Go wasip1, ~3.2 MiB artifacts). `--runtime tinygo` produces ~50 KiB artifacts but requires `tinygo >= 0.34` on the operator's PATH.
- The gateway's Wasm host (`internal/plugins/wasm`) doesn't care which toolchain produced the binary — it sees `.wasm` bytes. The tradeoff is purely at build time for the plugin author.
- TinyGo's `reflect` is a subset of Go's. Plugins that reach for `reflect.Value.Call` may fail to compile under TinyGo. Standard `encoding/json` struct marshal/unmarshal works.

### 11.6 Wasm plugin runtime uses the interpreter engine

- `wazero.NewRuntimeConfigInterpreter()` — not the optimizing compiler. The interpreter is 3–5× slower per instruction but sidesteps Go-version-specific codegen quirks in `wazevo`. Plugin invocations are capped at 2 seconds (`QORVEN_SUBMIT_TIMEOUT`), so the interpreter's overhead is absorbed by the bound.
- For compute-heavy plugins (image processing, crypto), consider offloading to an HTTP-backed integration that the plugin invokes via the orchestrator's existing tool surface. Wasm plugins should be lightweight JSON transforms.

### 11.7 Shadow semantics on ExtraTools

- Tenant-uploaded Wasm plugins can shadow built-in tool names EXCEPT for the reserved list in `backend/internal/tools/reserved_manifest.go` (Phase 6). Reserved names are every destructive built-in + core orchestration primitives.
- A non-reserved shadow IS allowed: a tenant can upload a plugin named `web_search` and override the default implementation for their tenant's plan runs. This is a feature for legitimate customization; the reserved list is the security boundary.

### 11.8 No graceful shutdown / global context tree

- The gateway's long-running goroutines (sweeper manager, tenant-quota idle sweeper, Wasm host) use `context.Background()` so they live until process exit. Sending SIGTERM kills them abruptly.
- Consequence for operators: a rolling restart can interrupt an in-flight plan at any node boundary. The recovery path (Phase 8 chaos test in `backend/internal/orchestrator/chaos_mid_plan_test.go`) proves the next gateway instance resumes correctly — so this is a cosmetic rough edge, not a correctness issue.

### 11.9 RLS tests require a NOSUPERUSER role

- `internal/store/rls_test.go` connects as `qorven_app` (NOSUPERUSER, NOBYPASSRLS). CI provisions this role; local dev boxes may not. `rlsTestDSN()` derives the DSN from `QORVEN_APP_TEST_DSN` or substitutes `qorven_app:qorven_app` into `QORVEN_TEST_DSN`.
- Running RLS tests under the default `qorven` role (often superuser on dev machines) silently passes for the wrong reason — superusers bypass every RLS policy regardless of `FORCE`. The tests defend against this with a role-flag probe that fails loud if `rolsuper` or `rolbypassrls` is set.

### 11.10 The `permissions` flow assumes a human (or test) approver

- Built-in destructive tools go through `permissions.WrapLazy` → `Gate.Request`, which blocks the caller until `Gate.Reply` arrives or the timeout fires.
- Production assumes a human clicks approve in the UI. Automated approval (webhook, external workflow) is a matter of the operator POSTing to `/v1/permissions/{id}/reply` from whatever governance system they have. The gate itself has no auto-approve logic.
- Tests bypass this via the pattern in `internal/orchestrator/e2e_wasm_plugin_test.go`'s `startAutoApprover` helper — poll the DB for pending requests matching an allowlist of tool names, reply Allow. If you're writing an integration test that exercises a destructive tool, copy that pattern exactly — it has the race-safety properties the suite relies on.
