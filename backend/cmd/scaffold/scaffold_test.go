// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package scaffold_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/cmd/scaffold"
)

func TestRender_EmitsPluginAndUITrees(t *testing.T) {
	dir := t.TempDir()
	written, err := scaffold.Render(scaffold.Options{
		Name:        "my_tool",
		Description: "does a thing",
		Plugin:      true,
		UI:          true,
		TargetDir:   dir,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(written) == 0 {
		t.Fatalf("no files written")
	}

	// Core plugin files must exist.
	for _, rel := range []string{
		"plugin/plugin.go",
		"plugin/go.mod",
		"plugin/parameters.json",
		"plugin/Makefile",
		"plugin/README.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}

	// UI files.
	for _, rel := range []string{
		"ui/package.json",
		"ui/tsconfig.json",
		"ui/vite.config.ts",
		"ui/src/index.tsx",
		"ui/qorven-app.d.ts",
		"ui/README.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}

	// Template substitution: plugin name should appear verbatim in
	// generated Go source.
	src, err := os.ReadFile(filepath.Join(dir, "plugin/plugin.go"))
	if err != nil {
		t.Fatalf("read plugin.go: %v", err)
	}
	if !strings.Contains(string(src), "my_tool") {
		t.Errorf("plugin.go did not substitute name: %s", src)
	}
	// Description should land in parameters.json.
	params, _ := os.ReadFile(filepath.Join(dir, "plugin/parameters.json"))
	if !strings.Contains(string(params), "does a thing") {
		t.Errorf("parameters.json did not substitute description: %s", params)
	}

	// Vite config must contain the IIFE name derived from SlugCamel.
	// Name "my_tool" → Slug "my-tool" → SlugCamel "myTool"
	viteCfg, err := os.ReadFile(filepath.Join(dir, "ui/vite.config.ts"))
	if err != nil {
		t.Fatalf("read vite.config.ts: %v", err)
	}
	if !strings.Contains(string(viteCfg), "__app_myTool") {
		t.Errorf("vite.config.ts did not substitute SlugCamel — want __app_myTool in:\n%s", viteCfg)
	}

	// index.tsx must contain NamePascal (component name).
	// Name "my_tool" → NamePascal "MyTool"
	idx, err := os.ReadFile(filepath.Join(dir, "ui/src/index.tsx"))
	if err != nil {
		t.Fatalf("read src/index.tsx: %v", err)
	}
	if !strings.Contains(string(idx), "MyTool") {
		t.Errorf("src/index.tsx did not substitute NamePascal — want MyTool in:\n%s", idx)
	}
}

// TestRender_RejectsInvalidName protects the single invariant the
// target-side registry relies on: a scaffolded plugin must be
// uploadable without rename. If the scaffold emitted an invalid
// name, `make upload` would fail with ErrInvalidName.
func TestRender_RejectsInvalidName(t *testing.T) {
	bad := []string{"", "1digit", "Has-Dash", "UPPER", strings.Repeat("x", 64)}
	for _, name := range bad {
		_, err := scaffold.Render(scaffold.Options{
			Name: name, Plugin: true, TargetDir: t.TempDir(),
		})
		if err == nil {
			t.Errorf("Render(name=%q) returned nil error", name)
		}
	}
}

func TestRender_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Render(scaffold.Options{
		Name: "t", Plugin: true, UI: false, TargetDir: dir,
	}); err != nil {
		t.Fatalf("first render: %v", err)
	}

	// Second render MUST fail because plugin/plugin.go already exists.
	_, err := scaffold.Render(scaffold.Options{
		Name: "t", Plugin: true, UI: false, TargetDir: dir,
	})
	if err == nil {
		t.Fatalf("second render did not error — scaffold would silently clobber user work")
	}
	if !strings.Contains(err.Error(), "overwrite") {
		t.Errorf("error does not mention overwrite: %v", err)
	}
}

// TestRender_PluginCompilesToWasip1 is the golden test: the
// scaffolded Go source must compile as-is under the real
// GOOS=wasip1 GOARCH=wasm target. If the template drifts and
// stops compiling, every new plugin author hits the error — so
// we catch it here.
//
// Skips if GOOS=wasip1 support is unavailable (Go < 1.21 on the
// build host). CI must have Go >= 1.21, which we already require
// for the main build.
func TestRender_PluginCompilesToWasip1(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain missing: %v", err)
	}

	dir := t.TempDir()
	if _, err := scaffold.Render(scaffold.Options{
		Name: "scaffold_compile_test", Description: "x",
		Plugin: true, UI: false, TargetDir: dir,
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	pluginDir := filepath.Join(dir, "plugin")
	// `go mod tidy` first so the generated go.mod resolves std-only
	// deps cleanly. The scaffold only imports encoding/json and
	// io/os, so tidy is a fast no-op.
	cmd := exec.Command("go", "build", "-o", "out.wasm", ".")
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build wasip1 failed: %v\n%s", err, out)
	}
	// Artifact must exist + be non-empty.
	info, err := os.Stat(filepath.Join(pluginDir, "out.wasm"))
	if err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("artifact is empty")
	}
	// A complete Go-wasip1 binary is ~3 MiB. Flag wildly off sizes
	// (sub-KB = template broken; >20 MiB = accidentally embedded
	// something huge).
	if info.Size() < 100_000 || info.Size() > 20<<20 {
		t.Errorf("artifact size %d bytes looks wrong", info.Size())
	}
}

func TestRender_PluginOnlyFlag(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Render(scaffold.Options{
		Name: "t", Plugin: true, UI: false, TargetDir: dir,
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "plugin")); err != nil {
		t.Errorf("plugin tree missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "ui")); !os.IsNotExist(err) {
		t.Errorf("ui tree should NOT exist with UI=false, got stat err=%v", err)
	}
}

// TestRender_TinyGoRuntime — the --runtime=tinygo path emits the
// TinyGo template (different Makefile + README) with the
// same Go source shape. Does NOT require tinygo installed — we only
// verify the files land and the source mentions the expected imports.
// The separate TestRender_TinyGoCompiles test runs only when tinygo
// is on PATH.
func TestRender_TinyGoRuntime(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Render(scaffold.Options{
		Name:        "tg_plugin",
		Description: "tiny thing",
		Plugin:      true,
		UI:          false,
		Runtime:     scaffold.RuntimeTinyGo,
		TargetDir:   dir,
	}); err != nil {
		t.Fatalf("Render(tinygo): %v", err)
	}

	// The Makefile must mention tinygo; the standard-go Makefile
	// does NOT. This is the tripwire for "we accidentally served the
	// go template under the tinygo runtime".
	mk, err := os.ReadFile(filepath.Join(dir, "plugin/Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	if !strings.Contains(string(mk), "tinygo") {
		t.Errorf("TinyGo Makefile does not mention tinygo:\n%s", mk)
	}

	src, _ := os.ReadFile(filepath.Join(dir, "plugin/plugin.go"))
	if !strings.Contains(string(src), "encoding/json") {
		t.Errorf("TinyGo plugin source missing expected json import:\n%s", src)
	}
}

// TestRender_TinyGoCompiles is the compile gate — runs only when
// tinygo is on PATH. Produces a .wasm and asserts it's under 200 KiB
// (the headline size benefit). Skip cleanly otherwise so CI without
// tinygo doesn't fail — TestRender_PluginCompilesToWasip1 covers
// the default runtime.
func TestRender_TinyGoCompiles(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		t.Skipf("tinygo not installed: %v; install from https://tinygo.org", err)
	}

	dir := t.TempDir()
	if _, err := scaffold.Render(scaffold.Options{
		Name: "tg_compile", Description: "x",
		Plugin: true, UI: false,
		Runtime:   scaffold.RuntimeTinyGo,
		TargetDir: dir,
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	pluginDir := filepath.Join(dir, "plugin")
	cmd := exec.Command("tinygo", "build",
		"-target", "wasi-p1",
		"-no-debug",
		"-scheduler=none",
		"-gc=leaking",
		"-o", "out.wasm", ".")
	cmd.Dir = pluginDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tinygo build failed: %v\n%s", err, out)
	}
	info, err := os.Stat(filepath.Join(pluginDir, "out.wasm"))
	if err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	// TinyGo's whole pitch is size. Exceeding 200 KiB suggests GC/
	// runtime isn't being elided, or an import dragged in
	// fmt/reflect. Either is a regression worth catching.
	if info.Size() > 200_000 {
		t.Errorf("tinygo artifact %d bytes exceeds 200 KiB — size regression", info.Size())
	}
	if info.Size() < 1000 {
		t.Errorf("tinygo artifact %d bytes is suspiciously tiny — empty binary?", info.Size())
	}
}

func TestRender_RejectsUnknownRuntime(t *testing.T) {
	_, err := scaffold.Render(scaffold.Options{
		Name: "x", Plugin: true, TargetDir: t.TempDir(),
		Runtime: "rustforges",
	})
	if err == nil {
		t.Fatalf("unknown runtime should error")
	}
	if !strings.Contains(err.Error(), "unknown runtime") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestRender_UIOnlyFlag(t *testing.T) {
	dir := t.TempDir()
	if _, err := scaffold.Render(scaffold.Options{
		Name: "t", Plugin: false, UI: true, TargetDir: dir,
	}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ui")); err != nil {
		t.Errorf("ui tree missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "plugin")); !os.IsNotExist(err) {
		t.Errorf("plugin tree should NOT exist with Plugin=false")
	}
}

func TestRender_UITemplateVariables(t *testing.T) {
	if got := scaffold.SlugFromName("my_app"); got != "my-app" {
		t.Errorf("SlugFromName: got %q want %q", got, "my-app")
	}
	if got := scaffold.SlugCamelFromSlug("my-app"); got != "myApp" {
		t.Errorf("SlugCamelFromSlug: got %q want %q", got, "myApp")
	}
	if got := scaffold.NamePascalFromName("my_app"); got != "MyApp" {
		t.Errorf("NamePascalFromName: got %q want %q", got, "MyApp")
	}
	if got := scaffold.NameTitleFromName("my_app"); got != "My App" {
		t.Errorf("NameTitleFromName: got %q want %q", got, "My App")
	}
	if s := scaffold.SlugCamelFromSlug("my-long-app"); s != "myLongApp" {
		t.Errorf("SlugCamelFromSlug multi-word: got %q", s)
	}
	if s := scaffold.NamePascalFromName("my_long_app"); s != "MyLongApp" {
		t.Errorf("NamePascalFromName multi-word: got %q", s)
	}
	if s := scaffold.SlugFromName("tool"); s != "tool" {
		t.Errorf("SlugFromName single: got %q", s)
	}
}

func TestTemplateHelpers_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string) string
		in   string
		want string
	}{
		{"SlugFromName empty", scaffold.SlugFromName, "", ""},
		{"NamePascalFromName empty", scaffold.NamePascalFromName, "", ""},
		{"NameTitleFromName empty", scaffold.NameTitleFromName, "", ""},
		{"NamePascalFromName double underscore", scaffold.NamePascalFromName, "my__app", "MyApp"},
		{"NameTitleFromName double underscore", scaffold.NameTitleFromName, "my__app", "My App"},
		{"NamePascalFromName leading underscore", scaffold.NamePascalFromName, "_app", "App"},
		{"NameTitleFromName trailing underscore", scaffold.NameTitleFromName, "app_", "App"},
		{"SlugCamelFromSlug single char", scaffold.SlugCamelFromSlug, "a", "a"},
		{"NamePascalFromName single char", scaffold.NamePascalFromName, "a", "A"},
	}
	for _, tt := range tests {
		if got := tt.fn(tt.in); got != tt.want {
			t.Errorf("%s(%q): got %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}
