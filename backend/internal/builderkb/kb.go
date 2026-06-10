// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package builderkb holds the embedded "build apps on Qorven" knowledge base.
// The markdown docs are compiled into the binary and served to agents on demand
// via the get_builder_knowledge tool. The same docs back the always-on summary
// injected into every agent's system prompt (see Summary).
package builderkb

import (
	"embed"
	"sort"
	"strings"
)

//go:embed docs/*.md
var docsFS embed.FS

// topicFiles maps a public topic name to its embedded markdown file.
var topicFiles = map[string]string{
	"overview":     "docs/overview.md",
	"app-manifest": "docs/app-manifest.md",
	"scaffold":     "docs/scaffold.md",
	"ui":           "docs/ui.md",
	"db":           "docs/db.md",
	"external":     "docs/external.md",
}

// Topics returns the available knowledge topics, sorted.
func Topics() []string {
	out := make([]string, 0, len(topicFiles))
	for t := range topicFiles {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// Get returns the markdown for a topic. Unknown/empty topics return the
// overview plus the list of valid topics, and ok=false so callers can hint.
func Get(topic string) (content string, ok bool) {
	topic = strings.ToLower(strings.TrimSpace(topic))
	file, found := topicFiles[topic]
	if !found {
		overview, _ := docsFS.ReadFile("docs/overview.md")
		return string(overview) + "\n\n---\nValid topics: " + strings.Join(Topics(), ", "), false
	}
	b, err := docsFS.ReadFile(file)
	if err != nil {
		return "", false
	}
	return string(b), true
}

// Summary is the concise, always-on block injected into the agent system
// prompt. It tells the agent the app platform exists, the build flow, the hard
// rules, and to call get_builder_knowledge(topic) for full detail.
func Summary() []string {
	return []string{
		"### Building Apps on Qorven (platform extensibility)",
		"Qorven has a first-class app platform — like WordPress plugins. When a user asks you to \"build an app\" (a todo app, a dashboard, a CRM, …), they mean a Qorven app that installs into /apps, NOT a standalone project. You build on top of Qorven; we are the platform.",
		"",
		"An app is one directory with an `app.yaml` manifest. It can add: pages (UI under /apps/{slug}), tools (server-side scripts), a settings form, and its own DB tables (scoped migrations).",
		"",
		"Build flow (internal app) — do these with write_file + exec, then install_app:",
		"1. mkdir {appsDir}/{slug}/{migrations,tools,ui/frontend}",
		"2. write app.yaml (manifest)",
		"3. write migrations/001_create_tables.up.sql (CREATE TABLE IF NOT EXISTS, slug-prefixed)",
		"4. write tool scripts in tools/ (args via stdin JSON), chmod +x",
		"5. write ui/frontend/bundle.js as a PLAIN-JS IIFE (no npm/build — Node is not installed)",
		"6. install_app path={appsDir}/{slug}",
		"",
		"Hard rules: do NOT call scaffold_app (it makes Go-Wasm, wrong format); the manifest needs permissions:[tool_register] or tools won't run; use the host UI components on window.__QorvenUI and theme tokens (var(--primary), …) — never raw hex or text-[Npx]; scope DB rows by tenant_id.",
		"",
		"For full detail call get_builder_knowledge(topic): overview · app-manifest · scaffold · ui · db · external.",
		"",
	}
}
