// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// These tests use pure-Go SQLite (modernc.org/sqlite, registered as
// "sqlite") to end-to-end exercise the SQL toolset. Before this test,
// every sql_query_test.go case short-circuited before touching a real
// driver. The happy paths need coverage too.

// newSQLiteTestConn spins up an on-disk SQLite at t.TempDir() and
// registers it in a fresh SQLConnectionRegistry. Returns the registry
// ready for tool invocation. On-disk (not :memory:) so every
// goroutine the pool might spawn sees the same database — :memory:
// is per-connection in SQLite.
func newSQLiteTestConn(t *testing.T) *SQLConnectionRegistry {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")

	// Bootstrap some data directly so the tool tests have something
	// to query. Open a separate *sql.DB for bootstrap then close it
	// — the tool's registry will open its own pool.
	boot, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open bootstrap sqlite: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)`,
		`INSERT INTO users (id, name, email) VALUES (1, 'Alice', 'alice@example.com')`,
		`INSERT INTO users (id, name, email) VALUES (2, 'Bob',   'bob@example.com')`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, amount REAL)`,
		`INSERT INTO orders VALUES (1, 1, 42.50)`,
	} {
		if _, err := boot.Exec(stmt); err != nil {
			t.Fatalf("bootstrap %q: %v", stmt, err)
		}
	}
	_ = boot.Close()

	reg := NewSQLConnectionRegistry()
	if err := reg.Register(SQLConnection{
		Name:        "testdb",
		Driver:      "sqlite",
		DSN:         path,
		Description: "e2e test fixture",
	}); err != nil {
		t.Fatal(err)
	}
	return reg
}

// TestSQL_E2E_Schema: sql_schema against a live SQLite must enumerate
// our two tables with the columns we created.
func TestSQL_E2E_Schema(t *testing.T) {
	reg := newSQLiteTestConn(t)
	defer reg.Close()

	tool := NewSQLSchemaTool(reg)
	r := tool.Execute(context.Background(), map[string]any{"connection": "testdb"})
	if r.IsError {
		t.Fatalf("schema error: %s", r.ForLLM)
	}
	out := r.ForLLM
	for _, want := range []string{"## users", "## orders", "name (TEXT)", "email (TEXT)", "amount (REAL)"} {
		if !strings.Contains(out, want) {
			t.Errorf("schema missing %q in:\n%s", want, out)
		}
	}
	// PRIMARY KEY detection — value to the LLM when it's constructing queries.
	if !strings.Contains(out, "PRIMARY KEY") {
		t.Error("schema should annotate PRIMARY KEY columns")
	}
}

// TestSQL_E2E_SchemaSingleTable: passing table= limits output to just
// that table, which the LLM uses to keep context small.
func TestSQL_E2E_SchemaSingleTable(t *testing.T) {
	reg := newSQLiteTestConn(t)
	defer reg.Close()

	r := NewSQLSchemaTool(reg).Execute(context.Background(), map[string]any{
		"connection": "testdb",
		"table":      "users",
	})
	if r.IsError {
		t.Fatal(r.ForLLM)
	}
	if !strings.Contains(r.ForLLM, "## users") {
		t.Error("single-table schema missing users")
	}
	if strings.Contains(r.ForLLM, "## orders") {
		t.Error("single-table schema should not include orders")
	}
}

// TestSQL_E2E_SelectQuery: a read query must produce a markdown table
// with headers, separator row, and a returned-rows footer.
func TestSQL_E2E_SelectQuery(t *testing.T) {
	reg := newSQLiteTestConn(t)
	defer reg.Close()

	r := NewSQLQueryTool(reg).Execute(context.Background(), map[string]any{
		"connection": "testdb",
		"query":      "SELECT id, name FROM users ORDER BY id",
	})
	if r.IsError {
		t.Fatalf("query error: %s", r.ForLLM)
	}
	out := r.ForLLM
	for _, want := range []string{"| id | name |", "| 1 | Alice |", "| 2 | Bob |", "Returned 2 row"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// TestSQL_E2E_ParametersAreSafe: placeholder-based params must work
// AND prevent string-concat SQL injection. A naive implementation
// that interpolated would let `'; DROP TABLE users; --` land as-is.
func TestSQL_E2E_ParametersAreSafe(t *testing.T) {
	reg := newSQLiteTestConn(t)
	defer reg.Close()

	r := NewSQLQueryTool(reg).Execute(context.Background(), map[string]any{
		"connection": "testdb",
		"query":      "SELECT id FROM users WHERE name = ?",
		"params":     []any{"Alice"},
	})
	if r.IsError {
		t.Fatalf("param query error: %s", r.ForLLM)
	}
	if !strings.Contains(r.ForLLM, "| 1 |") {
		t.Errorf("parameterised query should find Alice; got %q", r.ForLLM)
	}
	// A would-be injection as the PARAMETER value becomes a literal.
	r = NewSQLQueryTool(reg).Execute(context.Background(), map[string]any{
		"connection": "testdb",
		"query":      "SELECT id FROM users WHERE name = ?",
		"params":     []any{"'; DROP TABLE users; --"},
	})
	if r.IsError {
		t.Fatalf("injection-as-param should return empty, not error: %s", r.ForLLM)
	}
	// Verify users is still there.
	r = NewSQLQueryTool(reg).Execute(context.Background(), map[string]any{
		"connection": "testdb",
		"query":      "SELECT count(*) FROM users",
	})
	if r.IsError || !strings.Contains(r.ForLLM, "| 2 |") {
		t.Errorf("users table must still exist after injection attempt; got %q", r.ForLLM)
	}
}

// TestSQL_E2E_WriteQueryWithoutConfirm: real live driver should still
// refuse DELETE without the confirm token. Duplicate of the unit-
// level check, but belt-and-suspenders against the dialect routing
// accidentally bypassing classifyStatement.
func TestSQL_E2E_WriteQueryWithoutConfirm(t *testing.T) {
	reg := newSQLiteTestConn(t)
	defer reg.Close()

	r := NewSQLQueryTool(reg).Execute(context.Background(), map[string]any{
		"connection": "testdb",
		"query":      "DELETE FROM users",
	})
	if !r.IsError {
		t.Fatal("DELETE without confirm must be rejected end-to-end")
	}
}

// TestSQL_E2E_WriteQueryConfirmed: with YES-WRITE, the write lands
// and the follow-up SELECT shows the effect.
func TestSQL_E2E_WriteQueryConfirmed(t *testing.T) {
	reg := newSQLiteTestConn(t)
	defer reg.Close()

	tool := NewSQLQueryTool(reg)
	// Insert via write path.
	r := tool.Execute(context.Background(), map[string]any{
		"connection": "testdb",
		"query":      "INSERT INTO users (id, name) VALUES (3, 'Carol')",
		"confirm":    "YES-WRITE",
	})
	if r.IsError {
		t.Fatalf("write with confirm should succeed: %s", r.ForLLM)
	}
	if !strings.Contains(r.ForLLM, "1 row") {
		t.Errorf("affected-rows count missing: %q", r.ForLLM)
	}
	// Verify with read.
	r = tool.Execute(context.Background(), map[string]any{
		"connection": "testdb",
		"query":      "SELECT name FROM users WHERE id = 3",
	})
	if !strings.Contains(r.ForLLM, "Carol") {
		t.Errorf("inserted row not visible: %q", r.ForLLM)
	}
}

// TestSQL_E2E_ReadOnlyEnforced: a connection registered read_only
// must refuse writes even WITH confirm=YES-WRITE.
func TestSQL_E2E_ReadOnlyEnforced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ro.db")
	boot, _ := sql.Open("sqlite", path)
	_, _ = boot.Exec(`CREATE TABLE t (x INT)`)
	_ = boot.Close()

	reg := NewSQLConnectionRegistry()
	_ = reg.Register(SQLConnection{
		Name: "ro", Driver: "sqlite", DSN: path, ReadOnly: true,
	})
	defer reg.Close()

	r := NewSQLQueryTool(reg).Execute(context.Background(), map[string]any{
		"connection": "ro",
		"query":      "INSERT INTO t VALUES (1)",
		"confirm":    "YES-WRITE",
	})
	if !r.IsError {
		t.Fatal("read-only connection must refuse INSERT even with confirm")
	}
	if !strings.Contains(r.ForLLM, "read-only") {
		t.Errorf("error should cite read-only; got %q", r.ForLLM)
	}
}
