// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package coworker

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// agent.go — Core coworker agent with memory, meeting prep, and email drafting.

type Agent struct {
	vault       *Vault
	liveNotes   *LiveNoteEngine
	integrations map[string]*Integration
}

func NewAgent(vaultPath string, searchFn func(ctx context.Context, query string) ([]SearchHit, error)) *Agent {
	vault := NewVault(vaultPath)
	return &Agent{
		vault:        vault,
		liveNotes:    NewLiveNoteEngine(vault, searchFn),
		integrations: make(map[string]*Integration),
	}
}

func (a *Agent) Vault() *Vault              { return a.vault }
func (a *Agent) LiveNotes() *LiveNoteEngine  { return a.liveNotes }

// Remember saves a piece of information as a note.
func (a *Agent) Remember(title, content string, tags []string) (*Note, error) {
	note := &Note{
		ID: slugify(title), Title: title, Content: content,
		Tags: tags, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	note.Backlinks = extractBacklinks(content)
	return note, a.vault.Save(note)
}

// Recall searches memory for relevant notes.
func (a *Agent) Recall(query string, limit int) []*Note {
	results := a.vault.Search(query)
	if limit > 0 && len(results) > limit { results = results[:limit] }
	return results
}

// PrepareMeeting gathers context for an upcoming meeting.
func (a *Agent) PrepareMeeting(title string, attendees []string) *MeetingContext {
	mc := &MeetingContext{Title: title, Attendees: attendees, Time: time.Now()}

	// Search vault for notes about each attendee
	for _, person := range attendees {
		notes := a.vault.Search(person)
		for _, n := range notes {
			mc.PriorNotes = append(mc.PriorNotes, n.ID)
			// Extract action items from attendee notes too
			extractOpenItems(n, mc)
		}
	}

	// Search for the meeting topic
	topicNotes := a.vault.Search(title)
	for _, n := range topicNotes {
		mc.PriorNotes = append(mc.PriorNotes, n.ID)
		extractOpenItems(n, mc)
	}

	// Deduplicate
	mc.PriorNotes = dedupStrings(mc.PriorNotes)
	mc.OpenItems = dedupStrings(mc.OpenItems)
	return mc
}

func extractOpenItems(n *Note, mc *MeetingContext) {
	for _, line := range strings.Split(n.Content, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "- [ ]") || strings.Contains(strings.ToLower(line), "todo") {
			mc.OpenItems = append(mc.OpenItems, line)
		}
		if strings.Contains(strings.ToLower(line), "decided") || strings.Contains(strings.ToLower(line), "decision") {
			mc.Decisions = append(mc.Decisions, line)
		}
	}
}

// DraftEmail creates an email draft grounded in vault context.
func (a *Agent) DraftEmail(to, subject string) *EmailDraft {
	draft := &EmailDraft{To: to, Subject: subject}

	// Search for context about the recipient and subject
	recipientNotes := a.vault.Search(to)
	subjectNotes := a.vault.Search(subject)

	var contextIDs []string
	for _, n := range recipientNotes { contextIDs = append(contextIDs, n.ID) }
	for _, n := range subjectNotes { contextIDs = append(contextIDs, n.ID) }
	draft.Context = dedupStrings(contextIDs)

	// Build context summary for LLM
	var context strings.Builder
	context.WriteString(fmt.Sprintf("Drafting email to: %s\nSubject: %s\n\nRelevant context:\n", to, subject))
	allNotes := append(recipientNotes, subjectNotes...)
	seen := map[string]bool{}
	for _, n := range allNotes {
		if seen[n.ID] { continue }
		seen[n.ID] = true
		context.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", n.Title, n.Content[:min2(len(n.Content), 500)]))
	}
	draft.Body = context.String()
	return draft
}

// AddIntegration registers an external integration.
func (a *Agent) AddIntegration(name string, config map[string]string) {
	a.integrations[name] = &Integration{Type: name, Config: config, Active: true}
}

// Shutdown stops all live notes and cleans up.
func (a *Agent) Shutdown() { a.liveNotes.StopAll() }

func dedupStrings(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss { if !seen[s] { seen[s] = true; out = append(out, s) } }
	return out
}

func min2(a, b int) int { if a < b { return a }; return b }
