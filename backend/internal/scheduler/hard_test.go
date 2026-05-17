// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package scheduler

import "testing"

func TestHard_Scheduler_QueueConfig(t *testing.T) {
	cfg := DefaultQueueConfig()
	if cfg.Cap <= 0 { t.Error("cap") }
	if cfg.MaxConcurrent <= 0 { t.Error("max concurrent") }
}

func TestHard_Scheduler_LaneConfigs(t *testing.T) {
	lanes := []LaneConfig{
		{Name: "default"},
		{Name: "priority"},
		{Name: "background"},
	}
	for _, l := range lanes {
		if l.Name == "" { t.Error("empty lane name") }
	}
}
