// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/client"
	"github.com/qorvenai/qorven/cmd/output"
	"github.com/qorvenai/qorven/cmd/tui"
	"gopkg.in/yaml.v3"
)

// Profile represents a saved server connection.
type Profile struct {
	Name   string `yaml:"name"`
	Server string `yaml:"server"`
}

type profilesFile struct {
	Active   string    `yaml:"active"`
	Profiles []Profile `yaml:"profiles"`
}

func profilesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".qorven")
}

func profilesPath() string {
	return filepath.Join(profilesDir(), "profiles.yaml")
}

func loadProfiles() (*profilesFile, error) {
	data, err := os.ReadFile(profilesPath())
	if err != nil {
		return &profilesFile{}, nil
	}
	var pf profilesFile
	yaml.Unmarshal(data, &pf)
	return &pf, nil
}

func saveProfiles(pf *profilesFile) error {
	os.MkdirAll(profilesDir(), 0700)
	data, _ := yaml.Marshal(pf)
	return os.WriteFile(profilesPath(), data, 0600)
}

func saveToken(profile, token string) error {
	os.MkdirAll(profilesDir(), 0700)
	return os.WriteFile(filepath.Join(profilesDir(), "token_"+profile), []byte(token), 0600)
}

func loadToken(profile string) string {
	data, err := os.ReadFile(filepath.Join(profilesDir(), "token_"+profile))
	if err != nil {
		return ""
	}
	return string(data)
}

// --- Commands ---

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with a Qorven gateway",
	RunE: func(cmd *cobra.Command, args []string) error {
		server := cfg.Server
		token := cfg.Token
		profileName, _ := cmd.Flags().GetString("profile")
		if profileName == "" {
			profileName = "default"
		}

		// Interactive prompts
		if server == "" && tui.IsInteractive() {
			server = tui.Input("Gateway URL", "http://localhost")
		}
		if server == "" {
			return client.ErrServerRequired
		}
		if token == "" && tui.IsInteractive() {
			// Try username/password login first
			username := tui.Input("Username", "admin")
			pw, err := tui.Password("Password")
			if err != nil {
				return err
			}
			// POST /auth/login to get JWT
			loginResp, loginErr := client.PostJSON(server+"/auth/login", map[string]string{"username": username, "password": pw})
			if loginErr == nil && loginResp["token"] != "" {
				token = loginResp["token"]
			} else {
				// Fallback to raw token
				fmt.Println("Username/password login failed, enter token directly:")
				token, err = tui.Password("Gateway token")
				if err != nil {
					return err
				}
			}
		}
		if token == "" {
			return fmt.Errorf("token required - use --token or QORVEN_TOKEN")
		}

		// Verify connection
		c := client.NewHTTPClient(server, token, false)
		if err := c.HealthCheck(); err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		// Save profile
		pf, _ := loadProfiles()
		found := false
		for i, p := range pf.Profiles {
			if p.Name == profileName {
				pf.Profiles[i].Server = server
				found = true
				break
			}
		}
		if !found {
			pf.Profiles = append(pf.Profiles, Profile{Name: profileName, Server: server})
		}
		pf.Active = profileName
		if err := saveProfiles(pf); err != nil {
			return fmt.Errorf("save profile: %w", err)
		}
		if err := saveToken(profileName, token); err != nil {
			return fmt.Errorf("save token: %w", err)
		}

		printer.Success(fmt.Sprintf("Logged in to %s (profile: %s)", server, profileName))
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored credentials",
	RunE: func(cmd *cobra.Command, args []string) error {
		profileName, _ := cmd.Flags().GetString("profile")
		if profileName == "" {
			profileName = "default"
		}
		os.Remove(filepath.Join(profilesDir(), "token_"+profileName))
		printer.Success(fmt.Sprintf("Logged out from profile %q", profileName))
		return nil
	},
}

var authWhoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current authentication info",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.Server == "" {
			return client.ErrServerRequired
		}
		if cfg.Token == "" {
			return client.ErrNotAuthenticated
		}
		c := client.NewHTTPClient(cfg.Server, cfg.Token, false)
		err := c.HealthCheck()
		status := "authenticated"
		if err != nil {
			status = "connection failed: " + err.Error()
		}

		tbl := output.NewTable("Field", "Value")
		tbl.AddRow("Server", cfg.Server)
		tbl.AddRow("Status", status)
		printer.Print(tbl)
		return nil
	},
}

var authListContextsCmd = &cobra.Command{
	Use:   "list-contexts",
	Short: "List configured profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		pf, _ := loadProfiles()
		if len(pf.Profiles) == 0 {
			printer.Success("No profiles configured. Run 'qorven auth login' to add one.")
			return nil
		}
		tbl := output.NewTable("ACTIVE", "NAME", "SERVER")
		for _, p := range pf.Profiles {
			marker := ""
			if p.Name == pf.Active {
				marker = "*"
			}
			tbl.AddRow(marker, p.Name, p.Server)
		}
		printer.Print(tbl)
		return nil
	},
}

var authUseContextCmd = &cobra.Command{
	Use:   "use-context <profile>",
	Short: "Switch active profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pf, _ := loadProfiles()
		for _, p := range pf.Profiles {
			if p.Name == args[0] {
				pf.Active = args[0]
				if err := saveProfiles(pf); err != nil {
					return err
				}
				printer.Success(fmt.Sprintf("Switched to profile %q", args[0]))
				return nil
			}
		}
		return fmt.Errorf("profile %q not found", args[0])
	},
}

func init() {
	authLoginCmd.Flags().String("profile", "default", "Profile name")
	authLogoutCmd.Flags().String("profile", "default", "Profile name")
	authCmd.AddCommand(authLoginCmd, authLogoutCmd, authWhoamiCmd, authListContextsCmd, authUseContextCmd)
	rootCmd.AddCommand(authCmd)
}
