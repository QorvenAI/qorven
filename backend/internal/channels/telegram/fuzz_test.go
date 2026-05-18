// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package telegram

import "testing"

func FuzzMarkdownToTelegramHTML(f *testing.F) {
	f.Add("**bold**")
	f.Add("*italic*")
	f.Add("`code`")
	f.Add("```go\nfunc main(){}\n```")
	f.Add("[link](https://x.com)")
	f.Add("# Header")
	f.Add("| A | B |\n|---|---|\n| 1 | 2 |")
	f.Add("")
	f.Add("plain text")

	f.Fuzz(func(t *testing.T, md string) {
		markdownToTelegramHTML(md)
	})
}

func FuzzChunkHTML(f *testing.F) {
	f.Add("short", 4096)
	f.Add("<b>bold</b>", 10)
	f.Add("", 4096)

	f.Fuzz(func(t *testing.T, html string, maxLen int) {
		if maxLen <= 0 { maxLen = 1 }
		if maxLen > 100000 { maxLen = 100000 }
		chunkHTML(html, maxLen)
	})
}

func FuzzEscapeHTML(f *testing.F) {
	f.Add("<script>alert('xss')</script>")
	f.Add("a & b")
	f.Add(`"quotes"`)
	f.Add("")

	f.Fuzz(func(t *testing.T, input string) {
		escapeHTML(input)
	})
}
