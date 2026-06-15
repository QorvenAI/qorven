// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// FilePart is an inbound chat file attachment (AI SDK "file" part shape).
type FilePart struct {
	URL       string
	MediaType string
	Filename  string
}

// EncodeAttachedFiles appends one <attached_file> block per file part to msg,
// mirroring web/app/api/chat/route.ts so static-export (production) chat handles
// attachments identically to the dev path. The blocks are consumed by
// ExtractFilesFromMessage in the agent loop.
func EncodeAttachedFiles(msg string, parts []FilePart) string {
	if len(parts) == 0 {
		return msg
	}

	var blocks []string
	for _, p := range parts {
		if p.URL == "" {
			continue
		}

		filename := p.Filename
		if filename == "" {
			filename = "attachment"
		}
		mediaType := p.MediaType
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}

		var content string
		if strings.HasPrefix(p.URL, "data:") {
			// data:<mediaType>;base64,<data>  OR  data:<mediaType>,<data> (plain text)
			commaIdx := strings.IndexByte(p.URL, ',')
			if commaIdx != -1 {
				header := p.URL[5:commaIdx] // e.g. "text/csv;base64"
				raw := p.URL[commaIdx+1:]
				if strings.HasSuffix(header, ";base64") {
					// Decode base64 → UTF-8 text
					decoded, err := base64.StdEncoding.DecodeString(raw)
					if err != nil {
						content = raw // fallback: pass raw
					} else {
						content = string(decoded)
					}
				} else {
					// Plain data URI — percent-decode like decodeURIComponent
					unescaped, err := url.QueryUnescape(raw)
					if err != nil {
						content = raw // fallback: pass raw
					} else {
						content = unescaped
					}
				}
			}
		} else {
			// Remote URL — pass as reference; agent can use web_fetch
			content = fmt.Sprintf("[File available at: %s]", p.URL)
		}

		// Cap at 80K chars to stay within context limits
		if len(content) > 80000 {
			content = content[:80000] + "\n\n[... truncated at 80K characters ...]"
		}

		blocks = append(blocks, fmt.Sprintf("<attached_file name=%q type=%q>\n%s\n</attached_file>", filename, mediaType, content))
	}

	if len(blocks) == 0 {
		return msg
	}

	joined := strings.Join(blocks, "\n\n")
	if msg != "" {
		return msg + "\n\n" + joined
	}
	return joined
}
