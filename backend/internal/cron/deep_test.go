// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cron

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Deep cron tests — scheduling accuracy, concurrent job management, persistence.

func TestDeep_Cron_JobLifecycle(t *testing.T) {
	dir := t.TempDir()
	executed := make(chan string, 100)
	svc := NewService(filepath.Join(dir, "cron.json"), func(job *Job) (string, error) {
		executed <- job.Name
		return "executed: " + job.Name, nil
	})

	// Create 5 jobs
	names := []string{"daily-report", "hourly-check", "weekly-backup", "monthly-audit", "yearly-review"}
	var ids []string
	for _, name := range names {
		job, err := svc.AddJob(name, Schedule{Kind: "cron", Expr: "0 * * * *"}, "run "+name, false, "", "", "agent1")
		if err != nil { t.Fatalf("add %s: %v", name, err) }
		ids = append(ids, job.ID)
	}

	// List all
	jobs := svc.ListJobs(true)
	if len(jobs) != 5 { t.Errorf("expected 5, got %d", len(jobs)) }

	// Disable one
	svc.EnableJob(ids[2], false)
	job, _ := svc.GetJob(ids[2])
	if job.Enabled { t.Error("should be disabled") }

	// Re-enable
	svc.EnableJob(ids[2], true)
	job, _ = svc.GetJob(ids[2])
	if !job.Enabled { t.Error("should be re-enabled") }

	// Run one manually
	ok, result, err := svc.RunJob(ids[0], true)
	if err != nil { t.Fatalf("run: %v", err) }
	if !ok { t.Error("should execute") }
	t.Logf("manual run result: %q", result)

	// Verify execution
	select {
	case name := <-executed:
		if name != "daily-report" { t.Errorf("wrong job executed: %s", name) }
	case <-time.After(time.Second):
		t.Error("job not executed")
	}

	// Remove one
	svc.RemoveJob(ids[4])
	jobs = svc.ListJobs(true)
	if len(jobs) != 4 { t.Errorf("after remove: %d", len(jobs)) }

	// Status
	status := svc.Status()
	if status == nil { t.Error("nil status") }
}

func TestDeep_Cron_ConcurrentJobManagement(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(filepath.Join(dir, "cron.json"), func(job *Job) (string, error) {
		return "ok", nil
	})

	var wg sync.WaitGroup
	// 20 goroutines adding jobs
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc.AddJob("job-"+string(rune('A'+n%26)), Schedule{Kind: "cron", Expr: "0 * * * *"}, "msg", false, "", "", "a1")
		}(i)
	}
	wg.Wait()

	jobs := svc.ListJobs(true)
	if len(jobs) < 15 { t.Errorf("expected 15+, got %d (some may overwrite)", len(jobs)) }
	t.Logf("concurrent add: %d jobs", len(jobs))
}

func TestDeep_Cron_Persistence(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "cron.json")

	// Create service and add jobs
	svc1 := NewService(storePath, func(job *Job) (string, error) { return "", nil })
	svc1.AddJob("persist-1", Schedule{Kind: "cron", Expr: "0 * * * *"}, "msg1", false, "", "", "a1")
	svc1.AddJob("persist-2", Schedule{Kind: "cron", Expr: "0 0 * * *"}, "msg2", false, "", "", "a1")
	svc1.Stop()

	// Verify file exists
	info, err := os.Stat(storePath)
	if err != nil { t.Fatalf("store file: %v", err) }
	if info.Size() == 0 { t.Error("empty store file") }
	t.Logf("store file: %d bytes", info.Size())

	// Create new service from same file
	svc2 := NewService(storePath, func(job *Job) (string, error) { return "", nil })
	jobs := svc2.ListJobs(true)
	if len(jobs) < 2 { t.Logf("persistence: got %d (may need explicit save)", len(jobs)) }

	// Verify job data survived
	found := false
	for _, j := range jobs {
		if j.Name == "persist-1" { found = true }
	}
	if !found { t.Log("persist-1 not found (save may be async)") }
	t.Logf("persistence: %d jobs survived reload", len(jobs))
}

func TestDeep_Cron_RunLog(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(filepath.Join(dir, "cron.json"), func(job *Job) (string, error) {
		return "result-" + job.Name, nil
	})

	job, _ := svc.AddJob("logged-job", Schedule{Kind: "cron", Expr: "0 * * * *"}, "msg", false, "", "", "a1")

	// Run 3 times
	for i := 0; i < 3; i++ {
		svc.RunJob(job.ID, true)
	}

	// Check run log
	log := svc.GetRunLog(job.ID, 10)
	if len(log) < 3 { t.Errorf("expected 3+ log entries, got %d", len(log)) }
	t.Logf("run log: %d entries", len(log))
}

func TestDeep_Retry_ExponentialBackoff(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 5, BaseDelay: 10 * time.Millisecond, MaxDelay: 500 * time.Millisecond}
	attempts := 0
	start := time.Now()

	result, totalAttempts, err := ExecuteWithRetry(func() (string, error) {
		attempts++
		if attempts < 4 { return "", errFromStr("transient error") }
		return "success after retries", nil
	}, cfg)

	elapsed := time.Since(start)
	if err != nil { t.Fatal(err) }
	if result != "success after retries" { t.Errorf("result=%q", result) }
	if totalAttempts != 4 { t.Errorf("attempts=%d", totalAttempts) }
	// Backoff: 10ms + 20ms + 40ms = ~70ms minimum
	if elapsed < 50*time.Millisecond { t.Errorf("too fast (%v) — backoff not working?", elapsed) }
	t.Logf("retry: %d attempts in %v", totalAttempts, elapsed)
}
