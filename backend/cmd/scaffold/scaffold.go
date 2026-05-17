// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// Package scaffold writes starter files for Qorven plugin + UI
// projects. Used by `qorven agent init`. No network, no DB — just
// embedded templates rendered into a target directory.
//
// ## Why separate from cmd/
//
// Keeping the rendering engine as a standalone package makes it
// trivially testable without instantiating the full cobra root. A
// test calls scaffold.Render(...) with a t.TempDir() and asserts
// the generated files compile.
package scaffold

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

// embeddedTemplates ships every template file in the binary. The
// embed path mirrors the cmd/scaffold/templates layout on disk.
//
//go:embed templates/plugin/* templates/plugin/** templates/plugin-tinygo/* templates/ui/* templates/ui/**
var embeddedTemplates embed.FS

// Runtime identifies which Wasm build toolchain the emitted plugin
// targets. TinyGo support reduces artifact size from ~3.2 MiB to
// ~50 KiB — useful for multi-tenant deployments.
type Runtime string

const (
	// RuntimeGo emits the standard-Go template (wasip1 native target).
	// No external dependencies beyond Go 1.21+. Bigger binaries,
	// simpler story, full stdlib.
	RuntimeGo Runtime = "go"

	// RuntimeTinyGo emits the TinyGo template. Requires tinygo on
	// PATH at build time. Drops the runtime + GC overhead for ~60×
	// smaller artifacts. Subset of stdlib — see the template's README.
	RuntimeTinyGo Runtime = "tinygo"
)

// Options controls what Render writes out.
type Options struct {
	// Name is the plugin identifier. Must match
	// ^[a-z][a-z0-9_]{0,62}$ — the same regex Qorven's registry
	// uses, so a scaffolded plugin can upload without rename.
	Name string

	// Description shows up in the LLM's tool listing and in the
	// scaffolded README. Keep short — a sentence.
	Description string

	// Plugin, UI toggle which trees to emit. At least one MUST be
	// true. Emitting both is the default when a user passes no
	// flags.
	Plugin bool
	UI     bool

	// Runtime selects the plugin build toolchain. Defaults to
	// RuntimeGo — the zero value — for back-compat with existing
	// callers. Ignored when Plugin=false.
	Runtime Runtime

	// TargetDir is the root under which scaffolded files land.
	// Rendered layout: <TargetDir>/plugin/... and/or <TargetDir>/ui/...
	// The directory is created if missing. Existing files are NOT
	// overwritten — Render returns an error on the first collision
	// so a user doesn't silently clobber in-flight work.
	TargetDir string

	// Computed fields — set by Render(), not by callers. Present in
	// templates as [[.Slug]], [[.SlugCamel]], [[.NamePascal]], [[.NameTitle]].
	Slug       string // "my-app"   (Name with _ → -)
	SlugCamel  string // "myApp"    (Slug with -X → X uppercase)
	NamePascal string // "MyApp"    (Name words joined, title-cased — valid JS identifier)
	NameTitle  string // "My App"   (Name with _ → space, title-cased — for display)
}

// nameRE matches the Qorven registry's plugin-name constraint.
// Keep in sync with backend/migrations/043 CHECK and
// backend/internal/plugins/registry/store.go.
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// SlugFromName converts "my_app" → "my-app".
func SlugFromName(name string) string {
	return strings.ReplaceAll(name, "_", "-")
}

// SlugCamelFromSlug converts "my-app" → "myApp".
func SlugCamelFromSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// NamePascalFromName converts "my_app" → "MyApp".
func NamePascalFromName(name string) string {
	parts := strings.Split(name, "_")
	var result []string
	for _, p := range parts {
		if len(p) > 0 {
			result = append(result, strings.ToUpper(p[:1])+p[1:])
		}
	}
	return strings.Join(result, "")
}

// NameTitleFromName converts "my_app" → "My App".
func NameTitleFromName(name string) string {
	parts := strings.Split(name, "_")
	var result []string
	for _, p := range parts {
		if len(p) > 0 {
			result = append(result, strings.ToUpper(p[:1])+p[1:])
		}
	}
	return strings.Join(result, " ")
}

// Render writes the scaffold. Returns a list of absolute paths that
// were written (useful for the CLI to print a summary) and an error
// on the first failure.
func Render(opts Options) ([]string, error) {
	if !nameRE.MatchString(opts.Name) {
		return nil, fmt.Errorf("scaffold: plugin name %q must match ^[a-z][a-z0-9_]{0,62}$", opts.Name)
	}
	if opts.TargetDir == "" {
		return nil, errors.New("scaffold: TargetDir required")
	}
	if !opts.Plugin && !opts.UI {
		return nil, errors.New("scaffold: enable at least one of Plugin, UI")
	}
	if opts.Description == "" {
		opts.Description = "A scaffolded Qorven Wasm plugin."
	}
	// Normalize / validate runtime. Zero value → Go (back-compat).
	if opts.Runtime == "" {
		opts.Runtime = RuntimeGo
	}
	if opts.Runtime != RuntimeGo && opts.Runtime != RuntimeTinyGo {
		return nil, fmt.Errorf("scaffold: unknown runtime %q (valid: %q, %q)",
			opts.Runtime, RuntimeGo, RuntimeTinyGo)
	}

	// Compute template variables.
	opts.Slug = SlugFromName(opts.Name)
	opts.SlugCamel = SlugCamelFromSlug(opts.Slug)
	opts.NamePascal = NamePascalFromName(opts.Name)
	opts.NameTitle = NameTitleFromName(opts.Name)

	if err := os.MkdirAll(opts.TargetDir, 0o755); err != nil {
		return nil, fmt.Errorf("scaffold: mkdir target: %w", err)
	}

	var written []string
	if opts.Plugin {
		srcDir := "templates/plugin"
		if opts.Runtime == RuntimeTinyGo {
			srcDir = "templates/plugin-tinygo"
		}
		paths, err := renderTree(embeddedTemplates, srcDir, "plugin", opts)
		if err != nil {
			return written, err
		}
		written = append(written, paths...)
	}
	if opts.UI {
		paths, err := renderTree(embeddedTemplates, "templates/ui", "ui", opts)
		if err != nil {
			return written, err
		}
		written = append(written, paths...)
	}
	return written, nil
}

// renderTree walks an embedded subtree and renders each file into
// TargetDir/<subdir>/<relpath-minus-.tmpl>.
func renderTree(efs embed.FS, src, destPrefix string, opts Options) ([]string, error) {
	var written []string
	err := fs.WalkDir(efs, src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		outName := strings.TrimSuffix(rel, ".tmpl")
		outPath := filepath.Join(opts.TargetDir, destPrefix, outName)

		if _, err := os.Stat(outPath); err == nil {
			return fmt.Errorf("scaffold: refusing to overwrite %s — remove it first or pick a fresh target dir", outPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("scaffold: stat %s: %w", outPath, err)
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("scaffold: mkdir %s: %w", filepath.Dir(outPath), err)
		}

		raw, err := efs.ReadFile(path)
		if err != nil {
			return fmt.Errorf("scaffold: read %s: %w", path, err)
		}
		// Use [[ ... ]] delimiters so JSX's own {{ ... }} style
		// bindings pass through verbatim. Every .tmpl file uses
		// [[.FieldName]] for substitution.
		t, err := template.New(rel).Delims("[[", "]]").Parse(string(raw))
		if err != nil {
			return fmt.Errorf("scaffold: parse template %s: %w", path, err)
		}
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("scaffold: create %s: %w", outPath, err)
		}
		if err := t.Execute(f, opts); err != nil {
			_ = f.Close()
			return fmt.Errorf("scaffold: render %s: %w", outPath, err)
		}
		if err := f.Close(); err != nil {
			return err
		}
		written = append(written, outPath)
		return nil
	})
	return written, err
}
