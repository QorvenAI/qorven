// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// LearningLoop runs after every agent task to extract learnings via a
// five-step auto-retrospective.
//
// Five steps:
//   1. Curate Memory — what's worth remembering from this conversation
//   2. Extract Skills — was a reusable procedure demonstrated
//   3. User Modeling — what preferences/patterns were revealed
//   4. Skill Improvement — did user correct something a Skill should learn
//   5. Daily Log — append to today's log for long-term context

type LearningLoop struct {
	provider     providers.Provider
	model        string
	memStore     interface{ Save(ctx context.Context, tenantID, agentID, key, value, scope string) error }
	skillCrys    interface{ MaybeExtract(ctx context.Context, agentID, userMsg, response string, toolsUsed []string) (interface{}, error) }
	pool         interface{ Exec(ctx context.Context, sql string, args ...any) (interface{}, error) }
	lastRun      map[string]time.Time   // agentID → last run time
	learnedHints map[string]string      // agentID → learned hints string
	mu           sync.Mutex
}

func NewLearningLoop(provider providers.Provider, model string) *LearningLoop {
	// Leave model as "" — the provider will use its own default.
	// Avoids hardcoding a model that may not be available for the tenant.
	return &LearningLoop{provider: provider, model: model, lastRun: make(map[string]time.Time), learnedHints: make(map[string]string)}
}

func (ll *LearningLoop) SetPool(pool interface{ Exec(ctx context.Context, sql string, args ...any) (interface{}, error) }) {
	ll.pool = pool
}

func (ll *LearningLoop) SetMemStore(ms interface{ Save(ctx context.Context, tenantID, agentID, key, value, scope string) error }) {
	ll.memStore = ms
}

func (ll *LearningLoop) SetCrystallizer(c interface{ MaybeExtract(ctx context.Context, agentID, userMsg, response string, toolsUsed []string) (interface{}, error) }) {
	ll.skillCrys = c
}

// GetLearnedHints returns accumulated learned preferences for an agent.
func (ll *LearningLoop) GetLearnedHints(agentID string) string {
	ll.mu.Lock()
	defer ll.mu.Unlock()
	return ll.learnedHints[agentID]
}

// RunAfterTask is called after every agent task completes.
// It runs in a goroutine — never blocks the response.
func (ll *LearningLoop) RunAfterTask(ctx context.Context, agentID, tenantID, userMsg, response string, toolsUsed []string) {
	if ll.provider == nil || userMsg == "" || response == "" {
		return
	}
	// Skip trivial exchanges
	if len(userMsg) < 20 && len(response) < 100 {
		return
	}
	// Rate limit: max 1 call per 5 minutes per agent
	ll.mu.Lock()
	if last, ok := ll.lastRun[agentID]; ok && time.Since(last) < 5*time.Minute {
		ll.mu.Unlock()
		return
	}
	ll.lastRun[agentID] = time.Now()
	ll.mu.Unlock()

	start := time.Now()

	// Step 1+3: Extract memories and user preferences in one LLM call
	memories, userPrefs := ll.extractInsights(ctx, agentID, userMsg, response)

	// Save memories
	if ll.memStore != nil && len(memories) > 0 {
		for _, m := range memories {
			ll.memStore.Save(ctx, tenantID, agentID, m.Key, m.Value, m.Scope)
		}
		slog.Info("learning.memories_saved", "agent", agentID, "count", len(memories))
	}

	// Save user preferences
	if ll.memStore != nil && len(userPrefs) > 0 {
		for _, p := range userPrefs {
			ll.memStore.Save(ctx, tenantID, agentID, "user_pref:"+p.Key, p.Value, "user")
		}
		slog.Info("learning.user_prefs_saved", "agent", agentID, "count", len(userPrefs))

		// Build LearnedHints string for dynamic prompt injection
		var hints []string
		for _, p := range userPrefs {
			hints = append(hints, p.Key+": "+p.Value)
		}
		ll.mu.Lock()
		ll.learnedHints[agentID] = strings.Join(hints, "\n")
		ll.mu.Unlock()

		// Also update user_profiles table so system prompt picks it up
		if ll.pool != nil {
			prefsJSON, _ := json.Marshal(userPrefs)
			ll.pool.Exec(ctx,
				`UPDATE user_profiles SET preferences = preferences || $1::jsonb, updated_at = NOW() WHERE tenant_id = $2`,
				string(prefsJSON), tenantID)
		}
	}

	// Step 2: Try to crystallize a Skill (skip if agentID is empty/invalid)
	if ll.skillCrys != nil && len(toolsUsed) > 0 && len(agentID) > 10 {
		skill, err := ll.skillCrys.MaybeExtract(ctx, agentID, userMsg, response, toolsUsed)
		if err != nil {
			slog.Debug("learning.skill_extract_failed", "agent", agentID, "error", err)
		} else if skill != nil {
			slog.Info("learning.skill_extracted", "agent", agentID)
		}
	}

	slog.Debug("learning.loop_complete", "agent", agentID, "duration_ms", time.Since(start).Milliseconds())
}

type memoryItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Scope string `json:"scope"`
}

type userPref struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (ll *LearningLoop) extractInsights(ctx context.Context, agentID, userMsg, response string) ([]memoryItem, []userPref) {
	// Truncate to save tokens
	if len(userMsg) > 500 { userMsg = userMsg[:500] }
	if len(response) > 1000 { response = response[:1000] }

	prompt := `Analyze this conversation and extract two things:

1. MEMORIES: Facts worth remembering for future conversations (max 3)
   - Project names, tech stack, deadlines, decisions made
   - NOT trivial facts like greetings

2. USER_PREFERENCES: Patterns about how this user works (max 2)
   - Communication style, coding preferences, tool preferences
   - Only if clearly demonstrated, not guessed

Return JSON only:
{"memories": [{"key": "short_label", "value": "fact to remember", "scope": "project|company|personal"}], "user_preferences": [{"key": "pref_name", "value": "description"}]}

If nothing worth extracting, return: {"memories": [], "user_preferences": []}

USER: ` + userMsg + `
ASSISTANT: ` + response

	resp, err := ll.provider.Chat(ctx, providers.ChatRequest{
		Model: ll.model,
		Messages: []providers.Message{
			{Role: "system", Content: "You extract structured insights from conversations. Return only valid JSON."},
			{Role: "user", Content: prompt},
		},
		Options: map[string]any{"temperature": 0.1, "max_tokens": 300},
	})
	if err != nil {
		return nil, nil
	}

	// Parse JSON from response
	content := strings.TrimSpace(resp.Content)
	// Strip markdown code blocks if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result struct {
		Memories   []memoryItem `json:"memories"`
		UserPrefs  []userPref   `json:"user_preferences"`
	}
	if json.Unmarshal([]byte(content), &result) != nil {
		return nil, nil
	}

	return result.Memories, result.UserPrefs
}
