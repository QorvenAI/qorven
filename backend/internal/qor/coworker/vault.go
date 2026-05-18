// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package coworker

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// vault.go — Obsidian-compatible markdown vault with backlinks.

var backlinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

func NewVault(path string) *Vault {
	os.MkdirAll(path, 0755)
	v := &Vault{Path: path, Notes: make(map[string]*Note)}
	v.loadFromDisk()
	return v
}

func (v *Vault) loadFromDisk() {
	entries, _ := os.ReadDir(v.Path)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") { continue }
		id := strings.TrimSuffix(e.Name(), ".md")
		data, err := os.ReadFile(filepath.Join(v.Path, e.Name()))
		if err != nil { continue }
		content := string(data)
		title, body := parseNote(content)
		note := &Note{ID: id, Title: title, Content: body, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		note.Backlinks = extractBacklinks(body)
		note.Tags = extractTags(body)
		v.Notes[id] = note
	}
}

func (v *Vault) Save(note *Note) error {
	note.UpdatedAt = time.Now()
	if note.ID == "" { note.ID = slugify(note.Title) }
	note.Backlinks = extractBacklinks(note.Content)
	note.Tags = extractTags(note.Content)
	v.mu.Lock()
	v.Notes[note.ID] = note
	v.mu.Unlock()
	content := fmt.Sprintf("# %s\n\n%s", note.Title, note.Content)
	return os.WriteFile(filepath.Join(v.Path, note.ID+".md"), []byte(content), 0644)
}

func (v *Vault) Get(id string) (*Note, bool) { v.mu.RLock(); defer v.mu.RUnlock(); n, ok := v.Notes[id]; return n, ok }

func (v *Vault) Delete(id string) error {
	v.mu.Lock()
	delete(v.Notes, id)
	v.mu.Unlock()
	return os.Remove(filepath.Join(v.Path, id+".md"))
}

func (v *Vault) Search(query string) []*Note {
	q := strings.ToLower(query)
	v.mu.RLock()
	defer v.mu.RUnlock()
	var matches []*Note
	for _, n := range v.Notes {
		text := strings.ToLower(n.Title + " " + n.Content)
		if strings.Contains(text, q) { matches = append(matches, n) }
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].UpdatedAt.After(matches[j].UpdatedAt) })
	return matches
}

func (v *Vault) GetBacklinksTo(id string) []*Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var refs []*Note
	for _, n := range v.Notes {
		for _, bl := range n.Backlinks {
			if bl == id { refs = append(refs, n); break }
		}
	}
	return refs
}

func (v *Vault) ListRecent(limit int) []*Note {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var all []*Note
	for _, n := range v.Notes { all = append(all, n) }
	sort.Slice(all, func(i, j int) bool { return all[i].UpdatedAt.After(all[j].UpdatedAt) })
	if limit > 0 && len(all) > limit { all = all[:limit] }
	return all
}

func parseNote(content string) (title, body string) {
	lines := strings.SplitN(content, "\n", 3)
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "# ") { title = strings.TrimPrefix(l, "# "); continue }
		body += l + "\n"
	}
	return strings.TrimSpace(title), strings.TrimSpace(body)
}

func extractBacklinks(content string) []string {
	matches := backlinkRe.FindAllStringSubmatch(content, -1)
	var links []string
	seen := map[string]bool{}
	for _, m := range matches {
		id := slugify(m[1])
		if !seen[id] { seen[id] = true; links = append(links, id) }
	}
	return links
}

func extractTags(content string) []string {
	re := regexp.MustCompile(`#(\w+)`)
	matches := re.FindAllStringSubmatch(content, -1)
	var tags []string
	seen := map[string]bool{}
	for _, m := range matches {
		t := strings.ToLower(m[1])
		if !seen[t] { seen[t] = true; tags = append(tags, t) }
	}
	return tags
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
