// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package diff

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// GenerateDiff produces a unified diff string and counts additions/removals.
func GenerateDiff(oldContent, newContent, path string) (string, int, int) {
	if oldContent == newContent {
		return "", 0, 0
	}
	dmp := diffmatchpatch.New()
	a, b, c := dmp.DiffLinesToChars(oldContent, newContent)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, c)
	diffs = dmp.DiffCleanupSemantic(diffs)

	var sb strings.Builder
	additions, removals := 0, 0
	sb.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", path, path))
	for _, d := range diffs {
		lines := strings.Split(strings.TrimRight(d.Text, "\n"), "\n")
		switch d.Type {
		case diffmatchpatch.DiffDelete:
			for _, l := range lines {
				sb.WriteString("-" + l + "\n")
				removals++
			}
		case diffmatchpatch.DiffInsert:
			for _, l := range lines {
				sb.WriteString("+" + l + "\n")
				additions++
			}
		case diffmatchpatch.DiffEqual:
			// Show max 3 context lines around changes
			if len(lines) > 6 {
				for _, l := range lines[:3] {
					sb.WriteString(" " + l + "\n")
				}
				sb.WriteString(fmt.Sprintf("@@ ... %d unchanged lines ...\n", len(lines)-6))
				for _, l := range lines[len(lines)-3:] {
					sb.WriteString(" " + l + "\n")
				}
			} else {
				for _, l := range lines {
					sb.WriteString(" " + l + "\n")
				}
			}
		}
	}
	return sb.String(), additions, removals
}
