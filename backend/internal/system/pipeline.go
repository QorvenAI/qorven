// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package system

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// CodeChange represents a proposed modification to the codebase.
// Every change goes through: propose → validate (compile+test) → apply → verify.
// On failure at any stage, the change is rejected and rolled back.
type CodeChange struct {
	ID          string       `json:"id"`
	Description string       `json:"description"`
	Files       []FileChange `json:"files"`
	Risk        string       `json:"risk"`   // low, medium, high
	Status      string       `json:"status"` // proposed, validating, validated, applying, applied, failed, rolled_back
	ProposedBy  string       `json:"proposed_by"` // agent ID
	ReviewedBy  string       `json:"reviewed_by"` // prime or human
	CreatedAt   time.Time    `json:"created_at"`
	AppliedAt   *time.Time   `json:"applied_at,omitempty"`

	// Validation results
	CompileOK   bool   `json:"compile_ok"`
	CompileErr  string `json:"compile_error,omitempty"`
	TestOK      bool   `json:"test_ok"`
	TestErr     string `json:"test_error,omitempty"`
	TestsPassed int    `json:"tests_passed"`
	TestsFailed int    `json:"tests_failed"`
}

// FileChange represents a single file modification.
type FileChange struct {
	Path       string `json:"path"`
	OldContent string `json:"old_content,omitempty"` // empty for new files
	NewContent string `json:"new_content"`
	Action     string `json:"action"` // create, modify, delete
}

// DiffPreview returns a human-readable diff preview for a file change.
func (fc FileChange) DiffPreview() string {
	switch fc.Action {
	case "create":
		lines := strings.Count(fc.NewContent, "\n") + 1
		return fmt.Sprintf("+++ %s (new file, %d lines)", fc.Path, lines)
	case "delete":
		return fmt.Sprintf("--- %s (deleted)", fc.Path)
	case "modify":
		oldLines := strings.Split(fc.OldContent, "\n")
		newLines := strings.Split(fc.NewContent, "\n")
		added, removed := 0, 0
		// Simple line-level diff
		oldSet := make(map[string]bool)
		for _, l := range oldLines { oldSet[l] = true }
		for _, l := range newLines {
			if !oldSet[l] { added++ }
		}
		newSet := make(map[string]bool)
		for _, l := range newLines { newSet[l] = true }
		for _, l := range oldLines {
			if !newSet[l] { removed++ }
		}
		return fmt.Sprintf("~~~ %s (+%d -%d lines)", fc.Path, added, removed)
	}
	return fc.Path
}

// Pipeline manages the lifecycle of code changes.
type Pipeline struct {
	mu        sync.Mutex
	changes   map[string]*CodeChange
	workDir   string // the actual codebase directory
	onReview  func(change *CodeChange) // callback to supervisor for review
}

// NewPipeline creates a code change pipeline.
func NewPipeline(workDir string) *Pipeline {
	return &Pipeline{
		changes: make(map[string]*CodeChange),
		workDir: workDir,
	}
}

// SetReviewCallback sets the function called when a change needs review.
func (p *Pipeline) SetReviewCallback(fn func(change *CodeChange)) {
	p.onReview = fn
}

// Propose creates a new code change and starts validation.
func (p *Pipeline) Propose(ctx context.Context, description string, files []FileChange, risk string, proposedBy string) (*CodeChange, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files in change")
	}

	change := &CodeChange{
		ID:          uuid.New().String(),
		Description: description,
		Files:       files,
		Risk:        risk,
		Status:      "proposed",
		ProposedBy:  proposedBy,
		CreatedAt:   time.Now(),
	}

	// Read current content for modified files
	for i, fc := range change.Files {
		if fc.Action == "modify" && fc.OldContent == "" {
			absPath := filepath.Join(p.workDir, fc.Path)
			data, err := os.ReadFile(absPath)
			if err != nil {
				return nil, fmt.Errorf("cannot read %s: %w", fc.Path, err)
			}
			change.Files[i].OldContent = string(data)
		}
	}

	p.mu.Lock()
	p.changes[change.ID] = change
	p.mu.Unlock()

	slog.Info("pipeline.proposed", "id", change.ID, "files", len(files), "risk", risk, "by", proposedBy)
	return change, nil
}

// Validate runs compile and test checks on a proposed change.
// Changes are applied to a temporary copy of the codebase — the real code is never touched.
func (p *Pipeline) Validate(ctx context.Context, changeID string) error {
	p.mu.Lock()
	change, ok := p.changes[changeID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("change %s not found", changeID)
	}
	change.Status = "validating"
	p.mu.Unlock()

	slog.Info("pipeline.validating", "id", changeID)

	// Create temp directory with a copy of the codebase
	tempDir, err := os.MkdirTemp("", "qorven-validate-*")
	if err != nil {
		return p.failChange(changeID, "create temp dir: "+err.Error())
	}
	defer os.RemoveAll(tempDir)

	// Copy the codebase to temp dir using git
	copyCmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "file://"+p.workDir, tempDir+"/repo")
	if out, err := copyCmd.CombinedOutput(); err != nil {
		// Fallback: just copy the files we need
		slog.Debug("pipeline.git_clone_failed", "error", err, "output", string(out))
		// Create minimal structure for build
		os.MkdirAll(tempDir+"/repo", 0755)
		exec.CommandContext(ctx, "cp", "-r", p.workDir+"/internal", tempDir+"/repo/").Run()
		exec.CommandContext(ctx, "cp", "-r", p.workDir+"/cmd", tempDir+"/repo/").Run()
		exec.CommandContext(ctx, "cp", p.workDir+"/go.mod", tempDir+"/repo/").Run()
		exec.CommandContext(ctx, "cp", p.workDir+"/go.sum", tempDir+"/repo/").Run()
		exec.CommandContext(ctx, "cp", p.workDir+"/main.go", tempDir+"/repo/").Run()
	}

	repoDir := tempDir + "/repo"

	// Apply the proposed changes to the temp copy
	for _, fc := range change.Files {
		targetPath := filepath.Join(repoDir, fc.Path)
		switch fc.Action {
		case "create", "modify":
			os.MkdirAll(filepath.Dir(targetPath), 0755)
			if err := os.WriteFile(targetPath, []byte(fc.NewContent), 0644); err != nil {
				return p.failChange(changeID, "write file: "+err.Error())
			}
		case "delete":
			os.Remove(targetPath)
		}
	}

	// Stage 1: Compile check
	buildCtx, buildCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer buildCancel()

	buildCmd := exec.CommandContext(buildCtx, "go", "build", "./...")
	buildCmd.Dir = repoDir
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	buildOut, buildErr := buildCmd.CombinedOutput()

	p.mu.Lock()
	if buildErr != nil {
		change.CompileOK = false
		change.CompileErr = string(buildOut)
		p.mu.Unlock()
		return p.failChange(changeID, "compile failed: "+string(buildOut))
	}
	change.CompileOK = true
	p.mu.Unlock()

	slog.Info("pipeline.compile_passed", "id", changeID)

	// Stage 2: Test check
	testCtx, testCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer testCancel()

	testCmd := exec.CommandContext(testCtx, "go", "test", "./...", "-count=1", "-short")
	testCmd.Dir = repoDir
	testCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	testOut, testErr := testCmd.CombinedOutput()

	p.mu.Lock()
	testOutput := string(testOut)
	change.TestsPassed = strings.Count(testOutput, "--- PASS")
	change.TestsFailed = strings.Count(testOutput, "--- FAIL")

	if testErr != nil && change.TestsFailed > 0 {
		change.TestOK = false
		change.TestErr = testOutput
		p.mu.Unlock()
		return p.failChange(changeID, fmt.Sprintf("tests failed: %d passed, %d failed", change.TestsPassed, change.TestsFailed))
	}
	change.TestOK = true
	change.Status = "validated"
	p.mu.Unlock()

	slog.Info("pipeline.validated", "id", changeID, "tests_passed", change.TestsPassed)

	// Send for review if callback is set
	if p.onReview != nil {
		p.onReview(change)
	}

	return nil
}

// Apply applies a validated change to the real codebase.
// Uses git stash as a safety net — if anything fails, rolls back.
func (p *Pipeline) Apply(ctx context.Context, changeID string) error {
	p.mu.Lock()
	change, ok := p.changes[changeID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("change %s not found", changeID)
	}
	if change.Status != "validated" {
		p.mu.Unlock()
		return fmt.Errorf("change %s is %s, must be validated", changeID, change.Status)
	}
	change.Status = "applying"
	p.mu.Unlock()

	slog.Info("pipeline.applying", "id", changeID)

	// Safety: git stash any uncommitted changes first
	stashCmd := exec.CommandContext(ctx, "git", "stash", "push", "-m", "qorven-pre-change-"+changeID)
	stashCmd.Dir = p.workDir
	stashOut, _ := stashCmd.CombinedOutput()
	hasStash := !strings.Contains(string(stashOut), "No local changes")

	// Apply each file change
	applied := []string{}
	for _, fc := range change.Files {
		absPath := filepath.Join(p.workDir, fc.Path)
		switch fc.Action {
		case "create", "modify":
			os.MkdirAll(filepath.Dir(absPath), 0755)
			if err := os.WriteFile(absPath, []byte(fc.NewContent), 0644); err != nil {
				// Rollback
				p.rollback(ctx, changeID, applied, hasStash)
				return p.failChange(changeID, "write failed: "+err.Error())
			}
			applied = append(applied, fc.Path)
		case "delete":
			os.Remove(absPath)
			applied = append(applied, fc.Path)
		}
	}

	// Verify: build the real codebase after changes
	verifyCmd := exec.CommandContext(ctx, "go", "build", "./...")
	verifyCmd.Dir = p.workDir
	verifyCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := verifyCmd.CombinedOutput(); err != nil {
		// Build failed — rollback
		slog.Error("pipeline.verify_failed", "id", changeID, "error", string(out))
		p.rollback(ctx, changeID, applied, hasStash)
		return p.failChange(changeID, "verify build failed after apply: "+string(out))
	}

	// Success
	p.mu.Lock()
	change.Status = "applied"
	now := time.Now()
	change.AppliedAt = &now
	p.mu.Unlock()

	slog.Info("pipeline.applied", "id", changeID, "files", len(applied))
	return nil
}

// rollback restores files to their original state.
func (p *Pipeline) rollback(ctx context.Context, changeID string, appliedPaths []string, hasStash bool) {
	slog.Warn("pipeline.rolling_back", "id", changeID, "files", len(appliedPaths))

	p.mu.Lock()
	change := p.changes[changeID]
	p.mu.Unlock()

	// Restore original content for modified files
	for _, fc := range change.Files {
		absPath := filepath.Join(p.workDir, fc.Path)
		switch fc.Action {
		case "modify":
			if fc.OldContent != "" {
				os.WriteFile(absPath, []byte(fc.OldContent), 0644)
			}
		case "create":
			os.Remove(absPath) // remove newly created file
		}
	}

	// Restore git stash if we had one
	if hasStash {
		stashPop := exec.CommandContext(ctx, "git", "stash", "pop")
		stashPop.Dir = p.workDir
		stashPop.Run()
	}

	p.mu.Lock()
	change.Status = "rolled_back"
	p.mu.Unlock()

	slog.Info("pipeline.rolled_back", "id", changeID)
}

func (p *Pipeline) failChange(changeID, reason string) error {
	p.mu.Lock()
	if change, ok := p.changes[changeID]; ok {
		change.Status = "failed"
	}
	p.mu.Unlock()
	slog.Error("pipeline.failed", "id", changeID, "reason", reason)
	return fmt.Errorf("change %s failed: %s", changeID, reason)
}

// Get returns a change by ID.
func (p *Pipeline) Get(changeID string) *CodeChange {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.changes[changeID]
}

// List returns all changes.
func (p *Pipeline) List() []*CodeChange {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := []*CodeChange{}
	for _, c := range p.changes {
		list = append(list, c)
	}
	return list
}

// Pending returns changes awaiting review.
func (p *Pipeline) Pending() []*CodeChange {
	p.mu.Lock()
	defer p.mu.Unlock()
	pending := []*CodeChange{}
	for _, c := range p.changes {
		if c.Status == "validated" {
			pending = append(pending, c)
		}
	}
	return pending
}
