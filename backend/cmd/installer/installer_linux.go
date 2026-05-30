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

func probeSocketDSN() string {
	port := probePGPort()
	base := "postgres:///qorven?host=/var/run/postgresql&user=qorven&sslmode=disable"
	if port != "" && port != "5432" {
		base += "&port=" + port
	}
	return base
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

func installPgvector(pgMaj string) {
	if pgMaj == "" {
		return
	}
	versioned := "postgresql-" + pgMaj + "-pgvector"
	if runQuiet("apt-get", "install", "-y", "-qq", versioned) == nil {
		return
	}
	distro, _ := runSilent("bash", "-c",
		`source /etc/os-release 2>/dev/null && echo "${ID}"`)
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
	runQuiet("apt-get", "install", "-y", "-qq", "postgresql-pgvector")
}

func platformRequirementsText() string {
	req := func(icon, label, detail string) string {
		return icon + "  " + fgSt.Render(label) + "\n" +
			"   " + mutedSt.Render(detail)
	}
	return req("🐧", "Ubuntu 20.04+ / Debian 11+", "or any systemd-based Linux") + "\n" +
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
		dimSt.Render("  Port 443 already in use") + "\n" +
		dimSt.Render("  postgresql service not starting") + "\n" +
		dimSt.Render("  Disk full — needs 10 GB free") + "\n" +
		dimSt.Render("  Not running as root (use sudo)")
	logs = mutedSt.Render("  journalctl -xe") + "\n" +
		mutedSt.Render("  apt-get install -f")
	return
}

func executeStep(idx int, cfg Config) (detail string, warn bool, err error) {
	switch idx {
	case 0:
		out, _ := runSilent("bash", "-c", `source /etc/os-release 2>/dev/null && echo "$PRETTY_NAME"`)
		arch, _ := runSilent("uname", "-m")
		return strings.TrimSpace(out) + "  " + strings.TrimSpace(arch), false, nil

	case 1:
		if err = runQuiet("apt-get", "update", "-qq"); err != nil {
			return "skipped", true, nil
		}
		return "", false, nil

	case 2:
		if err = runQuiet("apt-get", "install", "-y", "-qq",
			"curl", "ca-certificates", "gnupg", "lsb-release", "openssl"); err != nil {
			return err.Error(), true, nil
		}
		return "", false, nil

	case 3: // PostgreSQL
		if cfg.SkipPG {
			return "skipped (--skip-postgres)", true, nil
		}
		if !commandExists("psql") {
			if err = runQuiet("apt-get", "install", "-y", "-qq", "postgresql", "postgresql-contrib"); err != nil {
				return "", false, fmt.Errorf("apt install postgresql: %w", err)
			}
		}
		runQuiet("systemctl", "enable", "postgresql")
		runQuiet("systemctl", "start", "postgresql")
		ready := false
		for i := 0; i < 20; i++ {
			if _, e := runSilent("pg_isready", "-q"); e == nil {
				ready = true
				break
			}
			time.Sleep(time.Second)
		}
		if !ready {
			return "", false, fmt.Errorf("postgresql did not become ready within 20s — check: sudo systemctl status postgresql")
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
		out, _ := runSilent("sudo", "-u", "postgres", "psql", "-tAc",
			"SELECT 1 FROM pg_database WHERE datname='qorven'")
		if strings.TrimSpace(out) != "1" {
			if dbOut, dbErr := runSilent("sudo", "-u", "postgres", "createdb", "qorven"); dbErr != nil {
				return "", false, fmt.Errorf("createdb: %w — %s", dbErr, strings.TrimSpace(dbOut))
			}
		}
		out, _ = runSilent("sudo", "-u", "postgres", "psql", "-tAc",
			"SELECT 1 FROM pg_roles WHERE rolname='qorven'")
		if strings.TrimSpace(out) != "1" {
			if cuOut, cuErr := runSilent("sudo", "-u", "postgres", "createuser",
				"--no-superuser", "--no-createdb", "--no-createrole", "qorven"); cuErr != nil {
				return "", false, fmt.Errorf("createuser: %w — %s", cuErr, strings.TrimSpace(cuOut))
			}
		}
		runSilent("sudo", "-u", "postgres", "psql", "-c",
			"GRANT ALL PRIVILEGES ON DATABASE qorven TO qorven;")
		runSilent("sudo", "-u", "postgres", "psql", "-d", "qorven", "-c",
			"GRANT ALL ON SCHEMA public TO qorven;")
		runSilent("sudo", "-u", "postgres", "psql", "-d", "qorven", "-c",
			"CREATE EXTENSION IF NOT EXISTS vector;")
		return "ready", false, nil

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
		os.Remove(symlink)
		if err = os.Symlink(target, symlink); err != nil {
			slog.Warn("install.symlink_failed", "err", err)
		}
		return target, false, nil

	case 9: // systemd
		if !commandExists("systemctl") {
			return "not available — skipped", true, nil
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
			if _, err = runSilent("apt-get", "install", "-y", "-qq", "nginx"); err != nil {
				return "skipped (nginx install failed)", true, nil
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
