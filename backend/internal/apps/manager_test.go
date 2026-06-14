//go:build unit

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package apps

import (
	"context"
	"errors"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// --- helpers -----------------------------------------------------------------

// toolRegistrar is the test-local interface (matches the interface we'll add to manager.go).
// fakeToolReg captures registered tools.
type fakeToolReg struct {
	tools map[string]tools.Tool
}

func (f *fakeToolReg) Register(t tools.Tool) {
	if f.tools == nil {
		f.tools = make(map[string]tools.Tool)
	}
	f.tools[t.Name()] = t
}

func (f *fakeToolReg) Unregister(name string) { delete(f.tools, name) }

// newTestManager returns a minimal manager with no pool.
func newTestManager(reg *fakeToolReg) *AppManager {
	return &AppManager{
		toolReg: reg,
		loaded:  make(map[string]*loadedApp),
	}
}

func testManifest(toolCmd string, timeout int) Manifest {
	return Manifest{
		Slug:        "test-app",
		Permissions: []string{"tool_register"},
		Tools: []ToolDef{{
			Name:    "my_tool",
			Command: toolCmd,
			Timeout: timeout,
		}},
	}
}

func testApp() App {
	return App{ID: "app-1", TenantID: "tenant-1", Slug: "test-app", InstallPath: "/tmp"}
}

// parseURL is a thin wrapper so tests don't need to import net/url directly.
func parseURL(s string) (*url.URL, error) { return url.Parse(s) }

// --- tests -------------------------------------------------------------------

func TestRegisterTools_StructuredOutput_Text(t *testing.T) {
	reg := &fakeToolReg{}
	m := newTestManager(reg)
	m.registerTools(testApp(), testManifest("echo hello", 0))

	tool := reg.tools["my_tool"]
	if tool == nil {
		t.Fatal("tool not registered")
	}
	result := tool.Execute(context.Background(), nil)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "hello") {
		t.Errorf("expected 'hello' in ForLLM, got: %q", result.ForLLM)
	}
}

func TestRegisterTools_StructuredOutput_JSON(t *testing.T) {
	scriptPath := t.TempDir() + "/tool.sh"
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '#!qorven:json\n{"text":"llm says hi","user":"user sees hi"}'
`), 0755)

	reg := &fakeToolReg{}
	m := newTestManager(reg)
	m.registerTools(testApp(), testManifest(scriptPath, 0))

	tool := reg.tools["my_tool"]
	result := tool.Execute(context.Background(), nil)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if result.ForLLM != "llm says hi" {
		t.Errorf("ForLLM=%q, want 'llm says hi'", result.ForLLM)
	}
	if result.ForUser != "user sees hi" {
		t.Errorf("ForUser=%q, want 'user sees hi'", result.ForUser)
	}
}

func TestRegisterTools_StructuredOutput_JSON_Widget(t *testing.T) {
	scriptPath := t.TempDir() + "/tool.sh"
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '#!qorven:json\n{"text":"result","widget":{"type":"table","data":{"rows":[]}}}'
`), 0755)

	reg := &fakeToolReg{}
	m := newTestManager(reg)
	m.registerTools(testApp(), testManifest(scriptPath, 0))

	tool := reg.tools["my_tool"]
	result := tool.Execute(context.Background(), nil)
	if result.Widget == nil {
		t.Fatal("expected Widget to be set")
	}
	if result.Widget.Type != "table" {
		t.Errorf("Widget.Type=%q, want 'table'", result.Widget.Type)
	}
}

func TestRegisterTools_StructuredOutput_JSON_Invalid_FallsBack(t *testing.T) {
	scriptPath := t.TempDir() + "/tool.sh"
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '#!qorven:json\nnot valid json'
`), 0755)

	reg := &fakeToolReg{}
	m := newTestManager(reg)
	m.registerTools(testApp(), testManifest(scriptPath, 0))

	tool := reg.tools["my_tool"]
	result := tool.Execute(context.Background(), nil)
	if result.IsError {
		t.Errorf("expected fallback to TextResult (not error) on malformed JSON")
	}
}

func TestRegisterTools_Timeout_Enforced(t *testing.T) {
	scriptPath := t.TempDir() + "/slow.sh"
	os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 5\necho done"), 0755)

	reg := &fakeToolReg{}
	m := newTestManager(reg)
	m.registerTools(testApp(), testManifest(scriptPath, 1)) // 1 second timeout

	start := time.Now()
	tool := reg.tools["my_tool"]
	result := tool.Execute(context.Background(), nil)
	elapsed := time.Since(start)

	if elapsed >= 4*time.Second {
		t.Errorf("timeout not enforced: elapsed %v", elapsed)
	}
	if !result.IsError {
		t.Error("expected IsError=true on timeout")
	}
}

func TestRegisterTools_DSN_Injected(t *testing.T) {
	scriptPath := t.TempDir() + "/env.sh"
	os.WriteFile(scriptPath, []byte("#!/bin/sh\necho \"DSN=$QORVEN_DB_DSN\""), 0755)

	reg := &fakeToolReg{}
	m := &AppManager{
		toolReg: reg,
		loaded:  make(map[string]*loadedApp),
		// appDSN (restricted role) is injected into subprocesses; dsn (superuser) is NOT.
		appDSN: "postgres://qorven_app:apppass@localhost/db",
	}
	m.registerTools(testApp(), testManifest(scriptPath, 0))

	tool := reg.tools["my_tool"]
	result := tool.Execute(context.Background(), nil)
	// The injected DSN must be the restricted app DSN, not the superuser one.
	if !strings.Contains(result.ForLLM, "postgres://qorven_app:apppass@localhost/db") {
		t.Errorf("app DSN not injected, got: %q", result.ForLLM)
	}
}

func TestRegisterTools_DSN_Withheld_WhenEmpty(t *testing.T) {
	scriptPath := t.TempDir() + "/env.sh"
	os.WriteFile(scriptPath, []byte("#!/bin/sh\necho \"DSN=$QORVEN_DB_DSN\""), 0755)

	reg := &fakeToolReg{}
	m := &AppManager{
		toolReg: reg,
		loaded:  make(map[string]*loadedApp),
		// appDSN empty — no DB DSN should appear in subprocess env.
		appDSN: "",
	}
	m.registerTools(testApp(), testManifest(scriptPath, 0))

	tool := reg.tools["my_tool"]
	result := tool.Execute(context.Background(), nil)
	// QORVEN_DB_DSN must not be set — output should show empty value.
	if strings.Contains(result.ForLLM, "postgres://") {
		t.Errorf("unexpected DSN in subprocess env when appDSN is empty, got: %q", result.ForLLM)
	}
}

// --- RunTool unit tests ---

func TestRunTool_NotFound_App(t *testing.T) {
	reg := &fakeToolReg{}
	m := newTestManager(reg)
	// No apps loaded — any slug should return ErrAppNotLoaded
	_, err := m.RunTool(context.Background(), "nonexistent-app", "any_tool", nil)
	if err == nil {
		t.Fatal("expected error for unknown app slug")
	}
	if !errors.Is(err, ErrAppNotLoaded) {
		t.Errorf("expected ErrAppNotLoaded, got: %v", err)
	}
}

func TestRunTool_NotFound_Tool(t *testing.T) {
	reg := &fakeToolReg{}
	m := &AppManager{
		toolReg: reg,
		loaded:  make(map[string]*loadedApp),
	}
	// Manually insert a loaded app with no tools
	m.loaded["my-app"] = &loadedApp{
		app: App{ID: "a1", TenantID: "t1", Slug: "my-app", Enabled: true, InstallPath: "/tmp"},
		manifest: Manifest{
			Slug:        "my-app",
			Permissions: []string{"tool_register"},
			Tools:       []ToolDef{},
		},
	}
	_, err := m.RunTool(context.Background(), "my-app", "missing_tool", nil)
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got: %v", err)
	}
}

func TestRunTool_StructuredOutput(t *testing.T) {
	scriptPath := t.TempDir() + "/tool.sh"
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '#!qorven:json\n{"text":"from RunTool","user":"user text"}'
`), 0755)

	reg := &fakeToolReg{}
	m := &AppManager{
		toolReg: reg,
		loaded:  make(map[string]*loadedApp),
	}
	m.loaded["my-app"] = &loadedApp{
		app: App{ID: "a1", TenantID: "t1", Slug: "my-app", Enabled: true, InstallPath: "/tmp"},
		manifest: Manifest{
			Slug:        "my-app",
			Permissions: []string{"tool_register"},
			Tools: []ToolDef{{
				Name:    "my_tool",
				Command: scriptPath,
			}},
		},
	}

	result, err := m.RunTool(context.Background(), "my-app", "my_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ForLLM != "from RunTool" {
		t.Errorf("ForLLM=%q, want 'from RunTool'", result.ForLLM)
	}
	if result.ForUser != "user text" {
		t.Errorf("ForUser=%q, want 'user text'", result.ForUser)
	}
}

func TestRunTool_Timeout(t *testing.T) {
	scriptPath := t.TempDir() + "/slow.sh"
	os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 5"), 0755)

	reg := &fakeToolReg{}
	m := &AppManager{
		toolReg: reg,
		loaded:  make(map[string]*loadedApp),
	}
	m.loaded["my-app"] = &loadedApp{
		app: App{ID: "a1", TenantID: "t1", Slug: "my-app", Enabled: true, InstallPath: "/tmp"},
		manifest: Manifest{
			Slug:        "my-app",
			Permissions: []string{"tool_register"},
			Tools: []ToolDef{{
				Name:    "slow_tool",
				Command: scriptPath,
				Timeout: 1, // 1 second
			}},
		},
	}

	start := time.Now()
	result, err := m.RunTool(context.Background(), "my-app", "slow_tool", nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if elapsed >= 4*time.Second {
		t.Errorf("timeout not enforced: elapsed %v", elapsed)
	}
	if !result.IsError {
		t.Error("expected IsError=true on timeout")
	}
}

// --- buildAppDSN pure-function tests (no DB required) ---

func TestBuildAppDSN_SwapsUserAndPassword(t *testing.T) {
	superDSN := "postgres://super:supersecret@localhost:5432/qorven_dev"
	appDSN, err := buildAppDSN(superDSN, "apppassword")
	if err != nil {
		t.Fatalf("buildAppDSN error: %v", err)
	}
	// Must contain qorven_app as the user.
	if !strings.Contains(appDSN, "qorven_app") {
		t.Errorf("expected qorven_app user in DSN, got: %q", appDSN)
	}
	// Must contain the app password.
	if !strings.Contains(appDSN, "apppassword") {
		t.Errorf("expected app password in DSN, got: %q", appDSN)
	}
	// Must NOT contain the superuser password.
	if strings.Contains(appDSN, "supersecret") {
		t.Errorf("superuser password must not appear in app DSN, got: %q", appDSN)
	}
	// Must NOT be equal to the superuser DSN.
	if appDSN == superDSN {
		t.Errorf("app DSN must differ from superuser DSN, got: %q", appDSN)
	}
	// Host, port, and database must be preserved.
	if !strings.Contains(appDSN, "localhost:5432") {
		t.Errorf("expected host:port in DSN, got: %q", appDSN)
	}
	if !strings.Contains(appDSN, "qorven_dev") {
		t.Errorf("expected database name in DSN, got: %q", appDSN)
	}
}

func TestBuildAppDSN_SpecialCharsInPassword(t *testing.T) {
	superDSN := "postgres://super:supersecret@localhost:5432/qorven_dev"
	// Password contains characters that must be percent-encoded in a URL.
	appDSN, err := buildAppDSN(superDSN, "p@ss!w0rd#%")
	if err != nil {
		t.Fatalf("buildAppDSN error: %v", err)
	}
	// The raw password must not appear literally (it must be percent-encoded).
	if strings.Contains(appDSN, "p@ss!w0rd#%") {
		t.Errorf("password should be percent-encoded in DSN, got: %q", appDSN)
	}
	// The DSN must still be parseable and round-trip back to the original password.
	u, err2 := parseURL(appDSN)
	if err2 != nil {
		t.Fatalf("resulting DSN not parseable: %v", err2)
	}
	pw, _ := u.User.Password()
	if pw != "p@ss!w0rd#%" {
		t.Errorf("password round-trip: got %q, want %q", pw, "p@ss!w0rd#%")
	}
}

func TestBuildAppDSN_InvalidScheme(t *testing.T) {
	_, err := buildAppDSN("mysql://root:pass@localhost/db", "pw")
	if err == nil {
		t.Error("expected error for non-postgres scheme")
	}
}

// TestBuildAppDSN_SocketURL verifies that a socket URL with a user= query param
// (the default prod DSN form) is correctly rewritten so qorven_app wins.
func TestBuildAppDSN_SocketURL_QueryParamUser(t *testing.T) {
	superDSN := "postgres:///qorven?host=/var/run/postgresql&user=qorven&sslmode=disable"
	appDSN, err := buildAppDSN(superDSN, "apppassword")
	if err != nil {
		t.Fatalf("buildAppDSN error: %v", err)
	}
	// The user= query param must be gone — it would override the URL userinfo.
	if strings.Contains(appDSN, "user=qorven") {
		t.Errorf("old user= query param must be stripped, got: %q", appDSN)
	}
	// qorven_app must appear in the result (as URL userinfo).
	if !strings.Contains(appDSN, "qorven_app") {
		t.Errorf("expected qorven_app in DSN, got: %q", appDSN)
	}
	// Must NOT contain the superuser name 'qorven' as a bare user.
	// (it may still appear in the path /qorven or host, but not as a user= param)
	u, parseErr := parseURL(appDSN)
	if parseErr != nil {
		t.Fatalf("result not parseable: %v", parseErr)
	}
	if u.User.Username() != appRoleName {
		t.Errorf("URL userinfo username = %q, want %q", u.User.Username(), appRoleName)
	}
	pw, _ := u.User.Password()
	if pw != "apppassword" {
		t.Errorf("URL userinfo password = %q, want 'apppassword'", pw)
	}
	// user= query param must be absent from the query string.
	if u.Query().Get("user") != "" {
		t.Errorf("user= query param must be absent, found: %q", u.Query().Get("user"))
	}
	// password= query param must be absent too.
	if u.Query().Get("password") != "" {
		t.Errorf("password= query param must be absent, found: %q", u.Query().Get("password"))
	}
	// sslmode and host params must still be preserved.
	if u.Query().Get("sslmode") != "disable" {
		t.Errorf("sslmode must be preserved, got: %q", u.Query().Get("sslmode"))
	}
}

// TestBuildAppDSN_KeywordDSN verifies that a libpq keyword DSN is correctly
// rewritten so qorven_app is the connecting user.
func TestBuildAppDSN_KeywordDSN(t *testing.T) {
	superDSN := "host=/var/run/postgresql user=qorven dbname=qorven sslmode=disable"
	appDSN, err := buildAppDSN(superDSN, "apppassword")
	if err != nil {
		t.Fatalf("buildAppDSN error: %v", err)
	}
	// user=qorven_app must be present.
	if !strings.Contains(appDSN, "user=qorven_app") {
		t.Errorf("expected user=qorven_app in keyword DSN, got: %q", appDSN)
	}
	// user=qorven (original superuser) must not remain.
	// Note: "qorven" appears in dbname=qorven too, so check specifically for "user=qorven".
	// After rewrite, user=qorven becomes user=qorven_app, so the standalone "user=qorven"
	// (without the _app suffix) must not exist.
	if regexp.MustCompile(`\buser=qorven\b`).MatchString(appDSN) {
		t.Errorf("old user=qorven must be gone, got: %q", appDSN)
	}
	// host and dbname must be preserved.
	if !strings.Contains(appDSN, "host=/var/run/postgresql") {
		t.Errorf("host must be preserved, got: %q", appDSN)
	}
	if !strings.Contains(appDSN, "dbname=qorven") {
		t.Errorf("dbname must be preserved, got: %q", appDSN)
	}
}

// TestDbNameFromConnStr_SocketURL verifies that dbNameFromConnStr correctly
// extracts the database name from a socket URL (no path-based dbname).
func TestDbNameFromConnStr_SocketURL(t *testing.T) {
	// The prod default DSN puts the dbname in the URL path and also uses dbname= in params.
	cases := []struct {
		connStr string
		want    string
	}{
		{"postgres:///qorven?host=/var/run/postgresql&user=qorven&sslmode=disable", "qorven"},
		{"postgres:///qorven_dev?host=/tmp&user=dev", "qorven_dev"},
		{"host=/var/run/postgresql user=qorven dbname=qorven_prod sslmode=disable", "qorven_prod"},
	}
	for _, tc := range cases {
		got := dbNameFromConnStr(tc.connStr)
		if got != tc.want {
			t.Errorf("dbNameFromConnStr(%q) = %q, want %q", tc.connStr, got, tc.want)
		}
	}
}

func TestDbNameFromConnStr(t *testing.T) {
	cases := []struct {
		connStr string
		want    string
	}{
		{"postgres://u:p@host:5432/mydb", "mydb"},
		{"postgres://u:p@host:5432/mydb?sslmode=disable", "mydb"},
		{"postgresql://u:p@host/otherdb", "otherdb"},
	}
	for _, tc := range cases {
		got := dbNameFromConnStr(tc.connStr)
		if got != tc.want {
			t.Errorf("dbNameFromConnStr(%q) = %q, want %q", tc.connStr, got, tc.want)
		}
	}
}
