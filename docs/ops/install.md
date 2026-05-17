# Installing Qorven

Three supported paths, same binary underneath.

---

## 1. One-line install (Linux / macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/qorvenai/qorven/main/install.sh | sudo sh
```

What the installer does:

1. Detects your OS + arch + package manager (apt / dnf / pacman / brew / apk).
2. Installs PostgreSQL + pgvector if not already present.
3. Creates the `qorven` system user and `/var/lib/qorven`, `/etc/qorven`, `/var/log/qorven`.
4. Downloads the matching release binary from GitHub.
5. Generates `/etc/qorven/config.toml` with a fresh encryption key + auth token (0640, owned by `qorven`).
6. Runs `qorven tls generate` + `qorven tls install-ca` so browsers get a green lock on first load.
7. Installs a hardened systemd unit and starts the service.
8. Prints the URL to open.

Audit before you trust the pipe — `| sh -s -- --dry-run` prints every command without running it. Uninstall with `| sudo sh -s -- --uninstall`.

**You will only be asked for:** the web UI port (default `443`). Everything else has a sensible default; change it later in `/etc/qorven/config.toml`.

---

## 2. Docker / docker-compose

```bash
cp .env.example .env       # set QORVEN_DB_PASSWORD to something non-empty
docker compose up -d
open https://localhost     # or whichever QORVEN_WEB_PORT you set
```

One container runs both the API and the web UI (the Next.js build is embedded via `go:embed`). Only Postgres is a separate service. See `docker-compose.yml` for the full env table.

---

## 3. Build from source

```bash
git clone https://github.com/qorvenai/qorven
cd qorven
make build                 # builds dist/qorven with embedded web UI
./dist/qorven start        # uses ~/.qorven/config.toml
```

Prereqs: Go 1.22+, Node.js 22+, pnpm, Postgres 14+ with pgvector.

Other useful Make targets: `make dev` (backend + next.js dev server), `make release` (cross-compile all platforms), `make verify` (typecheck + tests), `make clean`.

---

## Windows

Native install is advanced — we don't currently ship an installer. Two supported paths:

- **WSL2 (recommended):** `wsl --install`, then follow the Linux instructions inside the distro.
- **Docker:** `docker compose up`.
- **Native binary:** download `qorven-windows-amd64.exe` from the releases page. You'll need to install PostgreSQL + pgvector yourself and run the binary under a Windows Service (NSSM or similar). No systemd on Windows.

---

## macOS (manual)

If you skip the install script:

```bash
brew install postgresql@16 pgvector
brew services start postgresql@16
curl -fsSL https://github.com/qorvenai/qorven/releases/latest/download/qorven-darwin-arm64 \
  -o /usr/local/bin/qorven
chmod +x /usr/local/bin/qorven
qorven setup        # interactive wizard
qorven start
```

Use a `launchd` plist under `~/Library/LaunchAgents/` if you want it to run at login.

---

## After install

- **Browser doesn't trust the cert:** run `sudo qorven tls install-ca` (see [tls.md](tls.md)).
- **Change web UI without rebuilding the binary:** see [web-customization.md](web-customization.md).
- **Update to a newer version:** `sudo qorven update`.
- **Rotate HTTPS cert after IP change:** `sudo qorven tls regenerate && sudo qorven tls install-ca`.
- **Back up:** the database + `/etc/qorven/config.toml` (the encryption key there is the only copy — if you lose it you lose every secret in the vault and every provider key).
