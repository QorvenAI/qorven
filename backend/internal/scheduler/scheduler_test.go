// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package scheduler

import "testing"

func TestDefaultQueueConfig(t *testing.T) {
	cfg := DefaultQueueConfig()
	if cfg.Cap <= 0 { t.Error("cap should be > 0") }
}
