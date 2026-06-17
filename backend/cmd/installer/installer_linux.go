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

// platformUpgradeStepIndices returns the step indices that run during an
// upgrade. An upgrade only needs to: detect system (0), swap binary (8),
// restart the service (9). Steps 1-7 (package installs, OS user, DB) and
// 10-11 (nginx, Tailscale) are already in place and are skipped.
func platformUpgradeStepIndices() []int { return []int{0, 8, 9} }

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
	// Capture output so a migration failure surfaces the REAL reason (a missing
	// extension, a failing statement, a permission error) instead of a bare
	// "exit status 1".
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		// Migration errors put the useful line last — keep the tail.
		if len(msg) > 1200 {
			msg = "…" + msg[len(msg)-1200:]
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
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

// postgresServerPresent returns true when a PostgreSQL SERVER (not just the
// client tool psql) is installed and/or running. We check three signals:
//
//  1. pg_isready succeeds — server is actively listening.
//  2. pg_lsclusters lists at least one cluster (Debian/Ubuntu).
//  3. A postgresql server unit file exists in systemd.
//
// Importantly, installing only postgresql-client gives you `psql` but none
// of these three — so the old `commandExists("psql")` gate was a false
// positive that skipped the install even when no server was present.
func postgresServerPresent() bool {
	// 1. pg_isready — fastest path; succeeds if the server is up.
	if _, e := runSilent("pg_isready", "-q"); e == nil {
		return true
	}
	// 2. pg_lsclusters — Debian/Ubuntu cluster manager.
	if commandExists("pg_lsclusters") {
		out, err := exec.Command("pg_lsclusters", "-h").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return true
		}
	}
	// 3. systemd server unit file present.
	out, err := exec.Command("systemctl", "list-unit-files",
		"--no-legend", "--no-pager", "postgresql*").Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			fields := strings.Fields(line)
			// We want server units, not just the client; exclude pure client
			// packages which never register a service unit.
			if len(fields) >= 1 && strings.HasSuffix(fields[0], ".service") {
				return true
			}
		}
	}
	return false
}

// ensurePGDGRepo adds the official PGDG apt repository idempotently.
// If /etc/apt/sources.list.d/pgdg.list already contains the correct entry
// for this distro's codename the function is a no-op. The GPG key is
// written to /usr/share/keyrings/postgresql.asc.
func ensurePGDGRepo() error {
	// Determine codename: lsb_release -cs is canonical; fall back to
	// parsing /etc/os-release VERSION_CODENAME for minimal containers.
	codename, _ := runSilent("lsb_release", "-cs")
	codename = strings.TrimSpace(codename)
	if codename == "" {
		raw, err := os.ReadFile("/etc/os-release")
		if err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if strings.HasPrefix(line, "VERSION_CODENAME=") {
					codename = strings.Trim(strings.TrimPrefix(line, "VERSION_CODENAME="), `"'`)
					break
				}
			}
		}
	}
	if codename == "" {
		return fmt.Errorf("cannot determine distro codename for PGDG repo")
	}

	keyPath := "/usr/share/keyrings/postgresql.asc"
	listPath := "/etc/apt/sources.list.d/pgdg.list"
	repoLine := fmt.Sprintf(
		"deb [signed-by=%s] https://apt.postgresql.org/pub/repos/apt %s-pgdg main",
		keyPath, codename)

	// Idempotent: skip if the list already has the right entry.
	existing, readErr := os.ReadFile(listPath)
	if readErr == nil && strings.Contains(string(existing), codename+"-pgdg") {
		return nil
	}

	// Download and install the signing key.
	if _, err := runSilent("curl", "-fsSL",
		"https://www.postgresql.org/media/keys/accc4cf8.asc",
		"-o", keyPath); err != nil {
		return fmt.Errorf("download PGDG signing key: %w", err)
	}

	if err := os.WriteFile(listPath, []byte(repoLine+"\n"), 0644); err != nil {
		return fmt.Errorf("write PGDG sources.list: %w", err)
	}
	if err := runQuiet("apt-get", "update", "-qq"); err != nil {
		return fmt.Errorf("apt-get update after adding PGDG: %w", err)
	}
	return nil
}

// installPostgresApt installs PostgreSQL 16 from PGDG together with
// pgvector in one apt transaction. If the PG-16 packages are not
// available for this distro (very old release), it falls back to the
// distro's default postgresql package and a separate pgvector install.
func installPostgresApt() error {
	if err := ensurePGDGRepo(); err != nil {
		// PGDG failed — fall back to distro default.
		return runQuiet("apt-get", "install", "-y", "-qq", "postgresql", "postgresql-contrib")
	}

	// Try installing PG 16 + contrib + pgvector together from PGDG.
	err := runQuiet("apt-get", "install", "-y", "-qq",
		"postgresql-16",
		"postgresql-client-16",
		"postgresql-contrib-16",
		"postgresql-16-pgvector",
	)
	if err == nil {
		return nil
	}

	// PG 16 packages not available for this distro — fall back to
	// distro-default postgresql and let installPgvector handle the
	// pgvector package separately.
	if fbErr := runQuiet("apt-get", "install", "-y", "-qq",
		"postgresql", "postgresql-contrib"); fbErr != nil {
		return fmt.Errorf("apt install postgresql (fallback): %w", fbErr)
	}
	return nil
}

// runningPGMajor returns the major version of the PostgreSQL SERVER that is
// currently running (e.g. "16"). It tries multiple sources in order:
//
//  1. pg_lsclusters — most reliable on Debian/Ubuntu; shows the running server.
//  2. psql --version — works when the client matches the server major.
//  3. pg_config --version — available when the server's dev package is installed.
func runningPGMajor() string {
	// pg_lsclusters: first column is the major version.
	if commandExists("pg_lsclusters") {
		out, err := exec.Command("pg_lsclusters", "-h").Output()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 1 && fields[0] != "" {
					maj := strings.Split(fields[0], ".")[0]
					if maj != "" {
						return maj
					}
				}
			}
		}
	}
	// psql --version: "psql (PostgreSQL) 16.3"
	if v, err := runSilent("psql", "--version"); err == nil {
		parts := strings.Fields(v)
		if len(parts) >= 3 {
			return strings.Split(parts[2], ".")[0]
		}
	}
	// pg_config --version: "PostgreSQL 16.3"
	if v, err := runSilent("pg_config", "--version"); err == nil {
		parts := strings.Fields(v)
		if len(parts) >= 2 {
			return strings.Split(parts[1], ".")[0]
		}
	}
	return ""
}

// pgvectorAvailable checks whether the pgvector extension is available for
// the installed server (i.e. the shared library / control file exists).
// It does NOT check whether `CREATE EXTENSION vector` has been run.
func pgvectorAvailable(pgMaj string) bool {
	// Check the .control file for the specific server major (Debian/Ubuntu layout).
	if pgMaj != "" {
		controlPath := fmt.Sprintf("/usr/share/postgresql/%s/extension/vector.control", pgMaj)
		if _, err := os.Stat(controlPath); err == nil {
			return true
		}
	}
	// Generic path (RHEL / other layouts).
	paths := []string{
		"/usr/share/postgresql/extension/vector.control",
		"/usr/lib/postgresql/extension/vector.control",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// installPgvector ensures the pgvector extension package is installed for
// the given server major version. It now returns a bool (installed/already
// present) and an error. Callers treat a failure as a warning — Qorven
// works without vector search; the caller surfaces the appropriate message.
func installPgvector(pgMaj string) (bool, error) {
	if pgMaj == "" {
		return false, fmt.Errorf("pgvector: unknown server major version")
	}

	// Already installed?
	if pgvectorAvailable(pgMaj) {
		return true, nil
	}

	// Detect package manager.
	hasDnf := commandExists("dnf")
	hasYum := commandExists("yum")

	if commandExists("apt-get") {
		versioned := "postgresql-" + pgMaj + "-pgvector"

		// First attempt: try without adding the PGDG repo (it may already
		// be present, or the distro universe repo may carry it).
		if runQuiet("apt-get", "install", "-y", "-qq", versioned) == nil {
			return true, nil
		}

		// Ensure PGDG repo and retry. ensurePGDGRepo only runs apt-get update
		// when it actually writes the repo; if the repo already exists the apt
		// index may be stale on a fresh box, so refresh explicitly before retry.
		if err := ensurePGDGRepo(); err == nil {
			runQuiet("apt-get", "update", "-qq")
			if runQuiet("apt-get", "install", "-y", "-qq", versioned) == nil {
				return true, nil
			}
		}

		// Last-ditch fallback: unversioned name (Ubuntu universe).
		if runQuiet("apt-get", "install", "-y", "-qq", "postgresql-pgvector") == nil {
			return true, nil
		}
		return false, fmt.Errorf("could not install %s from any apt source", versioned)
	}

	if hasDnf || hasYum {
		pm := "dnf"
		if !hasDnf {
			pm = "yum"
		}
		// Add PGDG repo if not present.
		if _, e := runSilent("rpm", "-q", "pgdg-redhat-repo"); e != nil {
			repoUrl := fmt.Sprintf("https://download.postgresql.org/pub/repos/yum/reporpms/EL-$(rpm -E %%rhel)-x86_64/pgdg-redhat-repo-latest.noarch.rpm")
			runQuiet(pm, "install", "-y", repoUrl)
		}
		pkgName := fmt.Sprintf("pgvector_%s", pgMaj)
		if runQuiet(pm, "install", "-y", pkgName) == nil {
			return true, nil
		}
		if runQuiet(pm, "install", "-y", "pgvector") == nil {
			return true, nil
		}
		return false, fmt.Errorf("could not install pgvector for PG %s via %s", pgMaj, pm)
	}

	return false, fmt.Errorf("pgvector: no supported package manager found")
}

// startPostgresService handles the Debian/Ubuntu multi-name service problem.
// The generic "postgresql" unit may not exist; the real one is often
// "postgresql@16-main" or "postgresql@14-main". We try multiple names.
// After installing PG 16 specifically, if pg_lsclusters shows no cluster,
// we run pg_createcluster 16 main --start (PGDG Debian packages sometimes
// skip auto-create on fresh installs).
func startPostgresService() error {
	// If pg_lsclusters is present and shows no cluster, auto-create one.
	// This is needed after a fresh PGDG PG 16 install on Debian/Ubuntu.
	if commandExists("pg_lsclusters") {
		out, err := exec.Command("pg_lsclusters", "-h").Output()
		if err == nil && strings.TrimSpace(string(out)) == "" {
			// No clusters — create PG 16 main cluster.
			if commandExists("pg_createcluster") {
				runQuiet("pg_createcluster", "16", "main", "--start")
			}
		}
	}

	// Try generic name first (works on some distros).
	if runQuiet("systemctl", "start", "postgresql") == nil {
		return nil
	}
	// Try all versioned cluster units (Debian/Ubuntu style).
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
	// pg_ctlcluster fallback (Debian-only utility).
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
	// Final check.
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

// pgvectorEnabled checks whether the vector extension is actually active in
// the qorven database (i.e. CREATE EXTENSION vector has been run).
func pgvectorEnabled() bool {
	out, err := runSilent("sudo", "-u", "postgres", "psql", "-d", "qorven", "-tAc",
		"SELECT 1 FROM pg_extension WHERE extname='vector'")
	if err != nil {
		// Try TCP fallback.
		out, err = runSilent("psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "qorven", "-tAc",
			"SELECT 1 FROM pg_extension WHERE extname='vector'")
	}
	return err == nil && strings.TrimSpace(out) == "1"
}

// platformServiceCommands returns the post-install management commands shown in
// the final summary (plain text — no terminal styling).
func platformServiceCommands() string {
	return "  systemctl status qorven\n" +
		"  journalctl -u qorven -f\n" +
		"  sudo qorven migrate up"
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

		// FIX 1: Detect a real running/installed SERVER, not just the psql
		// client tool. postgresql-client provides `psql` but no server; the
		// old commandExists("psql") gate was therefore a false positive.
		if !postgresServerPresent() {
			// FIX 2: Install PostgreSQL 16 from PGDG + pgvector in one apt
			// transaction. For RHEL/dnf/yum the existing PGDG-RPM path is used.
			if commandExists("apt-get") {
				if err = installPostgresApt(); err != nil {
					return "", false, fmt.Errorf("install postgresql: %w\n\n%s", err, pgDiagnostic())
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

		// Wait for readiness.
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

		// FIX 3: Detect running server major and ensure pgvector ALWAYS,
		// regardless of whether PG was just freshly installed or was
		// pre-existing. This is the fix for the silent re-run skip.
		pgMaj := runningPGMajor()
		pvOK, pvErr := installPgvector(pgMaj)

		v, _ := runSilent("psql", "--version")
		vStr := strings.TrimSpace(v)

		if pvOK {
			return "installed — " + vStr + " — pgvector enabled ✓", false, nil
		}
		// pgvector package install failed here. Don't abort yet — the DB-setup
		// step retries the install and is the authoritative gate (pgvector is
		// REQUIRED by the schema, so it will hard-fail there if still missing).
		if pvErr != nil {
			slog.Warn("install.pgvector_failed", "err", pvErr)
		}
		return "installed — " + vStr + " — pgvector pending (will be verified at database setup)", true, nil

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

		// Always grant — idempotent, ensures re-run repairs broken permissions.
		psql("GRANT ALL PRIVILEGES ON DATABASE qorven TO qorven;", "postgres")
		psql("GRANT ALL ON SCHEMA public TO qorven;", "qorven")

		// pgvector — must be created as superuser (postgres), not the app user,
		// and MUST be created in the qorven database (the one migrations run
		// against), NOT the postgres maintenance database. Also grant qorven
		// SUPERUSER so it can recreate the extension after a factory reset.
		//
		// pgvector is a HARD requirement, NOT an optional add-on: the schema
		// declares vector(384)/vector(1536) columns and ivfflat/hnsw indexes.
		// Without the extension, the migration step dies with a cryptic
		// "type \"vector\" does not exist". We therefore enable it HERE and, if
		// it cannot be enabled, fail early with a clear, actionable message at
		// the step that owns the problem — instead of letting migration fail
		// unreadably four steps later.
		psql("CREATE EXTENSION IF NOT EXISTS vector;", "qorven")
		psql("ALTER USER qorven SUPERUSER;", "postgres")
		if !pgvectorEnabled() {
			// Last-ditch: (re)install the package for the RUNNING server major
			// — step 3's install may have warned-and-continued — then retry.
			if _, ivErr := installPgvector(runningPGMajor()); ivErr == nil {
				psql("CREATE EXTENSION IF NOT EXISTS vector;", "qorven")
			}
		}
		if pgvectorEnabled() {
			return "ready — pgvector enabled", false, nil
		}
		// Still not enabled. This WILL break migration, so stop now with the
		// real PostgreSQL error and the exact package to install.
		pgMaj := runningPGMajor()
		createOut, _ := psql("CREATE EXTENSION vector;", "qorven")
		detail := strings.TrimSpace(createOut)
		if detail == "" {
			detail = "(no error detail from PostgreSQL — the pgvector package is likely not installed)"
		}
		return "", false, fmt.Errorf(
			"pgvector could not be enabled — it is REQUIRED (the schema uses vector columns; "+
				"the install cannot continue without it).\n\n"+
				"PostgreSQL major version: %s\n"+
				"Install the matching package, then re-run the installer:\n"+
				"  Debian/Ubuntu:  sudo apt-get install -y postgresql-%s-pgvector\n"+
				"  RHEL/Fedora:    sudo dnf install -y pgvector_%s\n\n"+
				"PostgreSQL said: %s",
			pgMaj, pgMaj, pgMaj, detail)

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
		// Remove any old symlink at the legacy location before creating a new one.
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
		// Do NOT start the service here. Config + .env are written and migrations
		// run AFTER all steps complete; starting now would boot a service with no
		// DSN that restart-loops and races the installer's own migration on the
		// pg_extension catalog ("tuple concurrently updated"). The finalize step
		// starts the service once the schema is ready.
		return "enabled (starts after migrations)", false, nil

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
