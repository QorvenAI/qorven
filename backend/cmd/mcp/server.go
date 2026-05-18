// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var apiURL = "http://localhost:4200"
var apiToken = ""

func init() {
	if u := os.Getenv("QORVEN_API_URL"); u != "" {
		apiURL = u
	}
	if t := os.Getenv("QORVEN_API_TOKEN"); t != "" {
		apiToken = t
	}
	if t := os.Getenv("QORVEN_API_TOKEN"); t != "" && apiToken == "" {
		apiToken = t
	}
}

// Cmd returns the cobra command for `qorven mcp serve`
var Cmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run Qorven as an MCP server (for Claude Code, Cursor, VS Code, etc.)",
	Long: `Expose Qorven agents as MCP tools over stdio.

Any MCP-compatible client can connect:
  - Claude Code: add to .claude/settings.json
  - Cursor: add to .cursor/mcp.json
  - VS Code: add to settings.json

Example .claude/settings.json:
  {
    "mcpServers": {
      "qorven": {
        "command": "qorven",
        "args": ["mcp"],
        "env": { "QORVEN_API_TOKEN": "your-token" }
      }
    }
  }`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return Serve()
	},
}

// --- JSON-RPC types ---

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- MCP Protocol types ---

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// --- Server ---

func Serve() error {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = bytes_trimSpace(line)
		if len(line) == 0 {
			continue
		}

		var req rpcReq
		if err := json.Unmarshal(line, &req); err != nil {
			encoder.Encode(rpcResp{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "Parse error"}})
			continue
		}

		resp := handle(req)
		encoder.Encode(resp)
	}
}

func bytes_trimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\n' || b[0] == '\r') {
		b = b[1:]
	}
	for len(b) > 0 && (b[len(b)-1] == ' ' || b[len(b)-1] == '\t' || b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

func handle(req rpcReq) rpcResp {
	switch req.Method {
	case "initialize":
		return rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"subscribe": false, "listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    "qorven",
				"version": "1.0.0",
			},
		}}

	case "notifications/initialized":
		return rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}

	case "tools/list":
		return rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"tools": toolDefs(),
		}}

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		json.Unmarshal(req.Params, &params)
		result, isErr := callTool(params.Name, params.Arguments)
		return rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"content": []mcpContent{{Type: "text", Text: result}},
			"isError": isErr,
		}}

	case "resources/list":
		return rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"resources": resourceDefs(),
		}}

	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		json.Unmarshal(req.Params, &params)
		content := readResource(params.URI)
		return rpcResp{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"contents": []map[string]any{{
				"uri":      params.URI,
				"mimeType": "application/json",
				"text":     content,
			}},
		}}

	default:
		return rpcResp{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "Method not found: " + req.Method}}
	}
}

// --- Tool Definitions ---

func mk(name, desc string, props map[string]any, required []string) mcpTool {
	return mcpTool{
		Name: name, Description: desc,
		InputSchema: map[string]any{"type": "object", "properties": props, "required": required},
	}
}

func toolDefs() []mcpTool {
	return []mcpTool{
		// Agent tools - the core value
		mk("qorven_chat", "Chat with a Qorven AI agent. The agent has access to tools, memory, and web search.",
			map[string]any{
				"message":  map[string]any{"type": "string", "description": "Message to send to the agent"},
				"agent_id": map[string]any{"type": "string", "description": "Agent ID (optional, uses default if empty)"},
				"model":    map[string]any{"type": "string", "description": "Model override (optional)"},
			}, []string{"message"}),

		mk("qorven_search", "Search the web using Qorven's multi-provider search pipeline (Perplexity, Tavily, DuckDuckGo, etc.)",
			map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
			}, []string{"query"}),

		mk("qorven_research", "Deep research with citations. The agent searches multiple sources, extracts content, and synthesizes a comprehensive answer.",
			map[string]any{
				"topic": map[string]any{"type": "string", "description": "Research topic or question"},
			}, []string{"topic"}),

		mk("qorven_scenario", "Run a Scenario Lab simulation - multiple AI personas debate a topic and produce a report.",
			map[string]any{
				"seed":        map[string]any{"type": "string", "description": "Scenario seed/topic"},
				"agents":      map[string]any{"type": "integer", "description": "Number of personas (default 5)"},
				"rounds":      map[string]any{"type": "integer", "description": "Discussion rounds (default 5)"},
			}, []string{"seed"}),

		mk("qorven_memory", "Query agent memory - retrieve stored facts, preferences, and context about the user or project.",
			map[string]any{
				"query": map[string]any{"type": "string", "description": "Memory search query"},
			}, []string{"query"}),

		// Code intelligence tools
		mk("qorven_code_tree", "List files in a directory tree (respects .gitignore).",
			map[string]any{
				"path": map[string]any{"type": "string", "description": "Directory path (default: .)"},
			}, []string{}),

		mk("qorven_code_outline", "Get function/class/type definitions from a file.",
			map[string]any{
				"file": map[string]any{"type": "string", "description": "File path"},
			}, []string{"file"}),

		mk("qorven_code_search", "Search for a pattern across the codebase.",
			map[string]any{
				"query": map[string]any{"type": "string", "description": "Search pattern (regex)"},
				"path":  map[string]any{"type": "string", "description": "Directory to search (default: .)"},
			}, []string{"query"}),

		mk("qorven_code_read", "Read a file with optional line range.",
			map[string]any{
				"file":  map[string]any{"type": "string", "description": "File path"},
				"start": map[string]any{"type": "integer", "description": "Start line (optional)"},
				"end":   map[string]any{"type": "integer", "description": "End line (optional)"},
			}, []string{"file"}),

		mk("qorven_code_edit", "Write content to a file (creates directories as needed).",
			map[string]any{
				"file":    map[string]any{"type": "string", "description": "File path"},
				"content": map[string]any{"type": "string", "description": "File content"},
			}, []string{"file", "content"}),

		mk("qorven_code_symbol", "Find where a symbol (function, type, class) is defined.",
			map[string]any{
				"symbol": map[string]any{"type": "string", "description": "Symbol name to find"},
			}, []string{"symbol"}),


		mk("qorven_council", "Run an LLM Council query - multiple models answer, rank each other, then a Chairman synthesizes the best answer.",
			map[string]any{
				"query": map[string]any{"type": "string", "description": "The question to ask the council"},
				"depth": map[string]any{"type": "string", "description": "Depth: quick, balanced, deep, max (default: deep)"},
			}, []string{"query"}),
		// Supervisor tools - scoped escalation surface (mentor requirement)
		mk("supervisor_status", "Get the current supervisor system health and statistics.",
			map[string]any{}, []string{}),

		mk("supervisor_get_escalations", "Get pending escalations that need human approval.",
			map[string]any{}, []string{}),

		mk("supervisor_approve", "Approve a pending escalation.",
			map[string]any{
				"escalation_id": map[string]any{"type": "string", "description": "ID of the escalation to approve"},
				"reason":        map[string]any{"type": "string", "description": "Reason for approval"},
			}, []string{"escalation_id"}),

		mk("supervisor_reject", "Reject a pending escalation.",
			map[string]any{
				"escalation_id": map[string]any{"type": "string", "description": "ID of the escalation to reject"},
				"reason":        map[string]any{"type": "string", "description": "Reason for rejection"},
			}, []string{"escalation_id"}),
	}
}

// --- Resource Definitions ---

func resourceDefs() []mcpResource {
	return []mcpResource{
		{URI: "qorven://agents", Name: "Agent List", Description: "List of all configured Qorven agents", MimeType: "application/json"},
		{URI: "qorven://sessions", Name: "Recent Sessions", Description: "Recent chat sessions", MimeType: "application/json"},
		{URI: "qorven://models", Name: "Available Models", Description: "Models available through LiteLLM/Bedrock", MimeType: "application/json"},
		{URI: "qorven://tools/metrics", Name: "Tool Metrics", Description: "Per-tool execution statistics", MimeType: "application/json"},
	}
}

func readResource(uri string) string {
	switch uri {
	case "qorven://agents":
		return apiGet("/agents")
	case "qorven://sessions":
		return apiGet("/sessions")
	case "qorven://models":
		return apiGet("/providers/catalog")
	case "qorven://tools/metrics":
		return apiGet("/tools/metrics")
	default:
		return `{"error":"unknown resource"}`
	}
}

// --- Tool Execution ---

func callTool(name string, args map[string]any) (string, bool) {
	switch name {
	case "qorven_chat":
		msg, _ := args["message"].(string)
		agentID, _ := args["agent_id"].(string)
		model, _ := args["model"].(string)
		body := map[string]any{"messages": []map[string]string{{"role": "user", "content": msg}}, "stream": false}
		if model != "" {
			body["model"] = model
		}
		if agentID != "" {
			body["agent_id"] = agentID
		}
		return apiPost("/chat/completions", body), false

	case "qorven_search":
		q, _ := args["query"].(string)
		return apiPost("/chat/completions", map[string]any{
			"messages": []map[string]string{{"role": "user", "content": "Search: " + q}},
			"stream":   false,
		}), false

	case "qorven_research":
		topic, _ := args["topic"].(string)
		return apiPost("/chat/completions", map[string]any{
			"messages": []map[string]string{{"role": "user", "content": "Research with citations: " + topic}},
			"stream":   false,
		}), false

	case "qorven_scenario":
		seed, _ := args["seed"].(string)
		n := 5
		if v, ok := args["agents"].(float64); ok {
			n = int(v)
		}
		rounds := 5
		if v, ok := args["rounds"].(float64); ok {
			rounds = int(v)
		}
		return apiPost("/scenarios", map[string]any{"name": "MCP Scenario", "seed": seed, "agent_count": n, "rounds": rounds}), false

	case "qorven_memory":
		q, _ := args["query"].(string)
		return apiPost("/chat/completions", map[string]any{
			"messages": []map[string]string{{"role": "user", "content": "Check memory: " + q}},
			"stream":   false,
		}), false

	case "qorven_code_tree":
		p, _ := args["path"].(string)
		if p == "" {
			p = "."
		}
		if !safePath(p) {
			return "Error: invalid path", true
		}
		return safeExec("find", p, "-type", "f", "-maxdepth", "3", "-not", "-path", "*/.git/*"), false

	case "qorven_code_outline":
		f, _ := args["file"].(string)
		if !safePath(f) {
			return "Error: invalid path", true
		}
		return safeExec("grep", "-n", `func \|type \|class \|def \|import \|export `, f), false

	case "qorven_code_search":
		q, _ := args["query"].(string)
		p, _ := args["path"].(string)
		if p == "" {
			p = "."
		}
		if !safePath(p) {
			return "Error: invalid path", true
		}
		return safeExec("grep", "-rn", q, p, "--include=*.go", "--include=*.ts", "--include=*.py", "--include=*.rs"), false

	case "qorven_code_read":
		f, _ := args["file"].(string)
		if !safePath(f) {
			return "Error: invalid path", true
		}
		s := 1
		if v, ok := args["start"].(float64); ok {
			s = int(v)
		}
		e := 0
		if v, ok := args["end"].(float64); ok {
			e = int(v)
		}
		if e > 0 {
			return safeExec("sed", "-n", fmt.Sprintf("%d,%dp", s, e), f), false
		}
		return safeExec("cat", "-n", f), false

	case "qorven_code_edit":
		f, _ := args["file"].(string)
		if !safePath(f) {
			return "Error: invalid path", true
		}
		content, _ := args["content"].(string)
		if err := os.WriteFile(f, []byte(content), 0644); err != nil {
			return "Error: " + err.Error(), true
		}
		return "Written to " + f, false

	case "qorven_code_symbol":
		s, _ := args["symbol"].(string)
		if !safeIdent(s) {
			return "Error: invalid symbol", true
		}
		return safeExec("grep", "-rn", "func "+s+"\\|type "+s+"\\|class "+s+"\\|def "+s,
			".", "--include=*.go", "--include=*.ts", "--include=*.py"), false



	case "qorven_council":
		q, _ := args["query"].(string)
		depth, _ := args["depth"].(string)
		if depth == "" { depth = "deep" }
		return apiPost("/council", map[string]any{"query": q, "depth": depth}), false

	case "supervisor_status":
		return apiGet("/supervisor/status"), false

	case "supervisor_get_escalations":
		return apiGet("/supervisor/escalations"), false

	case "supervisor_approve":
		id, _ := args["escalation_id"].(string)
		reason, _ := args["reason"].(string)
		return apiPost("/supervisor/escalations/"+id+"/approve", map[string]any{"reason": reason}), false

	case "supervisor_reject":
		id, _ := args["escalation_id"].(string)
		reason, _ := args["reason"].(string)
		return apiPost("/supervisor/escalations/"+id+"/reject", map[string]any{"reason": reason}), false

	default:
		return "Unknown tool: " + name, true
	}
}

// --- API helpers ---

func apiGet(path string) string {
	return apiCall("GET", path, nil)
}

func apiPost(path string, body any) string {
	return apiCall("POST", path, body)
}

func apiCall(method, path string, body any) string {
	var bodyReader io.Reader
	if body != nil {
		d, _ := json.Marshal(body)
		bodyReader = strings.NewReader(string(d))
	}
	req, _ := http.NewRequest(method, apiURL+"/v1"+path, bodyReader)
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "Error: " + err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)

	// Extract content from chat completion response
	var r map[string]any
	if json.Unmarshal(b, &r) == nil {
		if ch, ok := r["choices"].([]any); ok && len(ch) > 0 {
			if m, ok := ch[0].(map[string]any); ok {
				if msg, ok := m["message"].(map[string]any); ok {
					if content, ok := msg["content"].(string); ok {
						return content
					}
				}
			}
		}
	}
	return string(b)
}

// --- Security ---

func safePath(p string) bool {
	if strings.Contains(p, "..") {
		return false
	}
	if strings.ContainsAny(p, ";|&$`\"'\\(){}!<>") {
		return false
	}
	if strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "/home/") && !strings.HasPrefix(p, "/tmp/") {
		return false
	}
	return true
}

func safeIdent(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '.') {
			return false
		}
	}
	return len(s) > 0 && len(s) < 200
}

func safeExec(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if len(out) > 10000 {
		out = out[:10000]
	}
	if err != nil {
		return string(out) + "\nError: " + err.Error()
	}
	return string(out)
}
