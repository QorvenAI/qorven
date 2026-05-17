// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver
	_ "modernc.org/sqlite"             // pure-Go SQLite; driver name "sqlite"
	// MySQL driver provided by the user's go.mod if they use it.
	// DuckDB intentionally left out — the only Go driver is CGO.
)

// SQLQueryTool lets an agent run read-only SQL against a registered
// database connection. Works on Postgres out of the box (pgx). The
// LLM sees the schema for each registered DB and can pick a connection
// by name.
//
// Why this exists as a separate tool instead of an MCP server:
//   - MCP servers are IPC; every query pays a round-trip through a
//     subprocess. For a snappy chat UX we want in-process latency.
//   - The LLM already knows how to form SQL — it doesn't need a
//     wrapper protocol. Expose the schema, take the query, execute,
//     format the result. That's the whole tool.
//
// Safety:
//   - Only connections explicitly registered by the user via
//     SQLConnectionRegistry are reachable. No DSN-in-args.
//   - Statement-type classifier rejects DDL (CREATE/DROP/ALTER)
//     and destructive DML (DELETE/UPDATE without WHERE) unless
//     the connection is registered as read-write AND the caller
//     passes confirm="YES-WRITE".
//   - Result size cap: 1,000 rows per query, 256 KiB total text.
//     Bigger queries should be written to a file via exec or
//     stream to the user via a different tool.
//   - Query timeout: 30 seconds default. Long-running analytics
//     queries need a dedicated adapter.

// SQLConnection is one registered database.
type SQLConnection struct {
	Name      string // short alias the LLM uses ("main", "analytics")
	Driver    string // "pgx" for now; add "mysql", "sqlite" etc. later
	DSN       string // stored in vault, not in plain args
	ReadOnly  bool   // true = refuse writes even with confirm token
	Timeout   time.Duration
	// Description is surfaced to the LLM so it knows what lives in
	// each DB ("production orders + users", "warehouse analytics").
	Description string
}

// SQLConnectionRegistry is the in-memory set of live connections.
// The gateway populates this from user_preferences on boot and when
// the user adds a new connection.
type SQLConnectionRegistry struct {
	mu    sync.RWMutex
	conns map[string]*registeredConn
}

type registeredConn struct {
	cfg SQLConnection
	db  *sql.DB // nil until first use — lazy open keeps boot fast
}

// NewSQLConnectionRegistry returns an empty registry.
func NewSQLConnectionRegistry() *SQLConnectionRegistry {
	return &SQLConnectionRegistry{conns: make(map[string]*registeredConn)}
}

// Register adds a connection. If a connection with the same Name
// already exists, it's replaced; the old *sql.DB is closed.
func (r *SQLConnectionRegistry) Register(c SQLConnection) error {
	if c.Name == "" {
		return errors.New("connection name required")
	}
	if c.Driver == "" {
		c.Driver = "pgx"
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.conns[c.Name]; ok && existing.db != nil {
		_ = existing.db.Close()
	}
	r.conns[c.Name] = &registeredConn{cfg: c}
	return nil
}

// List returns the names + descriptions of every registered connection.
// Exposed to the LLM via the sql_connections tool.
func (r *SQLConnectionRegistry) List() []SQLConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]SQLConnection, 0, len(r.conns))
	for _, rc := range r.conns {
		// Strip DSN from the public view — the LLM never needs it.
		c := rc.cfg
		c.DSN = ""
		out = append(out, c)
	}
	return out
}

// get returns the registered connection, opening it on first use.
func (r *SQLConnectionRegistry) get(ctx context.Context, name string) (*registeredConn, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rc, ok := r.conns[name]
	if !ok {
		return nil, fmt.Errorf("no such connection %q — use sql_connections to list available", name)
	}
	if rc.db == nil {
		db, err := sql.Open(rc.cfg.Driver, rc.cfg.DSN)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", name, err)
		}
		// Keep the pool small — an LLM agent doesn't need 25
		// parallel connections to run ad-hoc queries. Two is plenty.
		db.SetMaxOpenConns(2)
		db.SetMaxIdleConns(1)
		// Ping with a short timeout so a misconfigured DSN fails
		// fast instead of blocking the tool's 30s budget.
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("ping %s: %w", name, err)
		}
		rc.db = db
	}
	return rc, nil
}

// Close closes every registered connection. Call on gateway shutdown.
func (r *SQLConnectionRegistry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rc := range r.conns {
		if rc.db != nil {
			_ = rc.db.Close()
			rc.db = nil
		}
	}
}

// --- Tool: sql_connections ---

type SQLConnectionsTool struct{ reg *SQLConnectionRegistry }

func NewSQLConnectionsTool(r *SQLConnectionRegistry) *SQLConnectionsTool {
	return &SQLConnectionsTool{reg: r}
}

func (t *SQLConnectionsTool) Name() string { return "sql_connections" }

func (t *SQLConnectionsTool) Description() string {
	return "List the database connections that are registered and available to query. " +
		"Call this first if the user asks about data and you don't know which DB to use. " +
		"Each entry includes a name (use with sql_query), driver, a description of " +
		"what lives there, and whether the connection is read-only."
}

func (t *SQLConnectionsTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *SQLConnectionsTool) Execute(ctx context.Context, args map[string]any) *Result {
	conns := t.reg.List()
	if len(conns) == 0 {
		return TextResult(
			"No database connections registered. The user can add one in Settings → Connections → Database.")
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d database connection(s):\n\n", len(conns)))
	for _, c := range conns {
		mode := "read-write"
		if c.ReadOnly {
			mode = "read-only"
		}
		sb.WriteString(fmt.Sprintf("- **%s** (%s, %s)\n", c.Name, c.Driver, mode))
		if c.Description != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", c.Description))
		}
	}
	return TextResult(sb.String())
}

// --- Tool: sql_schema ---

type SQLSchemaTool struct{ reg *SQLConnectionRegistry }

func NewSQLSchemaTool(r *SQLConnectionRegistry) *SQLSchemaTool {
	return &SQLSchemaTool{reg: r}
}

func (t *SQLSchemaTool) Name() string { return "sql_schema" }

func (t *SQLSchemaTool) Description() string {
	return "Return the schema (tables + columns + types) for a registered database connection. " +
		"Call this before writing a sql_query to see what tables and columns exist."
}

func (t *SQLSchemaTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"connection": map[string]any{
				"type":        "string",
				"description": "Connection name from sql_connections.",
			},
			"table": map[string]any{
				"type":        "string",
				"description": "Optional: describe a single table. Omit to list every table + column.",
			},
		},
		"required": []string{"connection"},
	}
}

func (t *SQLSchemaTool) Execute(ctx context.Context, args map[string]any) *Result {
	name, _ := args["connection"].(string)
	if name == "" {
		return ErrorResult("connection is required")
	}
	rc, err := t.reg.get(ctx, name)
	if err != nil {
		return ErrorResult(err.Error())
	}
	table, _ := args["table"].(string)

	qctx, cancel := context.WithTimeout(ctx, rc.cfg.Timeout)
	defer cancel()

	// Dialect routing — SQLite uses PRAGMA, Postgres (and most
	// OpenAI-compat DBs) speak information_schema. MySQL also works
	// with information_schema but the column names differ slightly.
	switch rc.cfg.Driver {
	case "sqlite":
		return schemaSQLite(qctx, rc.db, name, table)
	default:
		return schemaInformationSchema(qctx, rc.db, name, table)
	}
}

// schemaInformationSchema queries Postgres-style information_schema.
// Works for pgx, MySQL, and most Postgres-wire-compatible DBs.
func schemaInformationSchema(ctx context.Context, db *sql.DB, name, table string) *Result {
	var query string
	var queryArgs []any
	if table == "" {
		query = `SELECT table_name, column_name, data_type
		         FROM information_schema.columns
		         WHERE table_schema = 'public'
		         ORDER BY table_name, ordinal_position`
	} else {
		query = `SELECT table_name, column_name, data_type
		         FROM information_schema.columns
		         WHERE table_schema = 'public' AND table_name = $1
		         ORDER BY ordinal_position`
		queryArgs = []any{table}
	}
	rows, err := db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return ErrorResult(fmt.Sprintf("schema query: %v", err))
	}
	defer rows.Close()

	currentTable := ""
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Schema for %s:\n\n", name))
	for rows.Next() {
		var tn, cn, dt string
		if err := rows.Scan(&tn, &cn, &dt); err != nil {
			return ErrorResult(fmt.Sprintf("scan: %v", err))
		}
		if tn != currentTable {
			if currentTable != "" {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("## %s\n", tn))
			currentTable = tn
		}
		sb.WriteString(fmt.Sprintf("- %s (%s)\n", cn, dt))
	}
	if currentTable == "" {
		sb.WriteString("(no tables in public schema)\n")
	}
	return TextResult(sb.String())
}

// schemaSQLite uses sqlite_master + PRAGMA table_info. Two-step:
// first get every user table, then describe its columns. Per-table
// PRAGMA calls are cheap on SQLite (no network, no parsing cost).
func schemaSQLite(ctx context.Context, db *sql.DB, name, table string) *Result {
	var tables []string
	if table != "" {
		tables = []string{table}
	} else {
		rows, err := db.QueryContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
		if err != nil {
			return ErrorResult(fmt.Sprintf("list tables: %v", err))
		}
		defer rows.Close()
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err == nil {
				tables = append(tables, n)
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Schema for %s:\n\n", name))
	if len(tables) == 0 {
		sb.WriteString("(no user tables)\n")
		return TextResult(sb.String())
	}
	for _, t := range tables {
		sb.WriteString(fmt.Sprintf("## %s\n", t))
		rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", t))
		if err != nil {
			sb.WriteString(fmt.Sprintf("  (describe error: %v)\n\n", err))
			continue
		}
		// PRAGMA table_info columns: cid, name, type, notnull, dflt_value, pk
		for rows.Next() {
			var cid int
			var cn, dt string
			var notnull int
			var dflt sql.NullString
			var pk int
			if err := rows.Scan(&cid, &cn, &dt, &notnull, &dflt, &pk); err != nil {
				continue
			}
			extra := ""
			if pk == 1 {
				extra += " PRIMARY KEY"
			}
			if notnull == 1 {
				extra += " NOT NULL"
			}
			sb.WriteString(fmt.Sprintf("- %s (%s)%s\n", cn, dt, extra))
		}
		rows.Close()
		sb.WriteString("\n")
	}
	return TextResult(sb.String())
}

// --- Tool: sql_query ---

type SQLQueryTool struct{ reg *SQLConnectionRegistry }

func NewSQLQueryTool(r *SQLConnectionRegistry) *SQLQueryTool {
	return &SQLQueryTool{reg: r}
}

func (t *SQLQueryTool) Name() string { return "sql_query" }

func (t *SQLQueryTool) Description() string {
	return "Run a SQL query against a registered database connection. " +
		"Read queries (SELECT, WITH) run directly. " +
		"Write queries (INSERT/UPDATE/DELETE/DDL) require confirm=\"YES-WRITE\" AND " +
		"a connection that is not read-only. Results are returned as a markdown table, " +
		"capped at 1000 rows."
}

func (t *SQLQueryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"connection": map[string]any{
				"type":        "string",
				"description": "Connection name from sql_connections.",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "SQL query. Use parameterised placeholders ($1, $2, ...) and pass values via `params` — never interpolate user input.",
			},
			"params": map[string]any{
				"type":        "array",
				"description": "Values for $1, $2, ... placeholders. Order-sensitive.",
				"items":       map[string]any{},
			},
			"confirm": map[string]any{
				"type":        "string",
				"description": "Required for write queries. Must equal YES-WRITE.",
			},
		},
		"required": []string{"connection", "query"},
	}
}

func (t *SQLQueryTool) Execute(ctx context.Context, args map[string]any) *Result {
	name, _ := args["connection"].(string)
	query, _ := args["query"].(string)
	if name == "" || strings.TrimSpace(query) == "" {
		return ErrorResult("connection and query are required")
	}

	kind := classifyStatement(query)
	if kind == stmtUnknown {
		return ErrorResult("could not classify statement — refusing to run")
	}
	if kind != stmtRead {
		confirm, _ := args["confirm"].(string)
		if confirm != "YES-WRITE" {
			return ErrorResult(fmt.Sprintf(
				"%s statements require confirm=\"YES-WRITE\"", kind.String()))
		}
	}

	rc, err := t.reg.get(ctx, name)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if kind != stmtRead && rc.cfg.ReadOnly {
		return ErrorResult(fmt.Sprintf("connection %q is read-only", name))
	}

	// Extract params if any.
	var params []any
	if raw, ok := args["params"].([]any); ok {
		params = raw
	}

	qctx, cancel := context.WithTimeout(ctx, rc.cfg.Timeout)
	defer cancel()

	if kind == stmtRead {
		return runSelect(qctx, rc.db, query, params)
	}
	return runExec(qctx, rc.db, query, params)
}

// --- SQL statement classification ---

type stmtKind int

const (
	stmtUnknown stmtKind = iota
	stmtRead             // SELECT / WITH / EXPLAIN / SHOW
	stmtWriteDML         // INSERT / UPDATE / DELETE / MERGE / UPSERT
	stmtDDL              // CREATE / DROP / ALTER / TRUNCATE / GRANT / REVOKE
)

func (k stmtKind) String() string {
	switch k {
	case stmtRead:
		return "read"
	case stmtWriteDML:
		return "write DML"
	case stmtDDL:
		return "DDL"
	}
	return "unknown"
}

var leadingCommentOrWS = regexp.MustCompile(`(?s)^(?:\s|--.*?\n|/\*.*?\*/)*`)

// classifyStatement inspects the first keyword after stripping leading
// whitespace and comments. Conservative: unknown = refuse to run.
func classifyStatement(sql string) stmtKind {
	stripped := leadingCommentOrWS.ReplaceAllString(sql, "")
	stripped = strings.TrimSpace(stripped)
	if stripped == "" {
		return stmtUnknown
	}
	// Get the first keyword.
	word := stripped
	if idx := strings.IndexAny(stripped, " \t\n("); idx >= 0 {
		word = stripped[:idx]
	}
	switch strings.ToUpper(word) {
	case "SELECT", "WITH", "EXPLAIN", "SHOW", "DESCRIBE", "DESC":
		return stmtRead
	case "INSERT", "UPDATE", "DELETE", "MERGE", "UPSERT", "REPLACE":
		return stmtWriteDML
	case "CREATE", "DROP", "ALTER", "TRUNCATE", "GRANT", "REVOKE", "VACUUM", "REINDEX":
		return stmtDDL
	}
	return stmtUnknown
}

// --- runners ---

const (
	maxRows     = 1000
	maxBytesOut = 256 * 1024
)

func runSelect(ctx context.Context, db *sql.DB, query string, params []any) *Result {
	rows, err := db.QueryContext(ctx, query, params...)
	if err != nil {
		return ErrorResult(fmt.Sprintf("query: %v", err))
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return ErrorResult(fmt.Sprintf("columns: %v", err))
	}

	// Collect rows once so we can emit BOTH a markdown table (for the
	// LLM) and a structured widget payload (for the SQLResultCard).
	// Capped at maxRows to preserve the existing safety envelope.
	var sb strings.Builder
	sb.WriteString("| ")
	for i, c := range cols {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(c)
	}
	sb.WriteString(" |\n| ")
	for i := range cols {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString("---")
	}
	sb.WriteString(" |\n")

	// widgetRows keeps typed values for the card. Cell rendering for
	// the markdown happens inline; we also preserve raw values so the
	// client can sort numerically without parsing our cell-formatted
	// strings.
	var widgetRows [][]any

	rowCount := 0
	truncated := false
	for rows.Next() {
		if rowCount >= maxRows {
			truncated = true
			break
		}
		if sb.Len() > maxBytesOut {
			truncated = true
			break
		}
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return ErrorResult(fmt.Sprintf("scan row %d: %v", rowCount+1, err))
		}
		sb.WriteString("| ")
		for i, v := range values {
			if i > 0 {
				sb.WriteString(" | ")
			}
			sb.WriteString(renderCell(v))
		}
		sb.WriteString(" |\n")

		// Snapshot typed values for the widget. []byte (bytea / BLOB)
		// is converted to a readable placeholder so JSON marshalling
		// doesn't spit bytes to the frontend.
		widgetRow := make([]any, len(values))
		for i, v := range values {
			switch x := v.(type) {
			case []byte:
				widgetRow[i] = fmt.Sprintf("<%d bytes>", len(x))
			default:
				widgetRow[i] = v
			}
		}
		widgetRows = append(widgetRows, widgetRow)
		rowCount++
	}
	if err := rows.Err(); err != nil {
		return ErrorResult(fmt.Sprintf("iter: %v", err))
	}

	sb.WriteString(fmt.Sprintf("\n_Returned %d row(s)", rowCount))
	if truncated {
		sb.WriteString(fmt.Sprintf("; capped at %d rows or %d KiB — to get the next page add LIMIT %d OFFSET %d to your query", maxRows, maxBytesOut/1024, maxRows, maxRows))
	}
	sb.WriteString("._\n")

	return &Result{
		ForLLM:  sb.String(),
		ForUser: sb.String(),
		Widget: &Widget{
			Type: "sql_result",
			Data: map[string]any{
				"columns":   cols,
				"rows":      widgetRows,
				"truncated": truncated,
				"query":     query,
			},
		},
	}
}

func runExec(ctx context.Context, db *sql.DB, query string, params []any) *Result {
	res, err := db.ExecContext(ctx, query, params...)
	if err != nil {
		return ErrorResult(fmt.Sprintf("exec: %v", err))
	}
	affected, _ := res.RowsAffected()
	return TextResult(fmt.Sprintf("OK. %d row(s) affected.", affected))
}

// renderCell formats a scanned value for a markdown cell. Pipes and
// newlines would break the table — replace them with readable
// substitutes rather than backslash-escaping (LLM reading the result
// shouldn't have to un-escape anything).
func renderCell(v any) string {
	if v == nil {
		return "_null_"
	}
	s := fmt.Sprint(v)
	// Common for byte-slice columns (bytea, BLOB): show as hex prefix.
	if b, ok := v.([]byte); ok {
		if len(b) > 64 {
			return fmt.Sprintf("0x%x…(%d bytes)", b[:32], len(b))
		}
		return fmt.Sprintf("0x%x", b)
	}
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", "⏎")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
