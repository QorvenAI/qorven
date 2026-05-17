// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cron

import (
	"os"
	"path/filepath"
	"testing"
)

// Diamond-hard cron tests — verify the scheduler works as a product.

func TestDiamond_Cron_FullLifecycle(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "cron.json")

	handler := func(job *Job) (string, error) { return "executed", nil }
	svc := NewService(storePath, handler)

	// 1. Add job
	job, err := svc.AddJob("DailyReport", Schedule{Kind: "cron", Expr: "0 9 * * *"}, "Generate report", false, "", "", "agent-1")
	if err != nil { t.Fatal(err) }

	// 2. Get and verify
	got, ok := svc.GetJob(job.ID)
	if !ok { t.Fatal("job not found") }
	if got.Name != "DailyReport" { t.Errorf("name: %q", got.Name) }
	if got.Schedule.Expr != "0 9 * * *" { t.Errorf("schedule: %q", got.Schedule.Expr) }

	// 3. Disable
	svc.EnableJob(job.ID, false)
	got2, _ := svc.GetJob(job.ID)
	if got2.Enabled { t.Error("should be disabled") }

	// 4. Re-enable
	svc.EnableJob(job.ID, true)
	got3, _ := svc.GetJob(job.ID)
	if !got3.Enabled { t.Error("should be re-enabled") }

	// 5. List
	jobs := svc.ListJobs(true)
	found := false
	for _, j := range jobs { if j.ID == job.ID { found = true } }
	if !found { t.Error("job not in list") }

	// 6. Force run
	ran, result, err := svc.RunJob(job.ID, true)
	if err != nil { t.Fatal(err) }
	if !ran { t.Error("job didn't run") }
	if result != "executed" { t.Errorf("result: %q", result) }

	// 7. Check run log
	log := svc.GetRunLog(job.ID, 10)
	if len(log) == 0 { t.Error("no run log entries") }

	// 8. Remove
	svc.RemoveJob(job.ID)
	_, ok = svc.GetJob(job.ID)
	if ok { t.Error("job still exists after remove") }

	t.Log("cron lifecycle: add→get→disable→enable→list→run→log→remove ✓")
}

func TestDiamond_Cron_ScheduleTypes(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "cron.json")
	handler := func(job *Job) (string, error) { return "ok", nil }
	svc := NewService(storePath, handler)

	// Test all schedule types
	tests := []struct {
		name     string
		schedule Schedule
	}{
		{"cron-daily", Schedule{Kind: "cron", Expr: "0 9 * * *"}},
		{"cron-hourly", Schedule{Kind: "cron", Expr: "0 * * * *"}},
		{"every-5min", Schedule{Kind: "every", EveryMS: int64Ptr(300000)}},
		{"every-1hr", Schedule{Kind: "every", EveryMS: int64Ptr(3600000)}},
	}

	for _, tt := range tests {
		job, err := svc.AddJob(tt.name, tt.schedule, "test", false, "", "", "agent-1")
		if err != nil { t.Errorf("%s: %v", tt.name, err); continue }
		defer svc.RemoveJob(job.ID)
	}

	jobs := svc.ListJobs(true)
	if len(jobs) < 4 { t.Errorf("expected 4 jobs, got %d", len(jobs)) }
	t.Logf("schedule types: %d jobs created ✓", len(jobs))
}

func TestDiamond_Cron_Persistence(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "cron.json")
	handler := func(job *Job) (string, error) { return "ok", nil }

	// Create service and add job
	svc1 := NewService(storePath, handler)
	svc1.AddJob("Persistent", Schedule{Kind: "cron", Expr: "0 0 * * *"}, "persist test", false, "", "", "agent-1")

	// Verify file was created
	if _, err := os.Stat(storePath); os.IsNotExist(err) { t.Fatal("store file not created") }

	// Create new service from same file (simulates restart)
	svc2 := NewService(storePath, handler)
	svc2.Start()
	jobs := svc2.ListJobs(true)

	found := false
	for _, j := range jobs {
		if j.Name == "Persistent" { found = true }
	}
	if !found { t.Error("job not persisted across restart") }

	t.Log("persistence: job survives service restart ✓")
}

func TestDiamond_Cron_UpdateJob(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "cron.json")
	handler := func(job *Job) (string, error) { return "ok", nil }
	svc := NewService(storePath, handler)

	job, _ := svc.AddJob("Original", Schedule{Kind: "cron", Expr: "0 9 * * *"}, "original message", false, "", "", "agent-1")
	defer svc.RemoveJob(job.ID)

	// Update name and message
	newSched := Schedule{Kind: "cron", Expr: "0 17 * * *"}
	updated, err := svc.UpdateJob(job.ID, JobPatch{
		Name:     "Updated",
		Schedule: &newSched,
		Message:  "updated message",
	})
	if err != nil { t.Fatal(err) }

	if updated.Name != "Updated" { t.Errorf("name: %q", updated.Name) }
	if updated.Schedule.Expr != "0 17 * * *" { t.Errorf("schedule: %q", updated.Schedule.Expr) }

	t.Log("update job: name + schedule changed ✓")
}

func int64Ptr(v int64) *int64 { return &v }
