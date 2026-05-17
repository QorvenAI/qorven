// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package voice

import "testing"

func FuzzCleanTextForTTS(f *testing.F) {
	f.Add("**bold** text")
	f.Add("```go\ncode\n```")
	f.Add("https://example.com")
	f.Add("| A | B |\n|---|---|\n| 1 | 2 |")
	f.Add("")
	f.Add("plain text")

	f.Fuzz(func(t *testing.T, input string) {
		CleanTextForTTS(input)
	})
}
