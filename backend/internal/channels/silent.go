// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import "strings"

// IsSilent checks if the agent response contains [SILENT] token.
// Case-insensitive: [SILENT], [silent], [Silent] all match.
func IsSilent(response string) bool {
	return strings.Contains(strings.ToUpper(strings.TrimSpace(response)), "[SILENT]")
}

// StripSilent removes the [SILENT] token from response text.
func StripSilent(response string) string {
	upper := strings.ToUpper(response)
	idx := strings.Index(upper, "[SILENT]")
	if idx < 0 { return response }
	return strings.TrimSpace(response[:idx] + response[idx+8:])
}
