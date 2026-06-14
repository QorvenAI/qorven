// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package apps

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/config"
)

const appRoleName = "qorven_app"

// buildAppDSN swaps the user and password in superConnStr to produce a DSN
// for the qorven_app restricted role. It handles two DSN forms:
//
//  1. URL form:  postgres://user:pass@host/db[?params]
//     and the common socket URL variant:
//     postgres:///db?host=/var/run/postgresql&user=qorven&sslmode=disable
//
//  2. Keyword form: host=/var/run/postgresql user=qorven dbname=qorven sslmode=disable
//
// In both cases the resulting DSN connects as qorven_app regardless of what
// user/password appeared in the original.
//
// This is a pure function (no DB I/O) and is called both by ensureAppRole and
// in unit tests.
func buildAppDSN(superConnStr, appPassword string) (string, error) {
	u, err := url.Parse(superConnStr)
	if err != nil {
		return "", fmt.Errorf("buildAppDSN: parse %q: %w", superConnStr, err)
	}

	if u.Scheme == "postgres" || u.Scheme == "postgresql" {
		// --- URL form (including socket URLs like postgres:///db?host=...&user=...) ---

		// Set the userinfo with qorven_app + appPassword.
		// url.UserPassword percent-encodes special characters automatically.
		u.User = url.UserPassword(appRoleName, appPassword)

		// Remove any user= or password= query params — libpq/pgx honours them
		// and they would override the URL userinfo we just set above.
		q := u.Query()
		q.Del("user")
		q.Del("password")
		u.RawQuery = q.Encode()

		return u.String(), nil
	}

	// If url.Parse produced an empty scheme the input is likely a libpq keyword
	// DSN (e.g. "host=/var/run/postgresql user=qorven dbname=qorven sslmode=disable").
	// Rewrite the user= and password= tokens in-place.
	if u.Scheme == "" {
		return rewriteKeywordDSN(superConnStr, appRoleName, appPassword), nil
	}

	return "", fmt.Errorf("buildAppDSN: unsupported scheme %q (expected postgres://… or a libpq keyword string)", u.Scheme)
}

// rewriteKeywordDSN rewrites the user= and password= key-value pairs in a
// libpq keyword connection string. Keys that are absent are appended.
// Handles both single-word values and single-quoted values.
//
// Examples:
//
//	host=/run/pg user=qorven dbname=qorven  →  host=/run/pg user=qorven_app password=… dbname=qorven
func rewriteKeywordDSN(dsn, newUser, newPassword string) string {
	dsn = rewriteKWToken(dsn, "user", newUser)
	dsn = rewriteKWToken(dsn, "password", newPassword)
	return dsn
}

// rewriteKWToken replaces the value of keyword key in a libpq keyword DSN, or
// appends it if absent. Understands unquoted and single-quoted values.
func rewriteKWToken(dsn, key, value string) string {
	// Pattern: key=<value> where value is either non-whitespace or 'quoted string'.
	// We replace the first occurrence.
	reUnquoted := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(key) + `=\S+`)
	reQuoted := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(key) + `='[^']*'`)

	replacement := key + "=" + pgKWQuote(value)

	if reQuoted.MatchString(dsn) {
		return reQuoted.ReplaceAllLiteralString(dsn, replacement)
	}
	if reUnquoted.MatchString(dsn) {
		return reUnquoted.ReplaceAllLiteralString(dsn, replacement)
	}
	// Key not present — append.
	return strings.TrimRight(dsn, " ") + " " + replacement
}

// pgKWQuote single-quotes a value for a libpq keyword DSN, escaping
// embedded single quotes and backslashes per the libpq spec.
func pgKWQuote(v string) string {
	// If value has no spaces, quotes, or backslashes it doesn't need quoting.
	if !strings.ContainsAny(v, " \t\\'") {
		return v
	}
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `'`, `\'`)
	return "'" + v + "'"
}

// ResolveAppDBPassword returns the password to use for the qorven_app DB role.
// If the operator-supplied password (from env/config) is non-empty it is used
// as-is, giving operators an explicit override. Otherwise the password is
// loaded-or-generated from disk (see loadOrCreateAppDBPassword), so prod
// deployments that never set QORVEN_APP_DB_PASSWORD still get a stable,
// strong, least-privilege credential automatically.
func ResolveAppDBPassword(configuredPassword string) string {
	if configuredPassword != "" {
		return configuredPassword
	}
	return loadOrCreateAppDBPassword()
}

// loadOrCreateAppDBPassword returns the password for the qorven_app role.
// It reads the password from <DataDir>/app_db_password; if the file is
// absent it generates a new 32-byte hex password, persists it at 0600, and
// returns it. On any write error it still returns the generated value so the
// current boot works — a warning is logged, but the secret is NEVER logged.
//
// This mirrors loadOrCreateSecret in internal/auth/auth.go.
func loadOrCreateAppDBPassword() string {
	path := config.Sub("app_db_password")

	data, err := os.ReadFile(path)
	if err == nil {
		pw := strings.TrimSpace(string(data))
		if pw != "" {
			return pw
		}
	}

	// Generate a new 32-byte (256-bit) random password, hex-encoded (64 chars).
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		// crypto/rand failure is extremely unusual; fall back to a weaker value
		// rather than refusing to start.
		slog.Warn("app_db_role: crypto/rand failed; app DB password will not persist across restarts")
		return "fallback-" + hex.EncodeToString([]byte("qorven-app-insecure"))
	}
	pw := hex.EncodeToString(raw)

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		slog.Warn("app_db_role: cannot create data dir for app_db_password", "err", err)
	} else if err := os.WriteFile(path, []byte(pw), 0600); err != nil {
		slog.Warn("app_db_role: cannot persist app_db_password", "err", err)
	} else {
		slog.Info("app_db_role: generated new app DB password")
	}
	return pw
}

// ensureAppRole provisions (or re-provisions) a least-privilege login role
// qorven_app that app tool subprocesses use for database access. It returns
// the DSN for that role, or "" if the role cannot be provisioned.
//
// Callers MUST treat "" as "withhold the DSN entirely" — never fall back to
// the superuser connstring.
//
// Idempotent: safe to call on every startup. The role is created if it does
// not exist; the password is (re)set so credential rotations take effect;
// GRANTs are reapplied unconditionally (GRANT is idempotent in Postgres).
func ensureAppRole(ctx context.Context, pool *pgxpool.Pool, superConnStr, appPassword string) string {
	if appPassword == "" {
		slog.Info("app_db_role.skip", "reason", "no app DB password — subprocesses will have no DB access")
		return ""
	}

	appDSN, err := buildAppDSN(superConnStr, appPassword)
	if err != nil {
		slog.Warn("app_db_role.skip", "reason", "cannot build app DSN", "err", err)
		return ""
	}

	// Extract the database name from the superuser DSN to issue GRANT CONNECT.
	dbName := dbNameFromConnStr(superConnStr)
	if dbName == "" {
		slog.Warn("app_db_role.skip", "reason", "cannot determine database name from superuser DSN")
		return ""
	}

	// Use a single connection from the pool for all DDL. Any error that
	// indicates insufficient privilege (e.g. CREATEROLE not held) causes us
	// to withhold the DSN entirely.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		slog.Warn("app_db_role.skip", "reason", "pool.Acquire failed", "err", err)
		return ""
	}
	defer conn.Release()

	// Step 1: CREATE ROLE if not exists (idempotent via DO block).
	createSQL := `DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '` + appRoleName + `') THEN
    CREATE ROLE ` + appRoleName + ` LOGIN PASSWORD '` + escapePGLiteral(appPassword) + `';
  END IF;
END $$;`
	if _, err := conn.Exec(ctx, createSQL); err != nil {
		if isInsufficientPrivilege(err) {
			slog.Warn("app_db_role.skip", "reason", "superuser lacks CREATEROLE — subprocesses will have no DB access")
		} else {
			slog.Warn("app_db_role.skip", "reason", "CREATE ROLE failed", "err", err)
		}
		return ""
	}

	// Step 2: Always reset the password so a credential rotation takes effect.
	alterSQL := `ALTER ROLE ` + appRoleName + ` WITH LOGIN PASSWORD '` + escapePGLiteral(appPassword) + `';`
	if _, err := conn.Exec(ctx, alterSQL); err != nil {
		slog.Warn("app_db_role.skip", "reason", "ALTER ROLE failed", "err", err)
		return ""
	}

	// Step 3: GRANT CONNECT on the database (idempotent).
	grantConnSQL := `GRANT CONNECT ON DATABASE ` + pgQuoteIdent(dbName) + ` TO ` + appRoleName + `;`
	if _, err := conn.Exec(ctx, grantConnSQL); err != nil {
		slog.Warn("app_db_role.skip", "reason", "GRANT CONNECT failed", "err", err)
		return ""
	}

	// Step 4: GRANT USAGE on schema public (idempotent).
	if _, err := conn.Exec(ctx, `GRANT USAGE ON SCHEMA public TO `+appRoleName+`;`); err != nil {
		slog.Warn("app_db_role.skip", "reason", "GRANT USAGE failed", "err", err)
		return ""
	}

	// Step 5: GRANT SELECT on all existing tables (idempotent).
	if _, err := conn.Exec(ctx, `GRANT SELECT ON ALL TABLES IN SCHEMA public TO `+appRoleName+`;`); err != nil {
		slog.Warn("app_db_role.skip", "reason", "GRANT SELECT failed", "err", err)
		return ""
	}

	// Step 6: ALTER DEFAULT PRIVILEGES so future tables are also readable.
	if _, err := conn.Exec(ctx, `ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO `+appRoleName+`;`); err != nil {
		slog.Warn("app_db_role.skip", "reason", "ALTER DEFAULT PRIVILEGES failed", "err", err)
		return ""
	}

	slog.Info("app_db_role.ready", "role", appRoleName)
	return appDSN
}

// grantAppWriteOnNewTables grants INSERT/UPDATE/DELETE to qorven_app on tables
// that were created by a db_write app's migrations. To avoid over-granting on
// Qorven's own core tables, only tables that did NOT exist before the migration
// ran are granted write access.
//
// tablesBefore is the set of table names in the public schema immediately before
// RunAppMigrations was called. tablesAfter is the set immediately after.
// The difference is the set of app-owned tables.
//
// If the diff is empty (the app's migrations created no new tables), this is a
// no-op. If any GRANT fails, the error is logged but Install is not aborted —
// read-only access remains in force (the safe fallback).
func grantAppWriteOnNewTables(ctx context.Context, pool *pgxpool.Pool, appSlug string, tablesBefore, tablesAfter map[string]struct{}) {
	var newTables []string
	for t := range tablesAfter {
		if _, existed := tablesBefore[t]; !existed {
			newTables = append(newTables, t)
		}
	}
	if len(newTables) == 0 {
		return
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		slog.Warn("app_db_role.grant_write.skip", "app", appSlug, "reason", "pool.Acquire failed", "err", err)
		return
	}
	defer conn.Release()

	// Grant INSERT/UPDATE/DELETE on each new table individually.
	// We grant per-table (not ALL TABLES) to ensure Qorven core tables
	// that predate this migration are never write-granted.
	for _, tbl := range newTables {
		sql := `GRANT INSERT, UPDATE, DELETE ON ` + pgQuoteIdent(tbl) + ` TO ` + appRoleName + `;`
		if _, err := conn.Exec(ctx, sql); err != nil {
			slog.Warn("app_db_role.grant_write.failed", "app", appSlug, "table", tbl, "err", err)
			// Continue — partial grants are better than none.
		}
	}

	// Grant USAGE + SELECT on all sequences (needed for serial/identity columns).
	if _, err := conn.Exec(ctx,
		`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO `+appRoleName+`;`,
	); err != nil {
		slog.Warn("app_db_role.grant_sequences.failed", "app", appSlug, "err", err)
	}

	slog.Info("app_db_role.grant_write.done", "app", appSlug, "tables", newTables)
}

// publicTableNames returns the set of table names currently in the public schema.
// Returns an empty map (not nil) on any error so callers can always use it safely.
func publicTableNames(ctx context.Context, pool *pgxpool.Pool) map[string]struct{} {
	rows, err := pool.Query(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_type = 'BASE TABLE'`)
	if err != nil {
		return map[string]struct{}{}
	}
	defer rows.Close()
	out := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			out[name] = struct{}{}
		}
	}
	return out
}

// dbNameFromConnStr extracts the database name from a postgres:// URL or a
// libpq keyword DSN.
func dbNameFromConnStr(connStr string) string {
	u, err := url.Parse(connStr)
	if err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		// Path is "/<dbname>" — strip the leading slash.
		p := strings.TrimPrefix(u.Path, "/")
		if idx := strings.IndexAny(p, "/?#"); idx >= 0 {
			p = p[:idx]
		}
		if p != "" {
			return p
		}
		// Socket URL: try dbname= query param
		if db := u.Query().Get("dbname"); db != "" {
			return db
		}
	}
	// Keyword DSN: look for dbname=<value>
	return extractKWValue(connStr, "dbname")
}

// extractKWValue extracts the value of key from a libpq keyword DSN.
// Returns "" if absent.
func extractKWValue(dsn, key string) string {
	reQuoted := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(key) + `='([^']*)'`)
	reUnquoted := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(key) + `=(\S+)`)
	if m := reQuoted.FindStringSubmatch(dsn); len(m) == 2 {
		return m[1]
	}
	if m := reUnquoted.FindStringSubmatch(dsn); len(m) == 2 {
		return m[1]
	}
	return ""
}

// escapePGLiteral escapes a string for safe embedding inside a Postgres
// single-quoted literal by doubling single quotes. Used for the password
// in DDL (CREATE/ALTER ROLE … PASSWORD '…') which cannot be parameterised.
func escapePGLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// pgQuoteIdent double-quotes a Postgres identifier, doubling any embedded
// double-quote characters. Used for the database name in GRANT CONNECT.
func pgQuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// isInsufficientPrivilege returns true if the error text indicates that the
// superuser role lacks the CREATEROLE privilege or similar.
func isInsufficientPrivilege(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "insufficient privilege") ||
		strings.Contains(msg, "must have createrole")
}
