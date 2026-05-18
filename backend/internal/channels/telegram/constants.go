// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package telegram

import "time"

const (
	// MaxMessageLen is the safe limit for Telegram messages.
	// Telegram's hard limit is 4096, but we use 4000 for safety.
	MaxMessageLen = 4000

	// CaptionMaxLen is the max length for media captions.
	CaptionMaxLen = 1024

	// PairingReplyDebounce is the minimum interval between pairing replies to the same user.
	PairingReplyDebounce = 60 * time.Second

	// SendOverallTimeout is the maximum duration for a multi-retry send sequence.
	SendOverallTimeout = 60 * time.Second

	// ProbeOverallTimeout is the maximum duration for initial bot status check and command sync.
	ProbeOverallTimeout = 60 * time.Second
)
