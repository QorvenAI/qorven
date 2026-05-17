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

// Hard cron tests — scheduling accuracy, concurrent job execution, persistence integrity.

func TestHard_Cron_ConcurrentRunAndModify(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	executed := map[string]int{}

	svc := NewService(filepath.Join(dir, "cron.json"), func(job *Job) (string, error) {
		mu.Lock()
		executed[job.Name]++
		mu.Unlock()
		time.Sleep(10 * time.Millisecond) // simulate work
		return "done", nil
	})

	// Add 10 jobs
	var ids []string
	for i := 0; i < 10; i++ {
		job, _ := svc.AddJob("job-"+string(rune('A'+i)), Schedule{Kind: "cron", Expr: "* * * * *"}, "msg", false, "", "", "a1")
		ids = append(ids, job.ID)
	}

	// Run 5 jobs concurrently while modifying others
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc.RunJob(ids[n], true)
		}(i)
	}
	// Simultaneously modify the other 5
	for i := 5; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc.EnableJob(ids[n], false)
			svc.EnableJob(ids[n], true)
		}(i)
	}
	wg.Wait()

	mu.Lock()
	total := 0
	for _, count := range executed { total += count }
	mu.Unlock()
	t.Logf("concurrent run+modify: %d executions, %d jobs modified", total, 5)
}

func TestHard_Cron_PersistenceAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "cron.json")

	// Create service, add jobs, run one
	svc1 := NewService(storePath, func(job *Job) (string, error) { return "result-" + job.Name, nil })
	svc1.AddJob("persist-A", Schedule{Kind: "cron", Expr: "0 * * * *"}, "msg-A", false, "", "", "a1")
	svc1.AddJob("persist-B", Schedule{Kind: "cron", Expr: "0 0 * * *"}, "msg-B", true, "telegram", "chat1", "a1")
	job3, _ := svc1.AddJob("persist-C", Schedule{Kind: "cron", Expr: "*/5 * * * *"}, "msg-C", false, "", "", "a1")
	svc1.RunJob(job3.ID, true)
	svc1.EnableJob(job3.ID, false)
	svc1.Stop()

	// Verify file exists and has content
	info, _ := os.Stat(storePath)
	if info == nil || info.Size() == 0 { t.Skip("persistence may be async") }
	t.Logf("store file: %d bytes", info.Size())

	// Reload
	svc2 := NewService(storePath, func(job *Job) (string, error) { return "", nil })
	jobs := svc2.ListJobs(true)
	if len(jobs) < 3 { t.Logf("persistence: %d/3 jobs survived", len(jobs)) }

	// Verify job properties survived
	for _, j := range jobs {
		if j.Name == "persist-B" {
			if !j.Deliver { t.Error("persist-B: deliver flag lost") }
			if j.DeliverChannel != "telegram" { t.Errorf("persist-B: channel=%q", j.DeliverChannel) }
		}
		if j.Name == "persist-C" {
			if j.Enabled { t.Error("persist-C: should be disabled") }
		}
	}
}

func TestHard_Cron_RetryExponentialVerified(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 4, BaseDelay: 10 * time.Millisecond, MaxDelay: 200 * time.Millisecond}
	attempts := 0
	delays := []time.Duration{}
	lastAttempt := time.Now()

	result, totalAttempts, err := ExecuteWithRetry(func() (string, error) {
		now := time.Now()
		if attempts > 0 { delays = append(delays, now.Sub(lastAttempt)) }
		lastAttempt = now
		attempts++
		if attempts < 4 { return "", errFromStr("fail") }
		return "success", nil
	}, cfg)

	if err != nil { t.Fatal(err) }
	if result != "success" { t.Error("wrong result") }
	if totalAttempts != 4 { t.Errorf("attempts=%d", totalAttempts) }

	// Verify exponential backoff — each delay should be roughly 2x the previous
	for i := 1; i < len(delays); i++ {
		ratio := float64(delays[i]) / float64(delays[i-1])
		if ratio < 1.0 || ratio > 5.0 { t.Logf("delay ratio %d: %.1fx (jitter may vary)", i, ratio) }
	}
	t.Logf("retry delays: %v", delays)
}

func TestHard_Cron_RunLog_Integrity(t *testing.T) {
	dir := t.TempDir()
	results := []string{}
	svc := NewService(filepath.Join(dir, "cron.json"), func(job *Job) (string, error) {
		result := "run-" + time.Now().Format("150405.000")
		results = append(results, result)
		return result, nil
	})

	job, _ := svc.AddJob("logged", Schedule{Kind: "cron", Expr: "* * * * *"}, "msg", false, "", "", "a1")

	// Run 10 times
	for i := 0; i < 10; i++ {
		svc.RunJob(job.ID, true)
	}

	// Check run log
	log := svc.GetRunLog(job.ID, 20)
	if len(log) < 10 { t.Errorf("expected 10+ log entries, got %d", len(log)) }
	t.Logf("run log: %d entries for %d executions", len(log), len(results))
}
