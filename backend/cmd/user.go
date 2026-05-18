// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() {
	userCmd := &cobra.Command{
		Use:   "user",
		Short: "Manage user accounts",
	}
	userCmd.AddCommand(userListCmd())
	userCmd.AddCommand(userResetPasswordCmd())
	rootCmd.AddCommand(userCmd)
}

func userListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all user accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _ := config.Load(os.Getenv("QORVEN_CONFIG"))
			if cfg == nil || cfg.Database.DSN == "" {
				return fmt.Errorf("database not configured — run: qorven init")
			}
			ctx := context.Background()
			pool, err := pgxpool.New(ctx, cfg.Database.DSN)
			if err != nil {
				return err
			}
			defer pool.Close()

			rows, err := pool.Query(ctx,
				`SELECT username, email, role, is_active, last_login_at, created_at
				 FROM users ORDER BY created_at`)
			if err != nil {
				return fmt.Errorf("query: %v", err)
			}
			defer rows.Close()

			fmt.Printf("\n  %-20s %-30s %-10s %-8s %s\n", "Username", "Email", "Role", "Active", "Last Login")
			fmt.Println("  " + strings.Repeat("─", 80))

			count := 0
			for rows.Next() {
				var username, role string
				var email *string
				var isActive bool
				var lastLogin, createdAt *string
				if err := rows.Scan(&username, &email, &role, &isActive, &lastLogin, &createdAt); err != nil {
					continue
				}
				emailStr := ""
				if email != nil {
					emailStr = *email
				}
				loginStr := "never"
				if lastLogin != nil {
					loginStr = *lastLogin
					if len(loginStr) > 16 {
						loginStr = loginStr[:16]
					}
				}
				activeStr := "yes"
				if !isActive {
					activeStr = "no"
				}
				fmt.Printf("  %-20s %-30s %-10s %-8s %s\n", username, emailStr, role, activeStr, loginStr)
				count++
			}
			if count == 0 {
				fmt.Println("  No users found.")
			}
			fmt.Println()
			return nil
		},
	}
}

func userResetPasswordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-password <username>",
		Short: "Reset a user's password directly (bypasses OTP flow)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]

			cfg, _ := config.Load(os.Getenv("QORVEN_CONFIG"))
			if cfg == nil || cfg.Database.DSN == "" {
				return fmt.Errorf("database not configured — run: qorven init")
			}
			ctx := context.Background()
			pool, err := pgxpool.New(ctx, cfg.Database.DSN)
			if err != nil {
				return err
			}
			defer pool.Close()

			// Confirm user exists
			var userID string
			err = pool.QueryRow(ctx,
				`SELECT id FROM users WHERE username = $1 LIMIT 1`,
				strings.ToLower(username),
			).Scan(&userID)
			if err != nil {
				return fmt.Errorf("user %q not found", username)
			}

			// Prompt for new password (hidden input)
			fmt.Printf("New password for %q: ", username)
			pw1, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return fmt.Errorf("could not read password: %v", err)
			}
			fmt.Print("Confirm password: ")
			pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return fmt.Errorf("could not read password: %v", err)
			}

			if string(pw1) != string(pw2) {
				return fmt.Errorf("passwords do not match")
			}
			if len(pw1) < 8 {
				return fmt.Errorf("password must be at least 8 characters")
			}

			svc := auth.NewAuthService(pool)
			if err := svc.ResetPassword(ctx, userID, string(pw1)); err != nil {
				return fmt.Errorf("reset failed: %v", err)
			}

			fmt.Printf("  Password updated for user %q.\n\n", username)
			return nil
		},
	}
}
