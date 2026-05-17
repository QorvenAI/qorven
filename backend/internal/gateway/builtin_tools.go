// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/tools"
)

// BuiltinToolDef represents a built-in tool definition for the database catalog.
type BuiltinToolDef struct {
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Enabled     bool            `json:"enabled"`
	Settings    json.RawMessage `json:"settings,omitempty"`
	Requires    []string        `json:"requires,omitempty"`
}

// builtinToolSeedData returns the canonical list of built-in tools.
func builtinToolSeedData() []BuiltinToolDef {
	return []BuiltinToolDef{
		// filesystem
		{Name: "read_file", DisplayName: "Read File", Description: "Read file contents from the agent's workspace", Category: "filesystem", Enabled: true},
		{Name: "write_file", DisplayName: "Write File", Description: "Write content to a file, creating directories as needed", Category: "filesystem", Enabled: true},
		{Name: "list_files", DisplayName: "List Files", Description: "List files and directories in a workspace path", Category: "filesystem", Enabled: true},
		{Name: "edit", DisplayName: "Edit File", Description: "Apply targeted search-and-replace edits to existing files", Category: "filesystem", Enabled: true},

		// runtime
		{Name: "exec", DisplayName: "Execute Command", Description: "Execute a shell command and return stdout/stderr", Category: "runtime", Enabled: true},

		// web
		{Name: "web_search", DisplayName: "Web Search", Description: "Search the web using SearXNG or DuckDuckGo", Category: "web", Enabled: true},
		{Name: "web_fetch", DisplayName: "Web Fetch", Description: "Fetch a web page and extract text content", Category: "web", Enabled: true},

		// memory
		{Name: "memory_search", DisplayName: "Memory Search", Description: "Search agent long-term memory using semantic similarity", Category: "memory", Enabled: true, Requires: []string{"memory"}},
		{Name: "memory_get", DisplayName: "Memory Get", Description: "Retrieve a specific memory document by path", Category: "memory", Enabled: true, Requires: []string{"memory"}},
		{Name: "knowledge_graph_search", DisplayName: "Knowledge Graph Search", Description: "Search entities and relationships in the knowledge graph", Category: "memory", Enabled: true, Requires: []string{"knowledge_graph"}},

		// media
		{Name: "read_image", DisplayName: "Read Image", Description: "Analyze images using a vision-capable LLM", Category: "media", Enabled: false, Requires: []string{"vision_provider"}},
		{Name: "read_document", DisplayName: "Read Document", Description: "Analyze documents (PDF, Word, Excel, CSV)", Category: "media", Enabled: false, Requires: []string{"document_provider"}},
		{Name: "create_image", DisplayName: "Create Image", Description: "Generate images from text prompts", Category: "media", Enabled: false, Requires: []string{"image_gen_provider"}},
		{Name: "read_audio", DisplayName: "Read Audio", Description: "Analyze audio files using an audio-capable LLM", Category: "media", Enabled: false, Requires: []string{"audio_provider"}},
		{Name: "tts", DisplayName: "Text to Speech", Description: "Convert text to natural-sounding speech", Category: "media", Enabled: true, Requires: []string{"tts_provider"}},

		// browser
		{Name: "browser", DisplayName: "Browser", Description: "Automate browser: navigate, click, fill forms, screenshot", Category: "browser", Enabled: true, Requires: []string{"browser"}},

		// sessions
		{Name: "sessions_list", DisplayName: "List Sessions", Description: "List active chat sessions across channels", Category: "sessions", Enabled: true},
		{Name: "session_status", DisplayName: "Session Status", Description: "Get status and metadata of a chat session", Category: "sessions", Enabled: true},
		{Name: "sessions_history", DisplayName: "Session History", Description: "Retrieve message history of a session", Category: "sessions", Enabled: true},
		{Name: "sessions_send", DisplayName: "Send to Session", Description: "Send a message to an active session", Category: "sessions", Enabled: true},

		// messaging
		{Name: "message", DisplayName: "Message", Description: "Send a proactive message to a user on a channel", Category: "messaging", Enabled: true},

		// scheduling
		{Name: "cron", DisplayName: "Cron Scheduler", Description: "Schedule recurring tasks with cron expressions", Category: "scheduling", Enabled: true},
		{Name: "datetime", DisplayName: "Date Time", Description: "Get current date/time for scheduling and timestamps", Category: "scheduling", Enabled: true},

		// delegation
		{Name: "delegate_to_soul", DisplayName: "Delegate to Soul", Description: "Delegate a task to a specialist Soul", Category: "delegation", Enabled: true},
		{Name: "list_souls", DisplayName: "List Souls", Description: "List available Souls for delegation", Category: "delegation", Enabled: true},
		{Name: "create_soul", DisplayName: "Create Soul", Description: "Create a new specialist Soul", Category: "delegation", Enabled: true},
		{Name: "handoff_to_soul", DisplayName: "Handoff to Soul", Description: "Transfer full conversation to another Soul", Category: "delegation", Enabled: true},

		// skills
		{Name: "skill_search", DisplayName: "Skill Search", Description: "Search available skills by keyword", Category: "skills", Enabled: true},
		{Name: "use_skill", DisplayName: "Use Skill", Description: "Activate a skill's specialized capabilities", Category: "skills", Enabled: true},

		// teams
		{Name: "team_tasks", DisplayName: "Team Tasks", Description: "View, create, update tasks on the team board", Category: "teams", Enabled: true},
		{Name: "team_message", DisplayName: "Team Message", Description: "Send messages between team members", Category: "teams", Enabled: true},

		// connectors
		{Name: "execute_action", DisplayName: "Execute Connector Action", Description: "Execute an action on a connected service", Category: "connectors", Enabled: true},

		// research
		{Name: "deep_research", DisplayName: "Deep Research", Description: "Multi-step agentic search with citations", Category: "research", Enabled: true},
	}
}

// seedBuiltinTools populates the builtin_tools table from the canonical list.
// Idempotent: preserves user-customized enabled/settings on conflict.
func seedBuiltinTools(ctx context.Context, pool *pgxpool.Pool) {
	seeds := builtinToolSeedData()
	for _, s := range seeds {
		settings := s.Settings
		if settings == nil {
			settings = json.RawMessage("{}")
		}
		requires, _ := json.Marshal(s.Requires)
		_, err := pool.Exec(ctx,
			`INSERT INTO builtin_tools (name, display_name, description, category, enabled, settings, requires)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (name) DO UPDATE SET
			   display_name = EXCLUDED.display_name,
			   description = EXCLUDED.description,
			   category = EXCLUDED.category,
			   requires = EXCLUDED.requires`,
			s.Name, s.DisplayName, s.Description, s.Category, s.Enabled, settings, requires)
		if err != nil {
			slog.Warn("builtin_tools.seed_failed", "tool", s.Name, "error", err)
		}
	}
	slog.Info("builtin tools seeded", "count", len(seeds))
}

// applyBuiltinToolDisables reads the DB and disables/enables tools in the registry.
func applyBuiltinToolDisables(ctx context.Context, pool *pgxpool.Pool, toolsReg *tools.Registry) {
	rows, err := pool.Query(ctx, "SELECT name, enabled FROM builtin_tools")
	if err != nil {
		slog.Warn("builtin_tools.disable_check_failed", "error", err)
		return
	}
	defer rows.Close()

	var disabled, enabled int
	for rows.Next() {
		var name string
		var isEnabled bool
		rows.Scan(&name, &isEnabled)
		if !isEnabled {
			toolsReg.Disable(name)
			disabled++
		} else {
			toolsReg.Enable(name)
			enabled++
		}
	}
	if disabled > 0 {
		slog.Info("builtin tools updated", "disabled", disabled, "enabled", enabled)
	}
}
