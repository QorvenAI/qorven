// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package apps

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const appRoleName = "qorven_app"

// buildAppDSN swaps the user and password in superConnStr to produce a DSN for
// the qorven_app restricted role. superConnStr must be a URL of the form
// postgres://user:pass@host:port/db[?params]. Returns an error if superConnStr
// cannot be parsed.
//
// This is a pure function (no DB I/O) and is called both by ensureAppRole and
// in unit tests.
func buildAppDSN(superConnStr, appPassword string) (string, error) {
	u, err := url.Parse(superConnStr)
	if err != nil {
		return "", fmt.Errorf("buildAppDSN: parse %q: %w", superConnStr, err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("buildAppDSN: unsupported scheme %q (expected postgres://…)", u.Scheme)
	}
	// Replace the userinfo with qorven_app + appPassword. url.UserPassword
	// percent-encodes special characters in the password automatically.
	u.User = url.UserPassword(appRoleName, appPassword)
	return u.String(), nil
}

// ensureAppRole provisions (or re-provisions) a least-privilege login role
// qorven_app that app tool subprocesses use for database access. It returns
// the DSN for that role, or "" if the role cannot be provisioned.
//
// Callers MUST treat "" as "withhold the DSN entirely" — never fall back to
// the superuser connstring.
//
// Idempotent: safe to call on every startup. The role is created if it does
// not exist; the password is (re)set so config changes take effect; GRANTs are
// reapplied unconditionally (GRANT is idempotent in Postgres).
func ensureAppRole(ctx context.Context, pool *pgxpool.Pool, superConnStr, appPassword string) string {
	if appPassword == "" {
		slog.Info("app_db_role.skip", "reason", "no app DB password configured — subprocesses will have no DB access")
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

	// Step 2: Always reset the password so a config change takes effect.
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

// dbNameFromConnStr extracts the database name from a postgres:// URL.
func dbNameFromConnStr(connStr string) string {
	u, err := url.Parse(connStr)
	if err != nil {
		return ""
	}
	// Path is "/<dbname>" — strip the leading slash.
	p := strings.TrimPrefix(u.Path, "/")
	// Strip any trailing query or extra path segments.
	if idx := strings.IndexAny(p, "/?#"); idx >= 0 {
		p = p[:idx]
	}
	return p
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
