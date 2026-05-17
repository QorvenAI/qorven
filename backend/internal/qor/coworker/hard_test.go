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

// hard_test.go — Diamond-hard tests for Qorven Coworker.

// ── Vault: Real disk I/O ──

func TestHard_Vault_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	vault := NewVault(dir)

	// Save a note
	note := &Note{Title: "Meeting Notes", Content: "Discussed [[project-alpha]] with team.\n#meeting #important", CreatedAt: time.Now()}
	err := vault.Save(note)
	if err != nil { t.Fatal(err) }

	// Verify file on disk
	path := filepath.Join(dir, note.ID+".md")
	data, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(data), "Meeting Notes") { t.Error("file should contain title") }
	if !strings.Contains(string(data), "project-alpha") { t.Error("file should contain backlink text") }

	// Load from disk (simulates restart)
	vault2 := NewVault(dir)
	got, ok := vault2.Get(note.ID)
	if !ok { t.Fatal("note should survive reload from disk") }
	if got.Title != "Meeting Notes" { t.Errorf("title after reload: %q", got.Title) }
}

func TestHard_Vault_Backlinks(t *testing.T) {
	dir := t.TempDir()
	vault := NewVault(dir)

	vault.Save(&Note{Title: "Project Alpha", Content: "Main project page"})
	vault.Save(&Note{Title: "Meeting Jan 5", Content: "Discussed [[project-alpha]] timeline"})
	vault.Save(&Note{Title: "Meeting Jan 12", Content: "Follow up on [[project-alpha]] and [[budget]]"})

	// Find backlinks to project-alpha
	refs := vault.GetBacklinksTo("project-alpha")
	if len(refs) < 2 { t.Errorf("expected 2 backlinks to project-alpha, got %d", len(refs)) }
}

func TestHard_Vault_Search(t *testing.T) {
	dir := t.TempDir()
	vault := NewVault(dir)

	vault.Save(&Note{Title: "Go Tutorial", Content: "How to write Go programs"})
	vault.Save(&Note{Title: "Python Guide", Content: "How to write Python scripts"})
	vault.Save(&Note{Title: "Go Concurrency", Content: "Goroutines and channels in Go"})

	results := vault.Search("Go")
	if len(results) < 2 { t.Errorf("expected 2 Go results, got %d", len(results)) }

	// Should not find Python
	for _, r := range results {
		if strings.Contains(r.Title, "Python") { t.Error("Go search should not return Python") }
	}
}

func TestHard_Vault_Delete(t *testing.T) {
	dir := t.TempDir()
	vault := NewVault(dir)

	vault.Save(&Note{Title: "Temporary Note", Content: "Delete me"})
	id := slugify("Temporary Note")

	_, ok := vault.Get(id)
	if !ok { t.Fatal("note should exist before delete") }

	vault.Delete(id)
	_, ok = vault.Get(id)
	if ok { t.Error("note should not exist after delete") }

	// File should be gone from disk
	_, err := os.Stat(filepath.Join(dir, id+".md"))
	if !os.IsNotExist(err) { t.Error("file should be deleted from disk") }
}

func TestHard_Vault_ConcurrentSave(t *testing.T) {
	dir := t.TempDir()
	vault := NewVault(dir)

	// 20 goroutines saving different notes simultaneously
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			vault.Save(&Note{
				Title:   "Concurrent Note " + string(rune('A'+n)),
				Content: "Content " + string(rune('A'+n)),
			})
		}(i)
	}
	wg.Wait()

	// Should have ~20 notes (some may collide on slugify)
	if len(vault.Notes) < 15 { t.Errorf("expected ~20 notes, got %d", len(vault.Notes)) }
}

// ── Tags and Slugify ──

func TestHard_ExtractTags(t *testing.T) {
	tags := extractTags("This is about #golang and #testing with #ci")
	if len(tags) != 3 { t.Errorf("expected 3 tags, got %d: %v", len(tags), tags) }
}

func TestHard_Slugify(t *testing.T) {
	cases := map[string]string{
		"Meeting Notes":       "meeting-notes",
		"Project Alpha v2.0":  "project-alpha-v2-0",
		"  Spaces  Everywhere ": "spaces-everywhere",
		"UPPERCASE":           "uppercase",
	}
	for input, expected := range cases {
		got := slugify(input)
		if got != expected { t.Errorf("slugify(%q) = %q, want %q", input, got, expected) }
	}
}

func TestHard_ExtractBacklinks(t *testing.T) {
	links := extractBacklinks("See [[project-alpha]] and [[meeting-notes]] for details. Also [[project-alpha]] again.")
	if len(links) != 2 { t.Errorf("expected 2 unique backlinks, got %d: %v", len(links), links) }
}

// ── Agent: Remember + Recall ──

func TestHard_Agent_RememberAndRecall(t *testing.T) {
	dir := t.TempDir()
	agent := NewAgent(dir, nil)
	defer agent.Shutdown()

	agent.Remember("John Smith", "VP of Engineering at Acme Corp. Met at conference.", []string{"person", "contact"})
	agent.Remember("Project Qorven", "AI agent platform. Go backend. 97K lines.", []string{"project"})
	agent.Remember("Budget Q2", "Approved $50K for infrastructure.", []string{"finance"})

	// Recall should find relevant notes
	results := agent.Recall("Qorven", 5)
	if len(results) == 0 { t.Fatal("should recall Qorven note") }
	found := false
	for _, r := range results {
		if strings.Contains(r.Content, "AI agent") { found = true }
	}
	if !found { t.Error("recall should find the Qorven note content") }
}

// ── Agent: Meeting Prep ──

func TestHard_Agent_MeetingPrep(t *testing.T) {
	dir := t.TempDir()
	agent := NewAgent(dir, nil)
	defer agent.Shutdown()

	agent.Remember("John Smith", "VP Engineering. Likes Go. - [ ] Follow up on API design", []string{"person"})
	agent.Remember("API Design Review", "Decided to use REST over GraphQL. John was in favor.", []string{"meeting"})

	mc := agent.PrepareMeeting("API Design Review", []string{"John Smith"})
	if len(mc.PriorNotes) == 0 { t.Error("should find prior notes about attendees/topic") }
	if len(mc.OpenItems) == 0 { t.Error("should extract open TODO items") }
}

// ── Agent: Email Draft ──

func TestHard_Agent_EmailDraft(t *testing.T) {
	dir := t.TempDir()
	agent := NewAgent(dir, nil)
	defer agent.Shutdown()

	agent.Remember("Alice Chen", "Product manager. Working on launch plan.", []string{"person"})
	agent.Remember("Launch Plan", "Target date: April 30. Alice is lead.", []string{"project"})

	draft := agent.DraftEmail("Alice Chen", "Launch Plan Update")
	if len(draft.Context) == 0 { t.Error("draft should have context from vault") }
	if draft.Body == "" { t.Error("draft body should not be empty") }
}

// ── Skills Registry ──

func TestHard_Skills_DefaultsRegistered(t *testing.T) {
	sr := NewSkillRegistry()
	skills := sr.List()
	if len(skills) != 5 { t.Errorf("expected 5 default skills, got %d", len(skills)) }

	// All should be enabled by default
	for _, s := range skills {
		if !s.Enabled { t.Errorf("skill %s should be enabled by default", s.Name) }
	}
}

func TestHard_Skills_EnableDisable(t *testing.T) {
	sr := NewSkillRegistry()
	sr.Disable(SkillDeletionGuardrails)
	s, _ := sr.Get(SkillDeletionGuardrails)
	if s.Enabled { t.Error("should be disabled") }

	sr.Enable(SkillDeletionGuardrails)
	s2, _ := sr.Get(SkillDeletionGuardrails)
	if !s2.Enabled { t.Error("should be re-enabled") }
}

// ── Event Bus ──

func TestHard_EventBus_PublishSubscribe(t *testing.T) {
	bus := NewEventBus()
	received := make(chan Event, 1)

	bus.Subscribe(EventNoteCreated, func(e Event) { received <- e })
	bus.Publish(Event{Type: EventNoteCreated, Payload: "test note", Source: "test"})

	select {
	case e := <-received:
		if e.Payload != "test note" { t.Errorf("payload: %v", e.Payload) }
		if e.Timestamp.IsZero() { t.Error("timestamp should be set") }
	case <-time.After(time.Second):
		t.Fatal("event not received within 1s")
	}
}

// ── Deletion Guardrails ──

func TestHard_Guardrails_BlocksDangerous(t *testing.T) {
	g := NewDeletionGuardrails()
	dangerous := []string{
		"rm -rf /",
		"rm -rf ~",
		"DROP TABLE users",
		"DELETE FROM agents",
		"dd if=/dev/zero of=/dev/sda",
	}
	for _, cmd := range dangerous {
		if !g.IsDangerous(cmd) { t.Errorf("should block: %q", cmd) }
	}
}

func TestHard_Guardrails_AllowsSafe(t *testing.T) {
	g := NewDeletionGuardrails()
	safe := []string{
		"ls -la",
		"cat file.txt",
		"go build ./...",
		"git status",
		"rm temp.txt", // single file is ok
	}
	for _, cmd := range safe {
		if g.IsDangerous(cmd) { t.Errorf("should allow: %q", cmd) }
	}
}

// ── Message Queue ──

func TestHard_MessageQueue_PushPop(t *testing.T) {
	q := NewMessageQueue(100)
	q.Push(Message{Type: "chat", Content: "hello"})
	q.Push(Message{Type: "chat", Content: "world"})

	if q.Len() != 2 { t.Errorf("expected 2, got %d", q.Len()) }

	msg, ok := q.Pop()
	if !ok { t.Fatal("should pop") }
	if msg.Content != "hello" { t.Errorf("first message: %q", msg.Content) }
	if msg.ID == "" { t.Error("message should have auto-generated ID") }
}

func TestHard_MessageQueue_MaxSize(t *testing.T) {
	q := NewMessageQueue(5)
	for i := 0; i < 10; i++ {
		q.Push(Message{Type: "test", Content: string(rune('A' + i))})
	}
	if q.Len() > 5 { t.Errorf("queue should cap at 5, got %d", q.Len()) }
}

// ── Live Notes ──

func TestHard_LiveNotes_CreateAndStop(t *testing.T) {
	dir := t.TempDir()
	vault := NewVault(dir)

	searchCalled := false
	engine := NewLiveNoteEngine(vault, func(ctx context.Context, query string) ([]SearchHit, error) {
		searchCalled = true
		return []SearchHit{{Platform: "test", Title: "Result", Snippet: "Found it"}}, nil
	})

	note, err := engine.Create("AI Trends", "artificial intelligence", 100*time.Millisecond)
	if err != nil { t.Fatal(err) }
	if !note.IsLive { t.Error("note should be live") }

	// Wait for at least one update
	time.Sleep(300 * time.Millisecond)
	if !searchCalled { t.Error("search function should have been called") }

	// Check note was updated
	updated, ok := vault.Get(note.ID)
	if !ok { t.Fatal("note should exist in vault") }
	if strings.Contains(updated.Content, "Loading...") { t.Error("note should have been updated from 'Loading...'") }

	// Stop
	engine.Stop(note.ID)
	time.Sleep(200 * time.Millisecond)

	stopped, _ := vault.Get(note.ID)
	if stopped.IsLive { t.Error("note should no longer be live after stop") }
}

// ── System Instructions ──

func TestHard_BuildInstructions_ContainsVaultInfo(t *testing.T) {
	dir := t.TempDir()
	agent := NewAgent(dir, nil)
	agent.Remember("Test Note", "Some content", nil)

	instructions := BuildInstructions(agent)
	if !strings.Contains(instructions, "persistent memory") { t.Error("should mention persistent memory") }
	if !strings.Contains(instructions, "test-note") { t.Error("should list recent notes") }
	if len(instructions) < 200 { t.Errorf("instructions too short: %d chars", len(instructions)) }
}
