// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// source_extract.go — Extract entities and relationships from source code files.
// Regex-based AST extraction for Go, Python, and TypeScript.

var (
	goFuncRe   = regexp.MustCompile(`^func\s+(?:\((\w+)\s+\*?(\w+)\)\s+)?(\w+)\(`)
	goTypeRe   = regexp.MustCompile(`^type\s+(\w+)\s+(struct|interface)`)
	goImportRe = regexp.MustCompile(`^\s+"([^"]+)"`)
	pyClassRe  = regexp.MustCompile(`^class\s+(\w+)`)
	pyFuncRe   = regexp.MustCompile(`^def\s+(\w+)\(`)
	pyImportRe = regexp.MustCompile(`^(?:from\s+(\S+)\s+)?import\s+(\S+)`)
	tsClassRe  = regexp.MustCompile(`^(?:export\s+)?class\s+(\w+)`)
	tsFuncRe   = regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)`)
	tsImportRe = regexp.MustCompile(`import\s+.*from\s+['"]([^'"]+)['"]`)
)

// SourceEntity is an entity extracted from source code.
type SourceEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // file, struct, interface, class, function, package
	File string `json:"file"`
}

// SourceRelation is a relationship between source entities.
type SourceRelation struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // contains, has_method, imports
}

// ExtractFromSource processes source files and builds entity/relationship lists.
func ExtractFromSource(paths []string) ([]SourceEntity, []SourceRelation) {
	entities := []SourceEntity{}
	relations := []SourceRelation{}
	seen := map[string]bool{}

	add := func(id, name, typ, file string) {
		if seen[id] { return }
		seen[id] = true
		entities = append(entities, SourceEntity{ID: id, Name: name, Type: typ, File: file})
	}

	for _, path := range paths {
		ext := strings.ToLower(filepath.Ext(path))
		stem := strings.TrimSuffix(filepath.Base(path), ext)
		fileID := "file::" + path
		add(fileID, filepath.Base(path), "file", path)

		f, err := os.Open(path)
		if err != nil { continue }
		scanner := bufio.NewScanner(f)
		var currentClass string

		for scanner.Scan() {
			line := scanner.Text()
			switch ext {
			case ".go":
				if m := goTypeRe.FindStringSubmatch(line); m != nil {
					id := stem + "::" + m[1]
					add(id, m[1], m[2], path)
					relations = append(relations, SourceRelation{Source: fileID, Target: id, Type: "contains"})
					currentClass = m[1]
				}
				if m := goFuncRe.FindStringSubmatch(line); m != nil {
					funcName, receiver := m[3], m[2]
					id := stem + "::" + receiver + "::" + funcName
					add(id, funcName, "function", path)
					if receiver != "" {
						relations = append(relations, SourceRelation{Source: stem + "::" + receiver, Target: id, Type: "has_method"})
					} else {
						relations = append(relations, SourceRelation{Source: fileID, Target: id, Type: "contains"})
					}
				}
				if m := goImportRe.FindStringSubmatch(line); m != nil {
					pkg := filepath.Base(m[1])
					id := "pkg::" + pkg
					add(id, pkg, "package", "")
					relations = append(relations, SourceRelation{Source: fileID, Target: id, Type: "imports"})
				}
			case ".py":
				if m := pyClassRe.FindStringSubmatch(line); m != nil {
					id := stem + "::" + m[1]
					add(id, m[1], "class", path)
					relations = append(relations, SourceRelation{Source: fileID, Target: id, Type: "contains"})
					currentClass = m[1]
				}
				if m := pyFuncRe.FindStringSubmatch(line); m != nil {
					id := stem + "::" + currentClass + "::" + m[1]
					add(id, m[1], "function", path)
					if currentClass != "" {
						relations = append(relations, SourceRelation{Source: stem + "::" + currentClass, Target: id, Type: "has_method"})
					} else {
						relations = append(relations, SourceRelation{Source: fileID, Target: id, Type: "contains"})
					}
				}
				if m := pyImportRe.FindStringSubmatch(line); m != nil {
					pkg := m[1]; if pkg == "" { pkg = m[2] }
					id := "pkg::" + pkg
					add(id, pkg, "package", "")
					relations = append(relations, SourceRelation{Source: fileID, Target: id, Type: "imports"})
				}
			case ".ts", ".tsx", ".js", ".jsx":
				if m := tsClassRe.FindStringSubmatch(line); m != nil {
					id := stem + "::" + m[1]
					add(id, m[1], "class", path)
					relations = append(relations, SourceRelation{Source: fileID, Target: id, Type: "contains"})
				}
				if m := tsFuncRe.FindStringSubmatch(line); m != nil {
					id := stem + "::" + m[1]
					add(id, m[1], "function", path)
					relations = append(relations, SourceRelation{Source: fileID, Target: id, Type: "contains"})
				}
				if m := tsImportRe.FindStringSubmatch(line); m != nil {
					pkg := filepath.Base(m[1])
					id := "pkg::" + pkg
					add(id, pkg, "package", "")
					relations = append(relations, SourceRelation{Source: fileID, Target: id, Type: "imports"})
				}
			}
		}
		f.Close()
	}
	return entities, relations
}
