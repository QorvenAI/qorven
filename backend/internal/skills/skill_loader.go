// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Skill is a reusable workflow defined as markdown with YAML frontmatter.
type Skill struct {
	Name         string   `json:"name" yaml:"name"`
	Description  string   `json:"description" yaml:"description"`
	WhenToUse    string   `json:"when_to_use" yaml:"when_to_use"`
	AllowedTools []string `json:"allowed_tools,omitempty" yaml:"allowed_tools"`
	Model        string   `json:"model,omitempty" yaml:"model"`
	Context      string   `json:"context" yaml:"context"` // "inline" or "fork"
	Agent        string   `json:"agent,omitempty" yaml:"agent"`
	Prompt       string   `json:"prompt" yaml:"-"` // the markdown body
	FilePath     string   `json:"file_path" yaml:"-"`
}

// LoadSkillsDir loads all SKILL.md files from a directory tree.
func LoadSkillsDir(root string) ([]*Skill, error) {
	skills := []*Skill{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() { return err }
		if strings.ToUpper(info.Name()) != "SKILL.MD" { return nil }
		skill, err := ParseSkillFile(path)
		if err != nil { return nil } // skip invalid
		skills = append(skills, skill)
		return nil
	})
	return skills, err
}

// ParseSkillFile parses a SKILL.md with YAML frontmatter.
func ParseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil { return nil, err }
	content := string(data)

	skill := &Skill{FilePath: path, Context: "inline"}

	// Parse YAML frontmatter (between --- delimiters)
	if strings.HasPrefix(content, "---") {
		parts := strings.SplitN(content[3:], "---", 2)
		if len(parts) == 2 {
			parseFrontmatter(strings.TrimSpace(parts[0]), skill)
			skill.Prompt = strings.TrimSpace(parts[1])
		}
	} else {
		skill.Prompt = content
	}

	if skill.Name == "" {
		skill.Name = strings.TrimSuffix(filepath.Base(filepath.Dir(path)), "/")
	}
	return skill, nil
}

func parseFrontmatter(fm string, s *Skill) {
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, ":"); ok {
			v = strings.TrimSpace(v)
			v = strings.Trim(v, "\"'")
			switch strings.TrimSpace(k) {
			case "name": s.Name = v
			case "description": s.Description = v
			case "when_to_use": s.WhenToUse = v
			case "model": s.Model = v
			case "context": s.Context = v
			case "agent": s.Agent = v
			case "allowed_tools":
				s.AllowedTools = parseList(v)
			}
		}
	}
}

func parseList(v string) []string {
	v = strings.Trim(v, "[]")
	items := []string{}
	for _, item := range strings.Split(v, ",") {
		item = strings.TrimSpace(strings.Trim(item, "\"'"))
		if item != "" { items = append(items, item) }
	}
	return items
}

// BuildSkillPrompt creates the system prompt injection for a skill.
func BuildSkillPrompt(s *Skill) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Skill: %s\n", s.Name)
	if s.Description != "" { fmt.Fprintf(&sb, "> %s\n\n", s.Description) }
	if len(s.AllowedTools) > 0 {
		fmt.Fprintf(&sb, "**Allowed tools:** %s\n\n", strings.Join(s.AllowedTools, ", "))
	}
	sb.WriteString(s.Prompt)
	return sb.String()
}
