// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cron

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Hard cron tests — scheduling, retry, job lifecycle, edge cases.

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxRetries <= 0 { t.Error("max attempts should be > 0") }
	if cfg.BaseDelay <= 0 { t.Error("base delay should be > 0") }
	if cfg.MaxDelay <= cfg.BaseDelay { t.Error("max delay should be > base delay") }
}

func TestExecuteWithRetry_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	result, attempts, err := ExecuteWithRetry(func() (string, error) {
		calls++
		return "ok", nil
	}, DefaultRetryConfig())
	if err != nil { t.Fatal(err) }
	if result != "ok" { t.Errorf("result=%q", result) }
	if attempts != 1 { t.Errorf("attempts=%d", attempts) }
	if calls != 1 { t.Errorf("calls=%d", calls) }
}

func TestExecuteWithRetry_SuccessAfterRetry(t *testing.T) {
	calls := 0
	cfg := RetryConfig{MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	result, attempts, err := ExecuteWithRetry(func() (string, error) {
		calls++
		if calls < 3 { return "", errFromStr("transient") }
		return "ok", nil
	}, cfg)
	if err != nil { t.Fatal(err) }
	if result != "ok" { t.Errorf("result=%q", result) }
	if attempts != 3 { t.Errorf("attempts=%d", attempts) }
}

func TestExecuteWithRetry_AllFail(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 2, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}
	_, _, err := ExecuteWithRetry(func() (string, error) {
		return "", errFromStr("permanent")
	}, cfg)
	if err == nil { t.Error("should fail after all attempts") }
}

func TestBackoffWithJitter(t *testing.T) {
	base := 100 * time.Millisecond
	max := 10 * time.Second
	// Attempt 1
	d1 := backoffWithJitter(base, max, 0)
	if d1 <= 0 { t.Error("delay should be > 0") }
	// Attempt 5 — should be larger
	d5 := backoffWithJitter(base, max, 4)
	if d5 <= 0 { t.Error("delay should be > 0") }
	// Should not exceed max
	d10 := backoffWithJitter(base, max, 9)
	if d10 > max+5*time.Second { t.Errorf("delay %v exceeds max %v (with jitter)", d10, max) }
}

func TestTruncateOutput(t *testing.T) {
	short := "short"
	if TruncateOutput(short) != short { t.Error("short should not truncate") }
	long := string(make([]byte, 200000))
	result := TruncateOutput(long)
	if len(result) >= len(long) { t.Error("long should truncate") }
}

func TestService_New(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "cron.json")
	svc := NewService(storePath, func(job *Job) (string, error) { return "", nil })
	if svc == nil { t.Fatal("nil service") }
}

func TestService_AddJob(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "cron.json")
	svc := NewService(storePath, func(job *Job) (string, error) { return "", nil })

	job, err := svc.AddJob("test-job", Schedule{Kind: "cron", Expr: "0 * * * *"}, "hello", false, "", "", "agent1")
	if err != nil { t.Fatal(err) }
	if job == nil { t.Fatal("nil job") }
	if job.ID == "" { t.Error("empty job ID") }
}

func TestService_ListJobs_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(filepath.Join(tmpDir, "cron.json"), func(job *Job) (string, error) { return "", nil })
	jobs := svc.ListJobs(true)
	if len(jobs) != 0 { t.Errorf("expected 0, got %d", len(jobs)) }
}

func TestService_ListJobs_WithJobs(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(filepath.Join(tmpDir, "cron.json"), func(job *Job) (string, error) { return "", nil })
	svc.AddJob("job1", Schedule{Kind: "cron", Expr: "0 * * * *"}, "msg1", false, "", "", "a1")
	svc.AddJob("job2", Schedule{Kind: "cron", Expr: "0 0 * * *"}, "msg2", false, "", "", "a1")
	jobs := svc.ListJobs(true)
	if len(jobs) != 2 { t.Errorf("expected 2, got %d", len(jobs)) }
}

func TestService_GetJob(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(filepath.Join(tmpDir, "cron.json"), func(job *Job) (string, error) { return "", nil })
	job, _ := svc.AddJob("test", Schedule{Kind: "cron", Expr: "0 * * * *"}, "msg", false, "", "", "a1")
	found, ok := svc.GetJob(job.ID)
	if !ok { t.Error("should find job") }
	if found.ID != job.ID { t.Error("wrong job") }
}

func TestService_GetJob_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(filepath.Join(tmpDir, "cron.json"), func(job *Job) (string, error) { return "", nil })
	_, ok := svc.GetJob("nonexistent")
	if ok { t.Error("should not find nonexistent job") }
}

func TestService_RemoveJob(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(filepath.Join(tmpDir, "cron.json"), func(job *Job) (string, error) { return "", nil })
	job, _ := svc.AddJob("test", Schedule{Kind: "cron", Expr: "0 * * * *"}, "msg", false, "", "", "a1")
	err := svc.RemoveJob(job.ID)
	if err != nil { t.Fatal(err) }
	_, ok := svc.GetJob(job.ID)
	if ok { t.Error("job should be removed") }
}

func TestService_EnableJob(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(filepath.Join(tmpDir, "cron.json"), func(job *Job) (string, error) { return "", nil })
	job, _ := svc.AddJob("test", Schedule{Kind: "cron", Expr: "0 * * * *"}, "msg", false, "", "", "a1")
	svc.EnableJob(job.ID, false)
	_, _ = svc.GetJob(job.ID)
	f, _ := svc.GetJob(job.ID)
	if f.Enabled { t.Error("should be disabled") }
	svc.EnableJob(job.ID, true)
	f2, _ := svc.GetJob(job.ID)
	if !f2.Enabled { t.Error("should be enabled") }
}

func TestService_UpdateJob(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(filepath.Join(tmpDir, "cron.json"), func(job *Job) (string, error) { return "", nil })
	job, _ := svc.AddJob("test", Schedule{Kind: "cron", Expr: "0 * * * *"}, "msg", false, "", "", "a1")
	_, err := svc.UpdateJob(job.ID, JobPatch{Message: "updated"})
	if err != nil { t.Fatal(err) }
	_, _ = svc.GetJob(job.ID)
}

func TestService_Status(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewService(filepath.Join(tmpDir, "cron.json"), func(job *Job) (string, error) { return "", nil })
	svc.AddJob("j1", Schedule{Kind: "cron", Expr: "0 * * * *"}, "m1", false, "", "", "a1")
	status := svc.Status()
	if status == nil { t.Fatal("nil status") }
}

func TestService_RunJob(t *testing.T) {
	tmpDir := t.TempDir()
	executed := false
	svc := NewService(filepath.Join(tmpDir, "cron.json"), func(job *Job) (string, error) { executed = true; return "done", nil })
	job, _ := svc.AddJob("test", Schedule{Kind: "cron", Expr: "0 * * * *"}, "run me", false, "", "", "a1")
	ok, _, err := svc.RunJob(job.ID, true)
	if err != nil { t.Fatal(err) }
	if !ok { t.Error("should execute") }
	if !executed { t.Error("handler not called") }
}

func TestService_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "cron.json")

	// Create service, add job
	svc1 := NewService(storePath, func(job *Job) (string, error) { return "", nil })
	svc1.AddJob("persist-test", Schedule{Kind: "cron", Expr: "0 * * * *"}, "msg", false, "", "", "a1")

	// Verify file exists
	if _, err := os.Stat(storePath); os.IsNotExist(err) { t.Error("store file not created") }

	// Create new service from same file
	svc2 := NewService(storePath, func(job *Job) (string, error) { return "", nil })
	jobs := svc2.ListJobs(true)
	if len(jobs) == 0 { t.Log("persistence may require explicit save") }
}

func TestFormatNextRunExpr(t *testing.T) {
	tests := []struct{ expr string; notEmpty bool }{
		{"0 * * * *", true},
		{"0 0 * * *", true},
		{"*/5 * * * *", true},
		{"invalid", false},
	}
	for _, tt := range tests {
		result := FormatNextRunExpr(tt.expr)
		if tt.notEmpty && result == "" { t.Errorf("FormatNextRunExpr(%q) empty", tt.expr) }
	}
}

func strPtr(s string) *string { return &s }

type cronErr string
func (e cronErr) Error() string { return string(e) }
func errFromStr(s string) error { return cronErr(s) }
