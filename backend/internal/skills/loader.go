// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Metadata holds parsed SKILL.md frontmatter.
type Metadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Slug        string `json:"slug"`
}

// Info describes a discovered skill.
type Info struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Path        string `json:"path"`
	BaseDir     string `json:"base_dir"`
	Source      string `json:"source"` // workspace, global, managed, builtin
	Description string `json:"description"`
}

// Loader discovers and loads SKILL.md files from multiple directories.
// 5-tier priority (highest first): workspace > project-agents > personal > global > managed > builtin
type Loader struct {
	workspaceSkills string
	globalSkills    string
	builtinSkills   string
	managedDir      string // DB-seeded versioned skills

	mu      sync.RWMutex
	cache   []Info
	version atomic.Int64
}

func NewLoader(workspace, globalSkills, builtinSkills string) *Loader {
	ws := ""
	if workspace != "" {
		ws = filepath.Join(workspace, "skills")
	}
	return &Loader{
		workspaceSkills: ws,
		globalSkills:    globalSkills,
		builtinSkills:   builtinSkills,
	}
}

func (l *Loader) SetManagedDir(dir string) { l.managedDir = dir; l.BumpVersion() }
func (l *Loader) BumpVersion()             { l.version.Add(1) }
func (l *Loader) Version() int64           { return l.version.Load() }

// Dirs returns all skill directories being monitored.
func (l *Loader) Dirs() []string {
	dirs := []string{}
	for _, d := range []string{l.workspaceSkills, l.globalSkills, l.builtinSkills, l.managedDir} {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// ListSkills returns all available skills, respecting priority hierarchy.
func (l *Loader) ListSkills() []Info {
	seen := make(map[string]bool)
	skills := []Info{}

	// Priority order: workspace > global > managed > builtin
	for _, src := range []struct {
		dir    string
		source string
	}{
		{l.workspaceSkills, "workspace"},
		{l.globalSkills, "global"},
	} {
		if src.dir == "" {
			continue
		}
		for _, info := range l.scanDir(src.dir, src.source) {
			if !seen[info.Slug] {
				seen[info.Slug] = true
				skills = append(skills, info)
			}
		}
	}

	// Managed skills (versioned subdirectories)
	if l.managedDir != "" {
		for _, info := range l.scanManaged() {
			if !seen[info.Slug] {
				seen[info.Slug] = true
				skills = append(skills, info)
			}
		}
	}

	// Builtin fallback
	if l.builtinSkills != "" {
		for _, info := range l.scanDir(l.builtinSkills, "builtin") {
			if !seen[info.Slug] {
				seen[info.Slug] = true
				skills = append(skills, info)
			}
		}
	}

	return skills
}

func (l *Loader) scanDir(dir, source string) []Info {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	skills := []Info{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "_shared" {
			continue
		}
		skillFile := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}
		meta := ParseFrontmatter(skillFile)
		info := Info{
			Name:    e.Name(),
			Slug:    e.Name(),
			Path:    skillFile,
			BaseDir: filepath.Join(dir, e.Name()),
			Source:  source,
		}
		if meta != nil {
			if meta.Name != "" {
				info.Name = meta.Name
			}
			info.Description = meta.Description
		}
		skills = append(skills, info)
	}
	return skills
}

func (l *Loader) scanManaged() []Info {
	entries, err := os.ReadDir(l.managedDir)
	if err != nil {
		return nil
	}
	skills := []Info{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		ver, dir := findLatestVersion(filepath.Join(l.managedDir, slug))
		if ver < 0 {
			continue
		}
		skillFile := filepath.Join(dir, "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}
		meta := ParseFrontmatter(skillFile)
		info := Info{Name: slug, Slug: slug, Path: skillFile, BaseDir: dir, Source: "managed"}
		if meta != nil {
			if meta.Name != "" {
				info.Name = meta.Name
			}
			info.Description = meta.Description
		}
		skills = append(skills, info)
	}
	return skills
}

func findLatestVersion(slugDir string) (int, string) {
	entries, err := os.ReadDir(slugDir)
	if err != nil {
		return -1, ""
	}
	versions := []int{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		v, err := strconv.Atoi(e.Name())
		if err != nil || v < 1 {
			continue
		}
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		return -1, ""
	}
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))
	return versions[0], filepath.Join(slugDir, strconv.Itoa(versions[0]))
}

// LoadSkill reads a skill's content by slug (frontmatter stripped).
func (l *Loader) LoadSkill(slug string) (string, bool) {
	for _, dir := range []string{l.workspaceSkills, l.globalSkills} {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, slug, "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := StripFrontmatter(string(data))
		content = strings.ReplaceAll(content, "{baseDir}", filepath.Join(dir, slug))
		return content, true
	}
	// Managed
	if l.managedDir != "" {
		ver, dir := findLatestVersion(filepath.Join(l.managedDir, slug))
		if ver >= 0 {
			data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
			if err == nil {
				content := StripFrontmatter(string(data))
				content = strings.ReplaceAll(content, "{baseDir}", dir)
				return content, true
			}
		}
	}
	// Builtin
	if l.builtinSkills != "" {
		data, err := os.ReadFile(filepath.Join(l.builtinSkills, slug, "SKILL.md"))
		if err == nil {
			content := StripFrontmatter(string(data))
			content = strings.ReplaceAll(content, "{baseDir}", filepath.Join(l.builtinSkills, slug))
			return content, true
		}
	}
	return "", false
}

// LoadForContext loads skills and formats for system prompt injection.
func (l *Loader) LoadForContext(allowList []string) string {
	names := []string{}
	if allowList == nil {
		for _, s := range l.ListSkills() {
			names = append(names, s.Slug)
		}
	} else {
		names = allowList
	}
	if len(names) == 0 {
		return ""
	}
	parts := []string{}
	for _, name := range names {
		content, ok := l.LoadSkill(name)
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("### Skill: %s\n\n%s", name, content))
	}
	if len(parts) == 0 {
		return ""
	}
	return "## Available Skills\n\n" + strings.Join(parts, "\n\n---\n\n")
}

// --- Frontmatter parsing ---

var frontmatterRe = regexp.MustCompile(`(?s)^---\n(.*?)\n---\n`)

func ParseFrontmatter(path string) *Metadata {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return ParseFrontmatterString(string(data))
}

func ParseFrontmatterString(content string) *Metadata {
	m := frontmatterRe.FindStringSubmatch(content)
	if len(m) < 2 {
		return nil
	}
	meta := &Metadata{}
	for _, line := range strings.Split(m[1], "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"'")
		switch key {
		case "name":
			meta.Name = val
		case "description":
			meta.Description = val
		case "slug":
			meta.Slug = val
		}
	}
	return meta
}

func StripFrontmatter(content string) string {
	return frontmatterRe.ReplaceAllString(content, "")
}

// Slugify converts a name to a URL-safe slug.
func Slugify(name string) string {
	s := strings.ToLower(name)
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// SlugRegexp validates skill slugs — defense against CVE-2026-28457.
var SlugRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

// GuardSkillContent scans for dangerous patterns in skill content.
// Defense against ClawHavoc supply chain attack.
var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)curl\s.*\|\s*(ba)?sh`),
	regexp.MustCompile(`(?i)wget\s.*-O\s*-\s*\|\s*(ba)?sh`),
	regexp.MustCompile(`(?i)pip\s+install\s+--pre`),
	regexp.MustCompile(`(?i)npm\s+install\s+-g\s+[^@\s]+`), // global npm without scope
	regexp.MustCompile(`(?i)eval\s*\(`),
	regexp.MustCompile(`(?i)exec\s*\(`),
	regexp.MustCompile(`(?i)os\.system\s*\(`),
	regexp.MustCompile(`(?i)subprocess\.(run|call|Popen)\s*\(`),
	regexp.MustCompile(`(?i)reverse.?shell`),
	regexp.MustCompile(`(?i)/dev/tcp/`),
}

func GuardSkillContent(content string) (violations []string, safe bool) {
	for _, p := range dangerousPatterns {
		if p.MatchString(content) {
			violations = append(violations, p.String())
		}
	}
	return violations, len(violations) == 0
}
