# Resilience

Qorven recovers automatically from port contention, dropped connections,
and process crashes. This doc describes the moving parts so you can
verify a production install, customise the defaults, or diagnose a
recovery that didn't recover.

## Table of contents

- [What happens when a port is busy](#what-happens-when-a-port-is-busy)
- [Health probes (`/livez`, `/readyz`, `/__qorven_runtime`)](#health-probes)
- [Graceful shutdown](#graceful-shutdown)
- [WebSocket heartbeat](#websocket-heartbeat)
- [Frontend reconnect](#frontend-reconnect)
- [Supervisors](#supervisors)
- [Troubleshooting](#troubleshooting)

---

## What happens when a port is busy

The gateway tries the configured API port first. If something is
already listening on it, it walks up by 1 — up to 10 attempts — before
giving up. The chosen port is recorded to `~/.qorven/runtime.json`
(or `$HOME/.qorven/` for the service user) and served at
`/__qorven_runtime` so the web client can find it.

```
$ qorven start                  # configured port: 4200
WARN port bind: preferred busy, using fallback
  preferred=0.0.0.0:4200 actual=0.0.0.0:4201 offset=1
INFO api listener starting addr=0.0.0.0:4201
INFO runtime.json written path=/home/alice/.qorven/runtime.json api_addr=0.0.0.0:4201
```

Implementation: `backend/internal/gateway/resilience.go:bindListener`.
Test coverage: `resilience_test.go:TestBindListener_*`.

### runtime.json

Always rewritten on boot. Format:

```json
{
  "api_addr": "0.0.0.0:4201",
  "api_port": 4201,
  "web_addr": "127.0.0.1:3000",
  "web_port": 3000,
  "pid": 1817790,
  "started_at": "2026-04-21T07:49:45Z",
  "version": "dev"
}
```

File permissions are `0o600`. If `$HOME` is unset (e.g. `sudo
systemctl start qorven` without `User=`), falls back to
`$TMPDIR/qorven-runtime.json`.

### Frontend discovery

`web/lib/websocket.ts:discoverApiUrl` fetches `/__qorven_runtime` at
boot, before the first WebSocket connect. If the call succeeds, the WS
URL uses the returned port. If it fails (backend still starting,
network unreachable) it falls back to `NEXT_PUBLIC_API_URL`.

---

## Health probes

Three endpoints, each with a different job:

| Endpoint | Purpose | Use for | DB check |
|---|---|---|---|
| `/health` | Legacy, kept for compat | — | No |
| `/livez` | "Process is alive" | k8s liveness, Docker HEALTHCHECK | No |
| `/readyz` | "Ready to serve traffic" | k8s readiness, load balancer | Yes |
| `/__qorven_runtime` | Port discovery for the web client | Frontend boot | No |

**Why `/livez` and `/readyz` are separate.** A DB blip should NOT
restart the gateway process — that would drop every active WebSocket
session. It should just pull us out of the load balancer until the
DB comes back. That's what the split lets you do:

```yaml
# k8s example
livenessProbe:
  httpGet: { path: /livez, port: 4200 }
  periodSeconds: 10
  failureThreshold: 3
readinessProbe:
  httpGet: { path: /readyz, port: 4200 }
  periodSeconds: 5
  failureThreshold: 2
```

`/readyz` returns `200` when the DB is configured and reachable, or
when no DB is configured at all (fresh installs before wizard). It
returns `503` only when a configured DB is actually unavailable.

---

## Graceful shutdown

`SIGTERM` and `SIGINT` both trigger a 10-second drain:

1. Server stops accepting new connections
2. In-flight HTTP requests and streaming responses finish
3. After 10s, any remaining connections are force-closed
4. Second signal during drain = hard exit (code 130)

Implementation: `installShutdownHandler` in `gateway.go`. Systemd unit
and Docker compose both give the process the full window:

- `backend/scripts/qorven.service`: `TimeoutStopSec=15s`
- `docker-compose.yml`: uses the default `stop_grace_period` of 10s

If you raise/lower `TimeoutStopSec`, keep it ≥ the internal 10s — a
shorter timeout means systemd will `SIGKILL` before the drain finishes,
which defeats the point.

---

## WebSocket heartbeat

Three WS endpoints, each with server-initiated heartbeat:

| Endpoint | Library | Ping interval | Timeout |
|---|---|---|---|
| `/ws/realtime` | `nhooyr.io/websocket` | 20s | 10s |
| `/ws` (RPC) | `nhooyr.io/websocket` | 20s | 10s |
| `/ws/voice` | `gorilla/websocket` | 20s | 40s (read deadline) |
| `/ws/voice/realtime` | `nhooyr.io/websocket` | (upstream-driven) | — |

The 20s cadence detects dead connections long before the OS-level TCP
keepalive (2 hours default on Linux). When a ping fails — client
crashed, NAT flow dropped, laptop closed — the handler cancels its
context, which unwinds the reader, writer, and any piping goroutines
together.

---

## Frontend reconnect

Three layers of recovery:

1. **Exponential backoff with jitter** — `lib/resilience.ts:nextBackoffMs`.
   Formula: `min(1000 * 2^attempt, 30000) + random(0..1000)`. Capped at
   30s because anything longer feels dead. Jitter prevents every tab
   from reconnecting in lockstep after a server restart.

2. **`window.online` listener** — when the browser regains the network
   (wifi flip, laptop wake), we force-reconnect immediately instead of
   waiting for the next backoff tick.

3. **Reconnect banner** — `components/reconnect-banner.tsx`. Amber bar
   appears after the connection has been down for >5s ("Reconnecting…
   (attempt N)"), flashes green on recovery, then hides itself.

4. **REST retry** — `lib/api.ts:fetchWithRetry`. Retries `GET` and
   `HEAD` up to 3 times with 300ms / ~700ms / ~1.5s delays on
   `ECONNREFUSED`/`fetch failed`. `POST`/`PATCH`/`DELETE` stay
   one-shot — replaying a write could double-create or double-apply.

---

## Supervisors

Pick one based on where you run:

### Docker Compose (recommended for OSS users)

```bash
docker compose up -d
```

Both services have `restart: unless-stopped` and healthchecks. A
crashed backend is restarted automatically; the web container waits
for the backend's healthcheck before starting.

### Systemd (bare-metal / VM)

```bash
sudo useradd --system --home /var/lib/qorven --shell /usr/sbin/nologin qorven
sudo install -o qorven -g qorven -d /var/lib/qorven /etc/qorven
sudo cp backend/scripts/qorven.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now qorven
```

The unit has `Restart=on-failure` and `StartLimitBurst=10` in a 5-minute
window — a crash loop eventually trips into a permanent `failed`
state instead of thrashing forever.

### Development: `air` hot-reload

```bash
# one-time
go install github.com/air-verse/air@latest

# every session
cd backend && make dev-watch
```

`backend/.air.toml` watches `cmd/` and `internal/`, rebuilds on change,
and restarts the binary on crash. Combined with the port probe, a
zombie listener from the previous run won't block the new process.

### Preflight: `scripts/dev-up.sh`

```bash
./scripts/dev-up.sh               # interactive port check
./scripts/dev-up.sh --kill        # auto-kill port occupants
./scripts/dev-up.sh --check-only  # report and exit, no prompts
```

Identifies the process holding your ports, offers to kill it, checks
for `go`/`node`/`pnpm`/`docker`, and ensures `~/.qorven/` exists and
is writable.

---

## Troubleshooting

### Frontend stuck on "Reconnecting…"

1. Confirm the backend is actually listening:
   ```bash
   curl -sf http://localhost:4200/livez && echo OK
   ```
2. Check which port it chose:
   ```bash
   cat ~/.qorven/runtime.json
   ```
3. Check that `NEXT_PUBLIC_API_URL` matches the bound port (or remove
   it to rely on discovery).

### Backend restarts every few minutes

- Docker Compose: `docker compose logs backend | grep -E 'killed|signal'`
  — a Linux OOM kill looks like `Killed` with exit code 137.
- `/livez` must stay fast. If it's slow (>3s), the Docker healthcheck
  times out and compose marks the container unhealthy → restart.
  `/livez` should never touch the DB — if you changed it, revert.

### Port probe exhausted

```
ERROR could not bind on 127.0.0.1:4200 or next 10 ports
```

Something is squatting on 10+ consecutive ports. Use `scripts/dev-up.sh
--kill` or manually identify the holders:

```bash
for p in $(seq 4200 4210); do
  ss -tlnp "sport = :$p" 2>/dev/null
done
```

### Systemd unit stuck in `failed`

```bash
systemctl status qorven        # check last restart + exit code
journalctl -u qorven -n 50     # see the crash
systemctl reset-failed qorven  # clear the failure count
```
