// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/audit"
	"golang.org/x/crypto/bcrypt"
)

var resetTargets = map[string]string{
	// Note: tasks.origin_session_id references sessions; if tasks rows exist this
	// will fail with a FK violation — that is intentional (no silent cascade).
	"sessions":      "TRUNCATE sessions, agent_messages",
	"tasks":         "TRUNCATE tasks, task_events CASCADE",
	"memories":      "TRUNCATE memory_embeddings, memory_hierarchy CASCADE",
	"audit_log":     "TRUNCATE audit_log",
	"provider_keys": "TRUNCATE provider_keys, credentials CASCADE",
	"agents":        "DELETE FROM agents WHERE agent_key NOT IN ('chief', 'prime')",
}

func (gw *Gateway) handleAdminReset(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "admin role required",
			"code":  "admin_only",
		})
		return
	}

	target := chi.URLParam(r, "target")
	sql, ok := resetTargets[target]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("unknown target %q; valid: sessions, tasks, memories, audit_log, provider_keys, agents", target),
			"code":  "unknown_target",
		})
		return
	}

	tag, err := gw.db.Pool.Exec(r.Context(), sql)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	if gw.auditStore != nil {
		gw.auditStore.Log(r.Context(), defaultTenant,
			audit.ActorUser, user.ID, user.Username,
			"admin_reset", "reset", target,
			map[string]any{"target": target, "deleted_rows": tag.RowsAffected()},
			r.RemoteAddr,
		)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"target":       target,
		"deleted_rows": tag.RowsAffected(),
	})
}

func (gw *Gateway) handleAdminFactoryReset(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "admin role required",
			"code":  "admin_only",
		})
		return
	}

	var body struct {
		Password string `json:"password"`
		Confirm  string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if body.Confirm != "RESET" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": `confirm field must equal exactly "RESET"`,
			"code":  "bad_confirm",
		})
		return
	}

	var passwordHash string
	var isActive bool
	var lockedUntil *time.Time
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT password_hash, is_active, locked_until FROM users WHERE id = $1`, user.ID,
	).Scan(&passwordHash, &isActive, &lockedUntil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to verify identity"})
		return
	}
	if !isActive {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "account is disabled", "code": "account_disabled"})
		return
	}
	if lockedUntil != nil && time.Now().Before(*lockedUntil) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "account is locked", "code": "account_locked"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(body.Password)) != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "incorrect password", "code": "bad_password"})
		return
	}

	if gw.auditStore != nil {
		gw.auditStore.Log(r.Context(), defaultTenant,
			audit.ActorUser, user.ID, user.Username,
			"factory_reset", "system", "",
			map[string]any{"initiated_by": user.Username},
			r.RemoteAddr,
		)
	}

	dbUser := "postgres"
	if gw.cfg != nil && gw.cfg.Database.DSN != "" {
		if u, err := url.Parse(gw.cfg.Database.DSN); err == nil && u.User != nil {
			dbUser = u.User.Username()
		}
	}

	pool := gw.db.Pool

	if _, err := pool.Exec(r.Context(), `DROP SCHEMA public CASCADE`); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "DROP SCHEMA: " + sanitizeError(err)})
		return
	}
	pool.Reset() // evict stale prepared statements before CREATE SCHEMA
	if _, err := pool.Exec(r.Context(), `CREATE SCHEMA public`); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "CREATE SCHEMA: " + sanitizeError(err)})
		return
	}
	// Safely quote a Postgres identifier: double internal double-quotes.
	safeDBUser := `"` + strings.ReplaceAll(dbUser, `"`, `""`) + `"`
	grantSQL := fmt.Sprintf(`GRANT ALL ON SCHEMA public TO %s`, safeDBUser)
	if _, err := pool.Exec(r.Context(), grantSQL); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "GRANT: " + sanitizeError(err)})
		return
	}

	// Re-create extensions that were dropped with the schema.
	// Must run outside a transaction and before migrations (migration 1 needs vector + pgcrypto).
	for _, ext := range []string{"pgcrypto", "vector", "uuid-ossp"} {
		pool.Exec(r.Context(), fmt.Sprintf(`CREATE EXTENSION IF NOT EXISTS "%s"`, ext))
	}

	migDir := "migrations"
	if err := gw.db.MigrateUpFS(embeddedMigrations, migDir); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "migration: " + sanitizeError(err)})
		return
	}

	home, _ := os.UserHomeDir()
	workspacesDir := filepath.Join(home, ".qorven", "workspaces")
	if _, err := os.Stat(workspacesDir); err == nil {
		os.RemoveAll(workspacesDir)
		os.MkdirAll(workspacesDir, 0o755)
	}

	// Remove installed apps from disk (DB rows were dropped with the schema).
	appsDir := filepath.Join(home, ".qorven", "apps")
	if _, err := os.Stat(appsDir); err == nil {
		os.RemoveAll(appsDir)
	}
	// Clear in-memory app registry so next LoadAll starts fresh.
	if gw.appMgr != nil {
		gw.appMgr.Reset()
	}

	if rp := runtimePath(); rp != "" {
		os.Remove(rp)
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
