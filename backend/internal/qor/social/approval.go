// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

// NeedsApproval reports whether a post created by an agent with the given
// outbound_approval setting must be approved before it publishes. "none" =
// autonomous; everything else (incl. the empty/default) requires sign-off —
// safe default: when unset, gate rather than auto-publish.
func NeedsApproval(outboundApproval string) bool {
	return outboundApproval != "none"
}
