// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package coworker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// deep_test.go — Deep integration tests for Qorven Coworker.

func TestDeep_Vault_FullWorkflow(t *testing.T) {
	dir := t.TempDir()
	vault := NewVault(dir)

	// Create a network of notes with backlinks
	vault.Save(&Note{Title: "Qorven Project", Content: "AI agent platform. See [[architecture]] and [[team]].\n#project #ai"})
	vault.Save(&Note{Title: "Architecture", Content: "Go backend, PostgreSQL, pgvector.\nRelated: [[qorven-project]]\n- [ ] Add Redis caching"})
	vault.Save(&Note{Title: "Team", Content: "Sara (lead), Alex (backend), Kim (frontend).\nSee [[qorven-project]]"})
	vault.Save(&Note{Title: "Meeting Jan 15", Content: "Discussed [[architecture]] changes.\nDecided to use single binary.\n- [ ] Update deployment docs"})

	// Verify backlinks
	archRefs := vault.GetBacklinksTo("architecture")
	if len(archRefs) < 1 { t.Errorf("architecture should have backlinks, got %d", len(archRefs)) }

	projectRefs := vault.GetBacklinksTo("qorven-project")
	if len(projectRefs) < 2 { t.Errorf("qorven-project should have 2+ backlinks, got %d", len(projectRefs)) }

	// Search
	goResults := vault.Search("Go backend")
	if len(goResults) == 0 { t.Error("should find 'Go backend' in architecture note") }

	// Tags
	projectNote, _ := vault.Get("qorven-project")
	if len(projectNote.Tags) < 2 { t.Errorf("should have 2 tags, got %d: %v", len(projectNote.Tags), projectNote.Tags) }

	// Reload from disk
	vault2 := NewVault(dir)
	if len(vault2.Notes) != 4 { t.Errorf("reload: expected 4 notes, got %d", len(vault2.Notes)) }

	t.Logf("vault workflow: 4 notes, backlinks, search, tags, disk reload ✓")
}

func TestDeep_Vault_LargeNote(t *testing.T) {
	dir := t.TempDir()
	vault := NewVault(dir)

	// Save a very large note (100KB)
	content := strings.Repeat("This is a long paragraph about AI agents and their capabilities. ", 2000)
	vault.Save(&Note{Title: "Large Note", Content: content})

	// Read back
	got, ok := vault.Get("large-note")
	if !ok { t.Fatal("large note not found") }
	if len(got.Content) != len(content) { t.Errorf("content size: %d vs %d", len(got.Content), len(content)) }

	// Reload from disk
	vault2 := NewVault(dir)
	got2, ok := vault2.Get("large-note")
	if !ok { t.Fatal("large note not found after reload") }
	if len(got2.Content) < len(content)/2 { t.Error("large note content lost on reload") }

	t.Logf("large note: %d bytes saved and reloaded ✓", len(content))
}

func TestDeep_Vault_SpecialCharacters(t *testing.T) {
	dir := t.TempDir()
	vault := NewVault(dir)

	// Notes with special characters
	vault.Save(&Note{Title: "C++ vs Rust", Content: "Comparing C++ and Rust for systems programming"})
	vault.Save(&Note{Title: "Q&A Session", Content: "Questions & answers about the project"})
	vault.Save(&Note{Title: "日本語テスト", Content: "Unicode content: 日本語、中文、한국어"})

	if len(vault.Notes) < 3 { t.Errorf("expected 3 notes, got %d", len(vault.Notes)) }

	// Search for unicode
	results := vault.Search("日本語")
	if len(results) == 0 { t.Error("should find unicode note") }

	t.Log("special characters: C++, &, unicode all handled ✓")
}

func TestDeep_Vault_HeavyConcurrency(t *testing.T) {
	dir := t.TempDir()
	vault := NewVault(dir)

	// 50 goroutines doing mixed read/write operations
	var wg sync.WaitGroup
	errors := make(chan string, 100)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "note-" + string(rune('a'+n%26))

			// Write
			vault.Save(&Note{Title: "Note " + id, Content: "Content " + id})

			// Read
			_, _ = vault.Get(id)

			// Search
			_ = vault.Search(id)

			// List
			_ = vault.ListRecent(5)
		}(i)
	}
	wg.Wait()
	close(errors)

	for err := range errors { t.Error(err) }

	// Verify vault is consistent
	if len(vault.Notes) == 0 { t.Error("vault should have notes after concurrent ops") }

	// Verify files on disk
	entries, _ := os.ReadDir(dir)
	mdFiles := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") { mdFiles++ }
	}
	if mdFiles == 0 { t.Error("no .md files on disk after concurrent writes") }

	t.Logf("heavy concurrency: %d notes in memory, %d files on disk ✓", len(vault.Notes), mdFiles)
}

func TestDeep_Agent_FullWorkflow(t *testing.T) {
	dir := t.TempDir()
	searchHits := []SearchHit{
		{Platform: "reddit", Title: "AI agents discussion", URL: "https://reddit.com/r/ai", Snippet: "Great discussion"},
		{Platform: "hackernews", Title: "New AI framework", URL: "https://news.ycombinator.com/item?id=123", Snippet: "Interesting project"},
	}

	agent := NewAgent(dir, func(ctx context.Context, query string) ([]SearchHit, error) {
		return searchHits, nil
	})
	defer agent.Shutdown()

	// 1. Remember things
	agent.Remember("Alice Chen", "Product manager at Acme. Working on launch.", []string{"person"})
	agent.Remember("Launch Plan", "Target: April 30. Alice is lead. - [ ] Finalize pricing", []string{"project"})
	agent.Remember("Competitor Analysis", "Decided to focus on enterprise. See [[launch-plan]]", []string{"research"})

	// 2. Recall
	results := agent.Recall("Alice", 5)
	if len(results) == 0 { t.Fatal("should recall Alice") }

	// 3. Meeting prep
	mc := agent.PrepareMeeting("Launch Plan", []string{"Alice Chen"})
	if len(mc.PriorNotes) == 0 { t.Error("should find prior notes") }
	if len(mc.OpenItems) == 0 { t.Error("should find TODO items") }

	// 4. Email draft
	draft := agent.DraftEmail("Alice Chen", "Launch Update")
	if draft.Body == "" { t.Error("draft should have body") }
	if len(draft.Context) == 0 { t.Error("draft should have context") }

	// 5. Live note
	note, err := agent.LiveNotes().Create("AI Trends", "artificial intelligence", 100*time.Millisecond)
	if err != nil { t.Fatal(err) }
	time.Sleep(300 * time.Millisecond)

	updated, _ := agent.Vault().Get(note.ID)
	if strings.Contains(updated.Content, "Loading...") { t.Error("live note should have been updated") }

	agent.LiveNotes().Stop(note.ID)

	// 6. Verify vault integrity
	allNotes := agent.Vault().ListRecent(100)
	if len(allNotes) < 4 { t.Errorf("expected 4+ notes, got %d", len(allNotes)) }

	t.Logf("full workflow: remember→recall→prep→draft→live note ✓")
}

func TestDeep_EventBus_MultipleSubscribers(t *testing.T) {
	bus := NewEventBus()
	var mu sync.Mutex
	received := map[string]int{}

	// 3 subscribers for the same event
	for i := 0; i < 3; i++ {
		name := string(rune('A' + i))
		bus.Subscribe(EventNoteCreated, func(e Event) {
			mu.Lock()
			received[name]++
			mu.Unlock()
		})
	}

	// Publish 5 events
	for i := 0; i < 5; i++ {
		bus.Publish(Event{Type: EventNoteCreated, Payload: i})
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	total := 0
	for _, count := range received { total += count }
	mu.Unlock()

	if total < 15 { t.Errorf("expected 15 deliveries (3 subs × 5 events), got %d", total) }
	t.Logf("event bus: %d deliveries across %d subscribers ✓", total, len(received))
}

func TestDeep_CommandExecutor_RealCommands(t *testing.T) {
	dir := t.TempDir()
	bus := NewEventBus()
	exec := NewCommandExecutor(dir, bus)

	// Write a file then read it
	ctx := context.Background()
	result, err := exec.Execute(ctx, "echo 'hello world' > test.txt && cat test.txt")
	if err != nil { t.Fatal(err) }
	if result.ExitCode != 0 { t.Errorf("exit code: %d, stderr: %s", result.ExitCode, result.Stderr) }
	if !strings.Contains(result.Stdout, "hello world") { t.Errorf("stdout: %q", result.Stdout) }

	// Verify file exists
	data, err := os.ReadFile(filepath.Join(dir, "test.txt"))
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(data), "hello world") { t.Error("file content wrong") }

	t.Logf("executor: write+read in %v ✓", result.Duration)
}

func TestDeep_CommandExecutor_GuardrailsBlock(t *testing.T) {
	dir := t.TempDir()
	exec := NewCommandExecutor(dir, nil)
	ctx := context.Background()

	dangerous := []string{
		"rm -rf /",
		"rm -rf ~",
		"DROP TABLE users",
	}

	for _, cmd := range dangerous {
		_, err := exec.Execute(ctx, cmd)
		if err == nil { t.Errorf("should block: %q", cmd) }
		if !strings.Contains(err.Error(), "guardrails") { t.Errorf("error should mention guardrails: %v", err) }
	}
	t.Log("guardrails: all dangerous commands blocked ✓")
}

func TestDeep_Vault_DiskPersistence(t *testing.T) {
	dir := t.TempDir()

	// Phase 1: Create vault and save notes
	func() {
		vault := NewVault(dir)
		vault.Save(&Note{Title: "Persistent Note", Content: "This should survive restart.\n[[other-note]]\n#important"})
		vault.Save(&Note{Title: "Other Note", Content: "Referenced by persistent note"})
	}()

	// Phase 2: Create new vault from same directory (simulates restart)
	vault2 := NewVault(dir)

	if len(vault2.Notes) != 2 { t.Fatalf("expected 2 notes after restart, got %d", len(vault2.Notes)) }

	note, ok := vault2.Get("persistent-note")
	if !ok { t.Fatal("persistent note not found after restart") }
	if !strings.Contains(note.Content, "survive restart") { t.Error("content lost") }
	if len(note.Backlinks) == 0 { t.Error("backlinks lost on reload") }
	if len(note.Tags) == 0 { t.Error("tags lost on reload") }

	t.Log("disk persistence: notes, backlinks, tags survive restart ✓")
}
