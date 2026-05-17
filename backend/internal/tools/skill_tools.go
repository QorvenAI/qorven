// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qorvenai/qorven/internal/skills"
)

// --- skill_search ---

type SkillSearchTool struct{ loader *skills.Loader }

func NewSkillSearchTool(loader *skills.Loader) *SkillSearchTool {
	return &SkillSearchTool{loader: loader}
}
func (t *SkillSearchTool) Name() string        { return "skill_search" }
func (t *SkillSearchTool) Description() string { return "Search available skills by keyword." }
func (t *SkillSearchTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"query":       map[string]any{"type": "string", "description": "Search keywords"},
		"max_results": map[string]any{"type": "integer", "description": "Max results (default 5)"},
	}, "required": []string{"query"}}
}

func (t *SkillSearchTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.loader == nil { return ErrorResult("skills not configured") }
	query, _ := args["query"].(string)
	if query == "" { return ErrorResult("query is required") }
	max := 5
	if n, ok := toInt(args["max_results"]); ok && n > 0 { max = n }

	allSkills := t.loader.ListSkills()
	results := skills.Search(allSkills, query, max)

	if len(results) == 0 { return TextResult("no skills found for: " + query) }

	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s (%s) — score: %.2f\n   %s\n\n", i+1, r.Name, r.Slug, r.Score, r.Description)
	}
	return TextResult(b.String())
}

// --- use_skill (observability marker) ---

type UseSkillTool struct{ loader *skills.Loader }

func NewUseSkillTool(loader *skills.Loader) *UseSkillTool { return &UseSkillTool{loader: loader} }
func (t *UseSkillTool) Name() string    { return "use_skill" }
func (t *UseSkillTool) Description() string {
	return "Invoke a skill by name. Returns the skill file path so you can read it with file_read."
}
func (t *UseSkillTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"name": map[string]any{"type": "string", "description": "Skill name/slug (from skill_search)"},
	}, "required": []string{"name"}}
}

func (t *UseSkillTool) Execute(ctx context.Context, args map[string]any) *Result {
	name, _ := args["name"].(string)
	if name == "" { return ErrorResult("name is required") }
	if t.loader != nil {
		for _, info := range t.loader.ListSkills() {
			if info.Slug == name || strings.EqualFold(info.Name, name) {
				return TextResult(fmt.Sprintf("Skill found: %s\nRead it now with file_read tool: %s", info.Name, info.Path))
			}
		}
		return ErrorResult(fmt.Sprintf("skill %q not found — use skill_search to find available skills", name))
	}
	return ErrorResult("skills not configured")
}

// --- skill_manage ---

// IsPinnedFn checks whether a skill slug is pinned. Gateway wires this on boot.
// When nil, the tool treats all skills as unpinned (backwards-compatible).
var OnSkillIsPinned func(ctx context.Context, slug string) bool

// OnSkillManage is called after a successful create/patch/delete so the
// gateway can flush skill-related caches.
var OnSkillManage func(action, slug string)

// SkillManageTool allows agents to create, patch, or delete workspace skills.
// It is fail-closed on pinned skills: any attempt to modify a pinned skill
// returns an error instead of writing.
type SkillManageTool struct {
	workspaceDir string
	loader       *skills.Loader
}

func NewSkillManageTool(workspaceDir string, loader *skills.Loader) *SkillManageTool {
	return &SkillManageTool{workspaceDir: workspaceDir, loader: loader}
}

func (t *SkillManageTool) Name() string        { return "skill_manage" }
func (t *SkillManageTool) Description() string {
	return "Create, patch, or delete workspace skills. Pinned skills cannot be modified by agents."
}
func (t *SkillManageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":  map[string]any{"type": "string", "enum": []string{"create", "patch", "delete"}, "description": "Operation to perform"},
			"slug":    map[string]any{"type": "string", "description": "Skill slug (used as directory name). Required for patch and delete."},
			"content": map[string]any{"type": "string", "description": "Full SKILL.md content (frontmatter + body). Required for create and patch."},
		},
		"required": []string{"action"},
	}
}

func (t *SkillManageTool) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)
	slug, _   := args["slug"].(string)
	content, _ := args["content"].(string)

	switch action {
	case "create":
		if content == "" { return ErrorResult("content is required for create") }
		// Parse slug from frontmatter if not supplied
		if slug == "" {
			if meta := skills.ParseFrontmatterString(content); meta != nil && meta.Slug != "" {
				slug = meta.Slug
			}
		}
		if slug == "" { return ErrorResult("slug is required (set slug: in frontmatter or pass as argument)") }
		if !skills.SlugRegexp.MatchString(slug) { return ErrorResult("invalid slug: must be lowercase alphanumeric with hyphens") }

		if isPinned(ctx, slug) {
			return ErrorResult(fmt.Sprintf("skill %q is pinned and cannot be modified by agents; ask an admin to unpin it first", slug))
		}

		// Security: reject dangerous patterns
		if violations, safe := skills.GuardSkillContent(content); !safe {
			return ErrorResult(fmt.Sprintf("skill content blocked — dangerous patterns detected: %v", violations))
		}

		dir := filepath.Join(t.workspaceDir, "skills", slug)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return ErrorResult("failed to create skill directory: " + err.Error())
		}
		path := filepath.Join(dir, "SKILL.md")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return ErrorResult("failed to write SKILL.md: " + err.Error())
		}
		if t.loader != nil { t.loader.BumpVersion() }
		notify(action, slug)
		return TextResult(fmt.Sprintf("skill %q created at %s", slug, path))

	case "patch":
		if slug == "" { return ErrorResult("slug is required for patch") }
		if content == "" { return ErrorResult("content is required for patch") }

		if isPinned(ctx, slug) {
			return ErrorResult(fmt.Sprintf("skill %q is pinned and cannot be modified by agents; ask an admin to unpin it first", slug))
		}

		if violations, safe := skills.GuardSkillContent(content); !safe {
			return ErrorResult(fmt.Sprintf("skill content blocked — dangerous patterns detected: %v", violations))
		}

		path := filepath.Join(t.workspaceDir, "skills", slug, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			return ErrorResult(fmt.Sprintf("skill %q not found; use action=create to create it", slug))
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return ErrorResult("failed to write SKILL.md: " + err.Error())
		}
		if t.loader != nil { t.loader.BumpVersion() }
		notify(action, slug)
		return TextResult(fmt.Sprintf("skill %q updated", slug))

	case "delete":
		if slug == "" { return ErrorResult("slug is required for delete") }

		if isPinned(ctx, slug) {
			return ErrorResult(fmt.Sprintf("skill %q is pinned and cannot be deleted by agents; ask an admin to unpin it first", slug))
		}

		dir := filepath.Join(t.workspaceDir, "skills", slug)
		if err := os.RemoveAll(dir); err != nil {
			return ErrorResult("failed to delete skill: " + err.Error())
		}
		if t.loader != nil { t.loader.BumpVersion() }
		notify(action, slug)
		return TextResult(fmt.Sprintf("skill %q deleted", slug))

	default:
		return ErrorResult(fmt.Sprintf("unknown action %q — use create, patch, or delete", action))
	}
}

func isPinned(ctx context.Context, slug string) bool {
	if OnSkillIsPinned == nil { return false }
	return OnSkillIsPinned(ctx, slug)
}

func notify(action, slug string) {
	if OnSkillManage != nil { OnSkillManage(action, slug) }
}
