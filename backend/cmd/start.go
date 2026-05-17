// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/internal/config"
	"github.com/qorvenai/qorven/internal/gateway"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the Qorven gateway server",
		RunE: func(cmd *cobra.Command, args []string) error {
			serverCfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("config: %w", err)
			}

			if cfg != nil && cfg.Verbose {
				slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
			}

			gateway.SetBuildInfo(Version, Commit, BuildTime)
			if exe, err2 := os.Executable(); err2 == nil {
				for _, p := range []string{
					filepath.Join(filepath.Dir(exe), "CHANGELOG.md"),
					filepath.Join(filepath.Dir(exe), "..", "CHANGELOG.md"),
					"CHANGELOG.md",
				} {
					if b, err3 := os.ReadFile(p); err3 == nil {
						gateway.SetChangelog(string(b))
						break
					}
				}
			}
			gw, err := gateway.New(serverCfg)
			if err != nil {
				return fmt.Errorf("gateway: %w", err)
			}

			stop := make(chan os.Signal, 1)
			signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				slog.Info("qorven starting", "version", Version, "addr", serverCfg.Server.Listen)
				if err := gw.Start(); err != nil {
					slog.Error("gateway failed", "error", err)
					os.Exit(1)
				}
			}()

			<-stop
			slog.Info("shutting down...")
			gw.Stop()
			return nil
		},
	})
}
