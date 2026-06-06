// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

// reset-password — CLI fallback for password recovery when Telegram is not
// paired. Connects directly to the database; does not require the server to
// be running. Works even if you've been fully locked out via the web UI.
//
// Usage:
//   qorven reset-password
//
// The command:
//   1. Connects to the database using the configured DSN.
//   2. Looks up the single admin user.
//   3. Prompts for a new password (no echo).
//   4. Bcrypt-hashes and saves it.
//   5. Revokes all refresh tokens so all existing sessions are invalidated.

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "reset-password",
		Short: "Reset the admin password directly via the database",
		Long: `Reset the admin user password without needing the web UI or Telegram.

Use this command when you have SSH access to the server but cannot log in
via the web UI (forgotten password, Telegram not paired, etc.).

The server does not need to be running. The command connects directly to
the database and updates the password hash. All existing sessions are
invalidated after the reset.

Examples:
  qorven reset-password

Environment (if config file is not found):
  QORVEN_POSTGRES_DSN   PostgreSQL connection string`,
		RunE: runResetPassword,
	})
}

func runResetPassword(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()

	// ── 1. Connect to database ──────────────────────────────────────────────
	serverCfg, err := config.Load("")
	if err != nil {
		// Config file not found is fine — fall through to env var
		serverCfg = nil
	}
	dsn := ""
	if serverCfg != nil {
		dsn = serverCfg.Database.DSN
	}
	if dsn == "" {
		dsn = os.Getenv("QORVEN_POSTGRES_DSN")
	}
	if dsn == "" {
		return fmt.Errorf("no database DSN found — set QORVEN_POSTGRES_DSN or ensure config.toml is readable")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// ── 2. Find the admin user ───────────────────────────────────────────────
	var userID, username string
	err = pool.QueryRow(ctx,
		`SELECT id, username FROM users WHERE role = 'admin' ORDER BY created_at ASC LIMIT 1`,
	).Scan(&userID, &username)
	if err != nil {
		return fmt.Errorf("no admin user found — run 'qorven init' first")
	}

	fmt.Printf("Resetting password for user: %s\n\n", username)

	// ── 3. Prompt for new password (no echo) ────────────────────────────────
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return fmt.Errorf("reset-password must be run from an interactive terminal (TTY required for secure password input)")
	}

	fmt.Print("New password: ")
	pw1, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	if len(pw1) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	fmt.Print("Confirm password: ")
	pw2, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return fmt.Errorf("read password confirmation: %w", err)
	}

	if string(pw1) != string(pw2) {
		return fmt.Errorf("passwords do not match")
	}

	// ── 4. Hash and save ────────────────────────────────────────────────────
	hash, err := bcrypt.GenerateFromPassword(pw1, 12)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = pool.Exec(ctx,
		`UPDATE users SET password_hash = $1, failed_logins = 0, locked_until = NULL WHERE id = $2`,
		string(hash), userID)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	// ── 5. Revoke all existing sessions ─────────────────────────────────────
	ct, _ := pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID)

	fmt.Printf("\n✓ Password updated for '%s'\n", username)
	if ct.RowsAffected() > 0 {
		fmt.Printf("✓ %d active session(s) revoked — all devices must log in again\n", ct.RowsAffected())
	}
	fmt.Println("\nYou can now log in at the web UI with the new password.")
	return nil
}
