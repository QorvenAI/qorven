// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package apps

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9\-]{0,62}$`)

// LoadManifest reads and validates app.yaml from dir.
func LoadManifest(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "app.yaml"))
	if err != nil {
		return Manifest{}, fmt.Errorf("no app.yaml in %s: %w", filepath.Base(dir), err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("invalid app.yaml: %w", err)
	}
	if !slugRe.MatchString(m.Slug) {
		return Manifest{}, fmt.Errorf("invalid slug %q: must match ^[a-z][a-z0-9\\-]{0,62}$", m.Slug)
	}
	if m.DisplayName == "" {
		return Manifest{}, fmt.Errorf("app.yaml missing display_name")
	}
	if m.Version == "" {
		m.Version = "0.0.0"
	}
	if m.MigrationsDir == "" {
		m.MigrationsDir = "migrations"
	}
	if m.Frontend.Bundle == "" {
		m.Frontend.Bundle = "frontend/bundle.js"
	}
	// Validate permissions against allowlist — reject unknown entries so
	// app authors know immediately if they misspelled a permission.
	for _, p := range m.Permissions {
		if !validPermissions[p] {
			return Manifest{}, fmt.Errorf("unknown permission %q in app.yaml", p)
		}
	}
	return m, nil
}

// CheckRequiresEnv returns an error if any required env var is missing.
func CheckRequiresEnv(m Manifest) error {
	for _, env := range m.RequiresEnv {
		if os.Getenv(env) == "" {
			return fmt.Errorf("app %q requires env var %s (not set)", m.Slug, env)
		}
	}
	return nil
}

// HasPermission reports whether the manifest declares a given permission.
func HasPermission(m Manifest, perm string) bool {
	for _, p := range m.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}
