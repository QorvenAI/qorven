# Contributing to Qorven

Thanks for wanting to contribute. Bug fixes, new channel integrations, doc improvements, and "good first issues" are all welcome. If anything here is unclear, open a GitHub Discussion — "the docs confused me" is a valid bug report.

---

## Good first issues

Look for the [`good first issue`](https://github.com/QorvenAI/qorven/issues?q=is%3Aopen+label%3A%22good+first+issue%22) label. Good starting points:

- Adding a new OpenAI-compatible AI provider (usually a catalog entry + wizard option — ~20 lines)
- Writing or improving a godoc comment on an exported package
- Adding a missing CLI flag or improving an error message
- Writing a setup guide for a specific channel
- Fixing a typo or incorrect code comment

Small, focused PRs merge fastest.

---

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Go | **1.26+** | Check `backend/go.mod` for the exact minimum |
| PostgreSQL | **15+** | With `pgvector` extension installed |
| Node.js | **20+** | For the Next.js frontend |
| pnpm | **9+** | `npm i -g pnpm` |
| `air` | latest | Hot reload: `go install github.com/air-verse/air@latest` |

---

## Local setup

```bash
git clone https://github.com/QorvenAI/qorven.git
cd qorven
cp .env.example .env           # set QORVEN_DB_PASSWORD
docker compose up -d           # starts Postgres + pgvector
```

**Backend** (hot reload on save):
```bash
cd backend
go mod download
export QORVEN_POSTGRES_DSN="postgres://qorven:yourpass@localhost:5432/qorven?sslmode=disable"
make dev-watch
```

On first run, apply migrations:
```bash
./dist/qorven migrate up
```

**Frontend**:
```bash
cd web
pnpm install
pnpm dev                       # :3000 — proxies API calls to :4200
```

The web UI is at `http://localhost:3000`. The backend API is at `http://localhost:4200`.

---

## Migration rules

**Read this before touching any `.sql` file.**

- `001_schema.up.sql` is the complete baseline schema for fresh installs. It runs exactly once (at version 1). **Never append SQL to this file** — the migration runner skips it on existing installs.
- New tables or columns go in a new numbered file: `002_your_change.up.sql` + `002_your_change.down.sql`
- Use zero-padded three-digit names: `002_add_foo.up.sql`, `003_add_bar.up.sql`
- After adding a file, apply it to your dev DB manually and mark it applied:
  ```bash
  psql -d qorven -f backend/migrations/002_your_change.up.sql
  psql -d qorven -c "INSERT INTO schema_migrations (version, dirty) VALUES (2, false) ON CONFLICT DO NOTHING;"
  ```
- Why this matters: new installs run migrations in version order from the embedded binary. A gap or wrong version causes "column does not exist" errors in production.

---

## Code style

### Go

- `gofmt -s` before every commit — CI enforces it.
- Exported symbols need a one-sentence godoc comment explaining **why** it exists, not just what it does.
- Wrap errors with context: `fmt.Errorf("load config: %w", err)` — never bare `return err` from unexported helpers.
- Avoid `interface{}` / `any` at public boundaries; use concrete types or named interfaces.
- Tests live next to code as `_test.go` files. Integration tests that need Postgres should skip cleanly when `QORVEN_TEST_DSN` is unset.
- Handler patterns — copy these exactly:
  - DB nil guard: `if gw.db == nil { writeJSON(w, 503, ...) }` at the top of any handler that uses the pool
  - Error responses: use `sanitizeError(err)` not `err.Error()` to avoid leaking internal details
  - New `/v1/*` routes: register in `backend/internal/gateway/routes_v1.go`, not `gateway_routes.go`

### TypeScript / React

- ESLint + Prettier — `pnpm lint` must pass before pushing.
- Strict mode is on; avoid `any`.
- Components are functional and hooks-driven.
- Tailwind-first for styling using the `qr-*` design token classes defined in `web/css/styles.css`.

### Commits

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(channels): add DingTalk channel integration
fix(auth): handle NULL email on setup-wizard users
refactor(gateway): extract session context helper
docs(contributing): fix Go version requirement
test(providers): add Bedrock unit tests
chore(ci): pin golangci-lint to v1.58
```

The scope is the affected package or area (`wizard`, `auth`, `gateway`, `providers`, `web/chat`, …). Commit messages explain **why**, not just what — the diff shows what.

---

## PR flow

1. **Open an issue first** for non-trivial changes — lets maintainers weigh in before you write code. Skip this for obvious bug fixes or typos.
2. Fork → create a feature branch off `main` (e.g., `feat/telegram-groups`, `fix/approval-gate-poll`).
3. Write tests. `go test -short ./...` is the minimum bar.
4. Run `gofmt -s -w .` and `pnpm lint` before pushing.
5. Open a PR against `main` and link the related issue.
6. A maintainer will review — typically within 48 h for small PRs.

---

## Testing

```bash
# Fast — no DB required
cd backend && go test -short ./...

# Full — needs Postgres
export QORVEN_TEST_DSN="postgres://qorven:yourpass@localhost:5432/qorven_test?sslmode=disable"
cd backend && go test ./...
```

Integration tests use `internal/testsupport.DSNOrSkip(t)` — they skip cleanly when no DB is available.

---

## Architecture overview

```
backend/
├── cmd/               # CLI entrypoints (start, install, chat, migrate, …)
├── internal/
│   ├── agent/         # agent loop, memory flush, delegation, soul bundles
│   ├── apps/          # app platform: install/enable/disable Go connectors
│   ├── auth/          # bcrypt + JWT + API keys
│   ├── autonomy/      # briefings, cron scheduler, goal tracker
│   ├── channels/      # Telegram / Slack / Discord / Email / WhatsApp / …
│   ├── gateway/       # HTTP handlers, WebSocket, middleware, routes
│   ├── memory/        # pgvector store with BM25 search
│   ├── providers/     # Anthropic / OpenAI / Gemini / Groq / … drivers
│   ├── research/      # multi-step web research engine
│   ├── testsupport/   # shared test helpers (never imported in production)
│   └── tools/         # 120+ built-in tools (file, shell, web, email, …)
├── migrations/        # SQL migration files — 001_schema.up.sql is the baseline
└── main.go
```

```
web/
├── app/
│   ├── (app)/         # authenticated routes (chat, code, channels, settings, …)
│   ├── login/         # login + fresh-install redirect
│   └── setup/         # 5-step browser setup wizard
├── components/        # shared UI components
├── css/               # Tailwind + Qorven design tokens (qr-*)
├── lib/               # API client, utilities
└── store/             # Zustand global state
```

---

## Recognition

Every merged contribution is credited in the release changelog. We follow the [All Contributors](https://allcontributors.org/) spec — code, docs, design, bug reports, and community support all count.

---

## Sponsoring Qorven

If Qorven saves you time or money, consider sponsoring:

- **[GitHub Sponsors](https://github.com/sponsors/QorvenAI)** — recurring or one-time
- **[Ko-fi](https://ko-fi.com/qorvenai)** — one-time via PayPal
- **[qorven.ai/sponsor](https://qorven.ai/sponsor)** — Razorpay for Indian supporters

---

## Security

Do **not** open a public GitHub issue for security vulnerabilities. See [`SECURITY.md`](./SECURITY.md) for responsible disclosure.

---

## License

By contributing, you agree your contributions will be licensed under [FSL-1.1-ALv2](./LICENSE).
