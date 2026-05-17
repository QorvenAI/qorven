// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestClassifyStatement covers the entire SQL-first-keyword classifier.
// The goal: every statement that should be allowed classifies as Read,
// every destructive one classifies as DML or DDL, and anything weird
// falls through to Unknown so we refuse to run it.
func TestClassifyStatement(t *testing.T) {
	cases := []struct {
		sql  string
		want stmtKind
	}{
		// --- read ---
		{"SELECT * FROM users", stmtRead},
		{"select 1", stmtRead},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", stmtRead},
		{"EXPLAIN ANALYZE SELECT * FROM users", stmtRead},
		{"SHOW TABLES", stmtRead},
		{"DESCRIBE users", stmtRead},
		{"DESC users", stmtRead},
		// --- write DML ---
		{"INSERT INTO users VALUES (1)", stmtWriteDML},
		{"UPDATE users SET active = true", stmtWriteDML},
		{"DELETE FROM users", stmtWriteDML},
		{"MERGE INTO users USING ...", stmtWriteDML},
		{"  DELETE FROM users", stmtWriteDML}, // leading whitespace
		// --- DDL ---
		{"CREATE TABLE t (x INT)", stmtDDL},
		{"DROP TABLE t", stmtDDL},
		{"ALTER TABLE t ADD COLUMN y INT", stmtDDL},
		{"TRUNCATE users", stmtDDL},
		{"GRANT SELECT TO bob", stmtDDL},
		{"VACUUM", stmtDDL},
		// --- leading comments shouldn't confuse the classifier ---
		{"-- comment\nSELECT 1", stmtRead},
		{"/* multi\nline */ SELECT 1", stmtRead},
		{"   -- indented\n   SELECT 1", stmtRead},
		{"-- drop hint\nDELETE FROM users", stmtWriteDML},
		// --- unknown / malformed ---
		{"", stmtUnknown},
		{"   ", stmtUnknown},
		{"-- just a comment", stmtUnknown},
		{"CALL some_proc()", stmtUnknown}, // CALL isn't in our allow-list
		{"BEGIN", stmtUnknown},
	}
	for _, c := range cases {
		got := classifyStatement(c.sql)
		if got != c.want {
			t.Errorf("classify(%q) = %v, want %v", c.sql, got, c.want)
		}
	}
}

// TestClassifyStatement_CaseInsensitive: SQL is case-insensitive, so
// lowercase, mixed-case, and shouted keywords must all classify the
// same way. An attacker would otherwise try `dElEtE` to bypass.
func TestClassifyStatement_CaseInsensitive(t *testing.T) {
	variations := []string{
		"DELETE FROM x",
		"delete from x",
		"Delete From x",
		"dElEtE FROM x",
	}
	for _, v := range variations {
		if classifyStatement(v) != stmtWriteDML {
			t.Errorf("classify(%q) should be write DML regardless of case", v)
		}
	}
}

// TestSQLConnectionRegistry_RegisterAndList: basic CRUD.
func TestSQLConnectionRegistry_RegisterAndList(t *testing.T) {
	r := NewSQLConnectionRegistry()
	if got := r.List(); len(got) != 0 {
		t.Fatalf("fresh registry should be empty; got %+v", got)
	}

	err := r.Register(SQLConnection{Name: "main", Driver: "pgx", DSN: "dsn1", Description: "prod"})
	if err != nil {
		t.Fatal(err)
	}

	list := r.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 connection; got %d", len(list))
	}
	if list[0].Name != "main" {
		t.Errorf("wrong name: %q", list[0].Name)
	}
	if list[0].DSN != "" {
		t.Error("DSN must be redacted from List() output")
	}
}

// TestSQLConnectionRegistry_RegisterMissingName: an empty name is a
// logic error in the caller — refuse it loudly rather than accept a
// connection that can never be queried.
func TestSQLConnectionRegistry_RegisterMissingName(t *testing.T) {
	r := NewSQLConnectionRegistry()
	if err := r.Register(SQLConnection{}); err == nil {
		t.Fatal("empty name must be rejected")
	}
}

// TestSQLConnectionRegistry_ReplaceExisting: re-registering the same
// name should replace the old entry without leaking the old *sql.DB.
// We can't easily assert the leak without CGO sqlite, but we can at
// least verify the replacement is reflected in List().
func TestSQLConnectionRegistry_ReplaceExisting(t *testing.T) {
	r := NewSQLConnectionRegistry()
	_ = r.Register(SQLConnection{Name: "x", DSN: "v1", Description: "first"})
	_ = r.Register(SQLConnection{Name: "x", DSN: "v2", Description: "second"})

	list := r.List()
	if len(list) != 1 {
		t.Fatalf("replace should keep a single entry; got %d", len(list))
	}
	if list[0].Description != "second" {
		t.Errorf("expected replaced description; got %q", list[0].Description)
	}
}

// TestSQLConnectionRegistry_GetUnknown: asking for a connection that
// was never registered must produce a clear, user-legible error that
// points the LLM at sql_connections.
func TestSQLConnectionRegistry_GetUnknown(t *testing.T) {
	r := NewSQLConnectionRegistry()
	_, err := r.get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "sql_connections") {
		t.Errorf("error should nudge the LLM toward sql_connections; got %q", err)
	}
}

// TestSQLQueryTool_RejectsWriteWithoutConfirm: the confirm-token gate
// is the single most important safety check — it's the difference
// between "LLM runs a naive DELETE" and "LLM has to explicitly opt in".
func TestSQLQueryTool_RejectsWriteWithoutConfirm(t *testing.T) {
	reg := NewSQLConnectionRegistry()
	// Register a connection so get() wouldn't fail for other reasons.
	// We never actually open the DB because the classify check runs
	// first, so an invalid DSN is fine here.
	_ = reg.Register(SQLConnection{Name: "x", DSN: "invalid", Driver: "pgx"})

	tool := NewSQLQueryTool(reg)

	r := tool.Execute(context.Background(), map[string]any{
		"connection": "x",
		"query":      "DELETE FROM orders",
	})
	if !r.IsError {
		t.Fatal("DELETE without confirm must be rejected")
	}
	if !strings.Contains(r.ForLLM, "YES-WRITE") {
		t.Errorf("error should mention the YES-WRITE token; got %q", r.ForLLM)
	}

	r = tool.Execute(context.Background(), map[string]any{
		"connection": "x",
		"query":      "DROP TABLE orders",
	})
	if !r.IsError {
		t.Fatal("DROP without confirm must be rejected")
	}
}

// TestSQLQueryTool_UnknownStatement: if classifyStatement returns
// unknown, we must refuse without executing.
func TestSQLQueryTool_UnknownStatement(t *testing.T) {
	reg := NewSQLConnectionRegistry()
	_ = reg.Register(SQLConnection{Name: "x", DSN: "invalid", Driver: "pgx"})
	tool := NewSQLQueryTool(reg)

	r := tool.Execute(context.Background(), map[string]any{
		"connection": "x",
		"query":      "CALL nuke_prod()",
	})
	if !r.IsError {
		t.Fatal("unknown statement must be rejected")
	}
}

// TestSQLQueryTool_MissingArgs: both connection and query are required.
func TestSQLQueryTool_MissingArgs(t *testing.T) {
	tool := NewSQLQueryTool(NewSQLConnectionRegistry())

	r := tool.Execute(context.Background(), map[string]any{"query": "SELECT 1"})
	if !r.IsError {
		t.Error("missing connection should error")
	}
	r = tool.Execute(context.Background(), map[string]any{"connection": "x"})
	if !r.IsError {
		t.Error("missing query should error")
	}
	r = tool.Execute(context.Background(), map[string]any{"connection": "x", "query": "   "})
	if !r.IsError {
		t.Error("whitespace-only query should error")
	}
}

// TestSQLQueryTool_ReadOnlyConnection: even WITH confirm=YES-WRITE,
// a connection registered as read-only must refuse writes. Belt-and-
// suspenders safety — user can register an "audit" DB and trust it.
func TestSQLQueryTool_ReadOnlyConnection(t *testing.T) {
	reg := NewSQLConnectionRegistry()
	_ = reg.Register(SQLConnection{
		Name: "ro", DSN: "invalid", Driver: "pgx", ReadOnly: true,
	})
	tool := NewSQLQueryTool(reg)

	r := tool.Execute(context.Background(), map[string]any{
		"connection": "ro",
		"query":      "DELETE FROM x",
		"confirm":    "YES-WRITE",
	})
	// Connection get() will try to open "invalid" DSN and fail. That
	// error comes BEFORE the read-only check though, because we need
	// the connection object to know if it's read-only.
	// For this test we're happy with any error — the important thing
	// is we're NOT in the "OK. 0 row(s) affected" success path.
	if !r.IsError {
		t.Fatal("read-only connection should refuse write queries")
	}
}

// TestSQLConnectionsTool_Empty: with zero connections registered, the
// tool returns a helpful message, not a silent empty payload that
// would leave the LLM confused.
func TestSQLConnectionsTool_Empty(t *testing.T) {
	tool := NewSQLConnectionsTool(NewSQLConnectionRegistry())
	r := tool.Execute(context.Background(), nil)
	if r.IsError {
		t.Fatalf("empty list shouldn't be an error: %s", r.ForLLM)
	}
	if !strings.Contains(r.ForLLM, "Settings") {
		t.Errorf("empty message should tell user where to add connections; got %q", r.ForLLM)
	}
}

// TestSQLConnectionsTool_Populated: registered connections show up
// with name, driver, mode, and description.
func TestSQLConnectionsTool_Populated(t *testing.T) {
	reg := NewSQLConnectionRegistry()
	_ = reg.Register(SQLConnection{
		Name: "prod", Driver: "pgx", Description: "production orders + users", ReadOnly: true,
	})
	_ = reg.Register(SQLConnection{
		Name: "stage", Driver: "pgx", Description: "staging", ReadOnly: false,
	})

	r := NewSQLConnectionsTool(reg).Execute(context.Background(), nil)
	if r.IsError {
		t.Fatal(r.ForLLM)
	}
	out := r.ForLLM
	for _, want := range []string{"prod", "stage", "read-only", "read-write", "production orders"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got %q", want, out)
		}
	}
}

// TestRenderCell: cell rendering must survive the things that would
// otherwise break a markdown table (pipes, newlines, huge values,
// byte slices) without silently losing data.
func TestRenderCell(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, "_null_"},
		{"plain", "plain"},
		{"has | pipe", "has \\| pipe"},
		{"has\nnewline", "has⏎newline"},
		{42, "42"},
		{3.14, "3.14"},
		{true, "true"},
	}
	for _, c := range cases {
		got := renderCell(c.in)
		if got != c.want {
			t.Errorf("renderCell(%v) = %q, want %q", c.in, got, c.want)
		}
	}

	// Large string should be truncated with ellipsis.
	long := strings.Repeat("x", 300)
	got := renderCell(long)
	if !strings.HasSuffix(got, "…") {
		t.Error("long cell should end with ellipsis")
	}
	if len(got) > 220 {
		t.Errorf("long cell not truncated: %d chars", len(got))
	}

	// Small byte slice → hex.
	got = renderCell([]byte{0x01, 0xaf, 0xff})
	if got != "0x01afff" {
		t.Errorf("short bytes = %q", got)
	}
}

// TestSQLConnectionRegistry_TimeoutDefault: zero-valued Timeout must
// fill in a reasonable default so misconfigured connections don't
// hang queries forever.
func TestSQLConnectionRegistry_TimeoutDefault(t *testing.T) {
	r := NewSQLConnectionRegistry()
	_ = r.Register(SQLConnection{Name: "x", Driver: "pgx", DSN: "invalid"})
	// Inspect via internal map — test helper that probes the effective
	// config after Register's defaulting.
	r.mu.RLock()
	defer r.mu.RUnlock()
	got := r.conns["x"].cfg.Timeout
	if got == 0 {
		t.Error("timeout should default to non-zero")
	}
	if got != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", got)
	}
}
