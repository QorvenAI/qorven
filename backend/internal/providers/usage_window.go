// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import "time"

// UsageWindow is a user-declared rate/request window for a key (typically an
// OAuth subscription, e.g. "N requests / 5h"). When exhausted within the
// current window, the key pool routes around the key so agents don't stall.
type UsageWindow struct {
	LimitCount int64
	UsedCount  int64
	ResetsAt   *time.Time
	WindowKind string
}

// Available reports whether the window currently permits another call. A nil
// window means "no declared window" → always available. When the window has
// passed its reset time it is treated as available (the loader/incrementer
// rolls it forward).
func (w *UsageWindow) Available(now time.Time) bool {
	if w == nil {
		return true
	}
	if w.ResetsAt != nil && now.After(*w.ResetsAt) {
		return true
	}
	return w.UsedCount < w.LimitCount
}
