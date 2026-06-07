//go:build linux

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package installer

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func platformSteps() []installStep {
	return []installStep{
		{label: "Detect system"},
		{label: "Update package index"},
		{label: "Install system packages"},
		{label: "Install PostgreSQL"},
		{label: "Install Docker"},
		{label: "Create OS user  qorven"},
		{label: "Create data directories"},
		{label: "Setup database"},
		{label: "Install binary to /opt/qorven/bin"},
		{label: "Install & start systemd service"},
		{label: "Configure nginx reverse proxy"},
		{label: "Install Tailscale"},
	}
}

func platformConfigDir() string { return "/etc/qorven" }

func platformBinPath() string { return "/opt/qorven/bin/qorven" }

func platformRestartService(configPath string) {
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "restart", "qorven").Run()
}

func platformMigrate(configPath, dsn string) error {
	cmd := exec.Command("sudo", "-u", "qorven",
		"env",
		"QORVEN_CONFIG="+configPath,
		"QORVEN_POSTGRES_DSN="+dsn,
		"/opt/qorven/bin/qorven", "migrate", "up",
	)
	return cmd.Run()
}

// probeSocketDSN tries Unix socket first (fastest, no password), falls back to TCP.
func probeSocketDSN() string {
	port := probePGPort()
	socketDSN := "postgres:///qorven?host=/var/run/postgresql&user=qorven&sslmode=disable"
	if port != "" && port != "5432" {
		socketDSN += "&port=" + port
	}
	// Verify the socket actually exists before committing to it.
	entries, err := os.ReadDir("/var/run/postgresql")
	if err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".s.PGSQL.") {
				return socketDSN
			}
		}
	}
	// Fall back to TCP loopback (works when socket is missing or on custom path).
	tcpPort := port
	if tcpPort == "" {
		tcpPort = "5432"
	}
	return "postgres://qorven@127.0.0.1:" + tcpPort + "/qorven?sslmode=disable"
}

func probePGPort() string {
	entries, err := os.ReadDir("/var/run/postgresql")
	if err != nil {
		return "5432"
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".s.PGSQL.") {
			p := strings.TrimPrefix(name, ".s.PGSQL.")
			if p != "" {
				return p
			}
		}
	}
	return "5432"
}

// startPostgresService handles the Debian/Ubuntu multi-name service problem.
// The generic "postgresql" unit may not exist; the real one is often
// "postgresql@16-main" or "postgresql@14-main". We try multiple names.
func startPostgresService() error {
	// Try generic name first (works on some distros)
	if runQuiet("systemctl", "start", "postgresql") == nil {
		return nil
	}
	// Try all versioned cluster units (Debian/Ubuntu style)
	out, err := exec.Command("systemctl", "list-units", "--type=service", "--all",
		"--no-legend", "--no-pager", "postgresql*").Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			svcName := strings.TrimSuffix(fields[0], ".service")
			if strings.HasPrefix(svcName, "postgresql@") || strings.HasPrefix(svcName, "postgresql") {
				if runQuiet("systemctl", "enable", svcName) == nil {
					if runQuiet("systemctl", "start", svcName) == nil {
						return nil
					}
				}
			}
		}
	}
	// pg_ctlcluster fallback (Debian-only utility)
	if commandExists("pg_ctlcluster") {
		// pg_lsclusters -h: version  cluster  port  status
		lsOut, lsErr := exec.Command("pg_lsclusters", "-h").Output()
		if lsErr == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(lsOut)), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					runQuiet("pg_ctlcluster", fields[0], fields[1], "start")
				}
			}
		}
	}
	// Final check
	if _, e := runSilent("pg_isready", "-q"); e == nil {
		return nil
	}
	return fmt.Errorf("could not start PostgreSQL — try: sudo systemctl status postgresql")
}

// pgDiagnostic returns journal output for postgresql units (for error messages).
func pgDiagnostic() string {
	out, err := exec.Command("journalctl", "-u", "postgresql*", "-n", "20", "--no-pager").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		out2, _ := exec.Command("journalctl", "-xe", "--no-pager", "-n", "15").Output()
		return strings.TrimSpace(string(out2))
	}
	return strings.TrimSpace(string(out))
}

func installPgvector(pgMaj string) {
	if pgMaj == "" {
		return
	}

	// Detect package manager
	hasDnf := commandExists("dnf")
	hasYum := commandExists("yum")

	if commandExists("apt-get") {
		// Debian/Ubuntu path
		versioned := "postgresql-" + pgMaj + "-pgvector"
		if runQuiet("apt-get", "install", "-y", "-qq", versioned) == nil {
			return
		}
		// Add PGDG repo and retry
		distro, _ := runSilent("bash", "-c", `source /etc/os-release 2>/dev/null && echo "${ID}"`)
		distro = strings.TrimSpace(distro)
		codename, _ := runSilent("lsb_release", "-cs")
		codename = strings.TrimSpace(codename)
		if (distro == "ubuntu" || distro == "debian") && codename != "" {
			keyPath := "/usr/share/keyrings/postgresql.asc"
			runSilent("curl", "-fsSL",
				"https://www.postgresql.org/media/keys/accc4cf8.asc",
				"-o", keyPath)
			repo := fmt.Sprintf(
				"deb [signed-by=%s] https://apt.postgresql.org/pub/repos/apt %s-pgdg main",
				keyPath, codename)
			os.WriteFile("/etc/apt/sources.list.d/pgdg.list", []byte(repo+"\n"), 0644)
			runQuiet("apt-get", "update", "-qq")
			if runQuiet("apt-get", "install", "-y", "-qq", versioned) == nil {
				return
			}
		}
		// Fallback: unversioned name (Ubuntu universe)
		runQuiet("apt-get", "install", "-y", "-qq", "postgresql-pgvector")
		return
	}

	if hasDnf || hasYum {
		// RHEL/Fedora/Amazon Linux path — use PGDG RPM repo
		pm := "dnf"
		if !hasDnf {
			pm = "yum"
		}
		// Add PGDG repo if not present
		if _, e := runSilent("rpm", "-q", "pgdg-redhat-repo"); e != nil {
			repoUrl := fmt.Sprintf("https://download.postgresql.org/pub/repos/yum/reporpms/EL-$(rpm -E %%rhel)-x86_64/pgdg-redhat-repo-latest.noarch.rpm")
			runQuiet(pm, "install", "-y", repoUrl)
		}
		pkgName := fmt.Sprintf("pgvector_%s", pgMaj)
		if runQuiet(pm, "install", "-y", pkgName) == nil {
			return
		}
		// Try alternate naming
		runQuiet(pm, "install", "-y", "pgvector")
	}
}

// pgvectorEnabled checks whether the vector extension is actually available in PostgreSQL.
func pgvectorEnabled() bool {
	out, err := runSilent("sudo", "-u", "postgres", "psql", "-d", "qorven", "-tAc",
		"SELECT 1 FROM pg_extension WHERE extname='vector'")
	if err != nil {
		// Try TCP fallback
		out, err = runSilent("psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "qorven", "-tAc",
			"SELECT 1 FROM pg_extension WHERE extname='vector'")
	}
	return err == nil && strings.TrimSpace(out) == "1"
}

func platformRequirementsText() string {
	req := func(icon, label, detail string) string {
		return icon + "  " + fgSt.Render(label) + "\n" +
			"   " + mutedSt.Render(detail)
	}
	return req("🐧", "Ubuntu 20.04+ / Debian 11+ / RHEL 8+", "systemd-based Linux") + "\n" +
		req("🔑", "root or sudo access", "to install packages & services") + "\n" +
		req("💾", "2 GB RAM  ·  10 GB disk", "minimum recommended") + "\n" +
		req("🌐", "Internet access", "to pull packages on first install")
}

func platformServiceCommands() string {
	return mutedSt.Render("  systemctl status qorven") + "\n" +
		mutedSt.Render("  journalctl -u qorven -f") + "\n" +
		mutedSt.Render("  sudo qorven migrate up")
}

func platformErrorHints() (common, logs string) {
	common = dimSt.Render("  No internet — check curl / DNS") + "\n" +
		dimSt.Render("  Port already in use") + "\n" +
		dimSt.Render("  postgresql service not starting") + "\n" +
		dimSt.Render("  Disk full — needs 10 GB free") + "\n" +
		dimSt.Render("  Not running as root (use sudo)")
	logs = mutedSt.Render("  journalctl -u qorven -f") + "\n" +
		mutedSt.Render("  journalctl -u postgresql* -n 30")
	return
}

func executeStep(idx int, cfg Config) (detail string, warn bool, err error) {
	switch idx {
	case 0:
		out, _ := runSilent("bash", "-c", `source /etc/os-release 2>/dev/null && echo "$PRETTY_NAME"`)
		arch, _ := runSilent("uname", "-m")
		// Warn if neither apt nor dnf/yum
		if !commandExists("apt-get") && !commandExists("dnf") && !commandExists("yum") {
			return strings.TrimSpace(out) + "  " + strings.TrimSpace(arch) +
				" — WARNING: unknown package manager, some steps may require manual intervention", true, nil
		}
		return strings.TrimSpace(out) + "  " + strings.TrimSpace(arch), false, nil

	case 1:
		if commandExists("apt-get") {
			if err = runQuiet("apt-get", "update", "-qq"); err != nil {
				return "skipped (apt-get update failed — continuing)", true, nil
			}
		} else if commandExists("dnf") {
			if err = runQuiet("dnf", "makecache", "--quiet"); err != nil {
				return "skipped", true, nil
			}
		} else if commandExists("yum") {
			if err = runQuiet("yum", "makecache", "--quiet"); err != nil {
				return "skipped", true, nil
			}
		}
		return "", false, nil

	case 2:
		if commandExists("apt-get") {
			args := append([]string{"install", "-y", "-qq"}, "curl", "ca-certificates", "openssl", "gnupg", "lsb-release")
			if err = runQuiet("apt-get", args...); err != nil {
				return err.Error(), true, nil
			}
		} else if commandExists("dnf") {
			args := append([]string{"install", "-y", "-q"}, "curl", "ca-certificates", "openssl")
			if err = runQuiet("dnf", args...); err != nil {
				return err.Error(), true, nil
			}
		}
		return "", false, nil

	case 3: // PostgreSQL
		if cfg.SkipPG {
			return "skipped (--skip-postgres)", true, nil
		}
		if !commandExists("psql") {
			if commandExists("apt-get") {
				if err = runQuiet("apt-get", "install", "-y", "-qq", "postgresql", "postgresql-contrib"); err != nil {
					return "", false, fmt.Errorf("apt install postgresql: %w\n\n%s", err, pgDiagnostic())
				}
			} else if commandExists("dnf") {
				runQuiet("dnf", "install", "-y", "-q", "postgresql-server", "postgresql-contrib")
				runQuiet("postgresql-setup", "--initdb")
			} else if commandExists("yum") {
				runQuiet("yum", "install", "-y", "-q", "postgresql-server")
				runQuiet("postgresql-setup", "initdb")
			} else {
				return "", false, fmt.Errorf("no supported package manager found — install PostgreSQL 15+ manually from https://postgresql.org/download")
			}
		}

		if err = startPostgresService(); err != nil {
			return "", false, fmt.Errorf("%w\n\nDiagnostic output:\n%s", err, pgDiagnostic())
		}

		// Wait for readiness
		ready := false
		for i := 0; i < 20; i++ {
			if _, e := runSilent("pg_isready", "-q"); e == nil {
				ready = true
				break
			}
			time.Sleep(time.Second)
		}
		if !ready {
			return "", false, fmt.Errorf("postgresql did not become ready within 20s\n\nDiagnostic:\n%s", pgDiagnostic())
		}

		v, _ := runSilent("psql", "--version")
		pgMaj := ""
		if parts := strings.Fields(strings.TrimSpace(v)); len(parts) >= 3 {
			pgMaj = strings.Split(parts[2], ".")[0]
		}
		installPgvector(pgMaj)
		return "installed — " + strings.TrimSpace(v), false, nil

	case 4: // Docker (optional)
		if cfg.SkipDocker {
			return "skipped (--skip-docker)", true, nil
		}
		if commandExists("docker") {
			v, _ := runSilent("docker", "--version")
			return strings.TrimSpace(v), false, nil
		}
		runQuiet("apt-get", "remove", "-y", "-qq",
			"docker.io", "docker-compose", "docker-compose-v2",
			"docker-doc", "podman-docker", "containerd", "runc")
		runQuiet("apt-get", "update", "-qq")
		scriptPath := "/tmp/get-docker.sh"
		if _, err = runSilent("curl", "-fsSL", "https://get.docker.com", "-o", scriptPath); err != nil {
			return "skipped (download failed — install Docker manually)", true, nil
		}
		os.Chmod(scriptPath, 0755)
		if err = runQuiet("sh", scriptPath); err != nil {
			return "skipped (install failed — run 'curl https://get.docker.com | sh' manually)", true, nil
		}
		runQuiet("systemctl", "enable", "--now", "docker")
		v, _ := runSilent("docker", "--version")
		return "installed — " + strings.TrimSpace(v), false, nil

	case 5: // OS user
		if _, err = runSilent("id", "qorven"); err != nil {
			if err = runQuiet("useradd", "--system", "--no-create-home",
				"--shell", "/usr/sbin/nologin", "qorven"); err != nil {
				return "", false, fmt.Errorf("useradd: %w", err)
			}
			if commandExists("docker") {
				runSilent("usermod", "-aG", "docker", "qorven")
			}
			return "created", false, nil
		}
		return "already exists", false, nil

	case 6: // Data dirs
		dirs := []string{cfg.DataDir, cfg.DataDir + "/logs", cfg.DataDir + "/workspaces", cfg.DataDir + "/tls", "/etc/qorven"}
		for _, d := range dirs {
			os.MkdirAll(d, 0755)
			runSilent("chown", "qorven:qorven", d)
		}
		return cfg.DataDir, false, nil

	case 7: // DB
		if cfg.SkipPG {
			return "skipped (--skip-postgres)", true, nil
		}

		// Try sudo -u postgres first (standard Debian/Ubuntu).
		// Fall back to TCP connection as postgres superuser.
		psqlUser := ""
		if _, e := runSilent("id", "postgres"); e == nil {
			psqlUser = "postgres"
		}

		psql := func(sql string, db string) (string, error) {
			if psqlUser != "" {
				out, err := runSilent("sudo", "-u", psqlUser, "psql", "-d", db, "-tAc", sql)
				return out, err
			}
			out, err := runSilent("psql", "-h", "127.0.0.1", "-U", "postgres", "-d", db, "-tAc", sql)
			return out, err
		}

		// Create DB
		out, _ := psql("SELECT 1 FROM pg_database WHERE datname='qorven'", "postgres")
		if strings.TrimSpace(out) != "1" {
			if psqlUser != "" {
				if dbOut, dbErr := runSilent("sudo", "-u", psqlUser, "createdb", "qorven"); dbErr != nil {
					return "", false, fmt.Errorf("createdb: %w — %s", dbErr, strings.TrimSpace(dbOut))
				}
			} else {
				if dbOut, dbErr := runSilent("psql", "-h", "127.0.0.1", "-U", "postgres",
					"-c", "CREATE DATABASE qorven;"); dbErr != nil {
					return "", false, fmt.Errorf("createdb: %w — %s", dbErr, strings.TrimSpace(dbOut))
				}
			}
		}

		// Create role
		out, _ = psql("SELECT 1 FROM pg_roles WHERE rolname='qorven'", "postgres")
		if strings.TrimSpace(out) != "1" {
			if psqlUser != "" {
				if cuOut, cuErr := runSilent("sudo", "-u", psqlUser, "createuser",
					"--no-superuser", "--no-createdb", "--no-createrole", "qorven"); cuErr != nil {
					return "", false, fmt.Errorf("createuser: %w — %s", cuErr, strings.TrimSpace(cuOut))
				}
			} else {
				psql("CREATE ROLE qorven LOGIN;", "postgres")
			}
		}

		// Always grant — idempotent, ensures re-run repairs broken permissions
		psql("GRANT ALL PRIVILEGES ON DATABASE qorven TO qorven;", "postgres")
		psql("GRANT ALL ON SCHEMA public TO qorven;", "qorven")

		// pgvector — must be created as superuser (postgres), not the app user.
		// Also grant qorven SUPERUSER so it can recreate the extension after factory reset.
		psql("CREATE EXTENSION IF NOT EXISTS vector;", "postgres")
		psql("ALTER USER qorven SUPERUSER;", "postgres")
		if pgvectorEnabled() {
			return "ready — pgvector enabled", false, nil
		}
		return "ready — pgvector not available (vector search disabled, Qorven works without it)", true, nil

	case 8: // Binary
		binDir := "/opt/qorven/bin"
		target := binDir + "/qorven"
		symlink := "/usr/local/bin/qorven"
		self, _ := os.Executable()
		selfReal, _ := filepath.EvalSymlinks(self)
		targetReal, evalErr := filepath.EvalSymlinks(target)
		if evalErr == nil && selfReal == targetReal {
			return "already in place", false, nil
		}
		runQuiet("systemctl", "stop", "qorven")
		if err = os.MkdirAll(binDir, 0755); err != nil {
			return "", false, fmt.Errorf("create bin dir: %w", err)
		}
		runQuiet("chown", "qorven:qorven", binDir)
		data, readErr := os.ReadFile(self)
		if readErr != nil {
			return "", false, fmt.Errorf("read binary: %w", readErr)
		}
		tmp := target + ".installing"
		if err = os.WriteFile(tmp, data, 0755); err != nil {
			return "", false, fmt.Errorf("write temp binary: %w", err)
		}
		if err = os.Rename(tmp, target); err != nil {
			os.Remove(tmp)
			return "", false, fmt.Errorf("rename binary: %w", err)
		}
		runQuiet("chown", "qorven:qorven", target)
		// Remove any old symlink at the legacy location before creating a new one
		os.Remove(symlink)
		if err = os.Symlink(target, symlink); err != nil {
			slog.Warn("install.symlink_failed", "err", err)
		}
		return target, false, nil

	case 9: // systemd
		if !commandExists("systemctl") {
			return "not available — skipped (Qorven is installed but will not auto-start; run: sudo qorven start)", true, nil
		}
		unit := `[Unit]
Description=Qorven AI Gateway
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=qorven
Group=qorven
Environment=QORVEN_CONFIG=/etc/qorven/config.toml
Environment=QORVEN_DATA_DIR=/var/lib/qorven
EnvironmentFile=-/etc/qorven/.env
ExecStart=/opt/qorven/bin/qorven start
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
WorkingDirectory=/var/lib/qorven
NoNewPrivileges=yes
ProtectSystem=full
ProtectHome=read-only
ReadWritePaths=/var/lib/qorven /etc/qorven /opt/qorven/bin
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
`
		if err = os.WriteFile("/etc/systemd/system/qorven.service", []byte(unit), 0644); err != nil {
			return "", false, fmt.Errorf("write unit: %w", err)
		}
		runQuiet("systemctl", "daemon-reload")
		runQuiet("systemctl", "enable", "qorven")
		runQuiet("systemctl", "start", "qorven")
		return "enabled", false, nil

	case 10: // nginx
		if cfg.SkipNginx {
			return "skipped (not requested)", true, nil
		}
		port := cfg.Port
		if port == 0 {
			port = 8486
		}
		nginxConf := fmt.Sprintf(`map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 80;
    listen [::]:80;

    location ~ ^/(ws|api/ws) {
        proxy_pass http://127.0.0.1:%d;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_read_timeout 3600s;
    }

    location ~ ^/(health|api/) {
        proxy_pass http://127.0.0.1:%d;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    location / {
        proxy_pass http://127.0.0.1:%d;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
`, port, port, port)
		if !commandExists("nginx") {
			if commandExists("apt-get") {
				if _, err = runSilent("apt-get", "install", "-y", "-qq", "nginx"); err != nil {
					return "skipped (nginx install failed)", true, nil
				}
			} else if commandExists("dnf") {
				runQuiet("dnf", "install", "-y", "-q", "nginx")
			}
		}
		confPath := "/etc/nginx/conf.d/qorven.conf"
		if err = os.WriteFile(confPath, []byte(nginxConf), 0644); err != nil {
			return "", false, fmt.Errorf("write nginx config: %w", err)
		}
		os.Remove("/etc/nginx/sites-enabled/default")
		if _, err = runSilent("nginx", "-t"); err != nil {
			return "", false, fmt.Errorf("nginx config test failed: %w", err)
		}
		runSilent("systemctl", "enable", "nginx")
		runSilent("systemctl", "reload", "nginx")
		return confPath, false, nil

	case 11: // Tailscale
		if cfg.SkipTailscale {
			return "skipped (--skip-tailscale)", true, nil
		}
		runQuiet("systemctl", "stop", "qorven")
		if commandExists("tailscale") {
			if out, err := exec.Command("tailscale", "ip", "-4").Output(); err == nil {
				ip := strings.TrimSpace(string(out))
				if strings.HasPrefix(ip, "100.") {
					return "connected:" + ip, false, nil
				}
			}
		}
		if !commandExists("tailscale") {
			script := "/tmp/tailscale-install.sh"
			if _, err = runSilent("curl", "-fsSL", "https://tailscale.com/install.sh", "-o", script); err != nil {
				return "skipped (download failed)", true, nil
			}
			os.Chmod(script, 0755)
			if err = runQuiet("sh", script); err != nil {
				return "skipped (install failed)", true, nil
			}
		}
		if cfg.TailscaleAuthKey != "" {
			if err = runQuiet("tailscale", "up",
				"--auth-key", cfg.TailscaleAuthKey,
				"--ssh", "--accept-routes", "--accept-dns"); err != nil {
				return "skipped (auth-key rejected)", true, nil
			}
			for i := 0; i < 15; i++ {
				time.Sleep(time.Second)
				if out, e := exec.Command("tailscale", "ip", "-4").Output(); e == nil {
					ip := strings.TrimSpace(string(out))
					if strings.HasPrefix(ip, "100.") {
						return "connected:" + ip, false, nil
					}
				}
			}
			return "skipped (IP not assigned)", true, nil
		}
		tsCmd := exec.Command("tailscale", "up", "--ssh", "--accept-routes", "--accept-dns")
		pr, pw, pipeErr := os.Pipe()
		if pipeErr != nil {
			return "skipped (pipe error)", true, nil
		}
		tsCmd.Stdout = pw
		tsCmd.Stderr = pw
		if err = tsCmd.Start(); err != nil {
			pw.Close(); pr.Close()
			return "skipped (start failed)", true, nil
		}
		pw.Close()
		authURL := ""
		buf := make([]byte, 4096)
		accumulated := ""
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) && authURL == "" {
			pr.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			n, _ := pr.Read(buf)
			if n > 0 {
				accumulated += string(buf[:n])
				for _, line := range strings.Split(accumulated, "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "https://login.tailscale.com/") {
						authURL = line
						break
					}
				}
			}
			if ip, e := exec.Command("tailscale", "ip", "-4").Output(); e == nil {
				ipStr := strings.TrimSpace(string(ip))
				if strings.HasPrefix(ipStr, "100.") {
					tsCmd.Process.Kill()
					pr.Close()
					return "connected:" + ipStr, false, nil
				}
			}
		}
		pr.Close()
		if authURL != "" {
			return "url:" + authURL, false, nil
		}
		tsCmd.Process.Kill()
		return "skipped (no auth URL found)", true, nil
	}
	return "", false, nil
}
