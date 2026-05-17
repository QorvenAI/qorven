// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package autonomy

import "testing"

func TestCronJob_Fields(t *testing.T) {
	j := CronJob{AgentID: "a1", Schedule: "0 * * * *"}
	if j.AgentID != "a1" { t.Error("wrong agent") }
}

func TestCronScheduler_New(t *testing.T) {
	s := NewCronScheduler(nil)
	if s == nil { t.Fatal("nil") }
}
