// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

var attachedFileRe = regexp.MustCompile(`(?s)<attached_file[^>]*>(.*?)</attached_file>`)
var fileNameRe = regexp.MustCompile(`name="([^"]*)"`)
var fileTypeRe = regexp.MustCompile(`type="([^"]*)"`)

// ExtractedFile holds a file extracted from the user message.
type ExtractedFile struct {
	Name    string
	Type    string
	Content string
}

// ExtractFilesFromMessage separates <attached_file> blocks from the user message.
// Returns the clean user message and extracted files.
func ExtractFilesFromMessage(msg string) (string, []ExtractedFile) {
	matches := attachedFileRe.FindAllStringSubmatch(msg, -1)
	if len(matches) == 0 {
		return msg, nil
	}

	var files []ExtractedFile
	clean := msg

	for _, m := range matches {
		fullMatch := m[0]
		content := strings.TrimSpace(m[1])

		name := "file"
		if nm := fileNameRe.FindStringSubmatch(fullMatch); len(nm) > 1 {
			name = nm[1]
		}
		ftype := "text/plain"
		if ft := fileTypeRe.FindStringSubmatch(fullMatch); len(ft) > 1 {
			ftype = ft[1]
		}

		files = append(files, ExtractedFile{Name: name, Type: ftype, Content: content})
		clean = strings.Replace(clean, fullMatch, "", 1)
	}

	clean = strings.TrimSpace(clean)
	if clean == "" {
		clean = "Analyze the attached file(s)."
	}
	return clean, files
}

// BuildFileContextMessage creates a structured context message for attached files.
// Injected as a separate user message BEFORE the actual user message (like RAG context).
func BuildFileContextMessage(files []ExtractedFile) providers.Message {
	var b strings.Builder
	b.WriteString("# Attached Files\n\nThe user has attached the following file(s). Read and analyze the content directly.\n\n")

	for i, f := range files {
		content := f.Content
		// Cap at 50K chars per file
		if len(content) > 50000 {
			content = content[:50000] + "\n\n[... truncated, file continues ...]"
		}
		b.WriteString("## File ")
		b.WriteString(fmt.Sprintf("%d", i+1))
		b.WriteString(": ")
		b.WriteString(f.Name)
		b.WriteString(" (")
		b.WriteString(f.Type)
		b.WriteString(")\n\n```\n")
		b.WriteString(content)
		b.WriteString("\n```\n\n")
	}

	return providers.Message{Role: "user", Content: b.String()}
}
