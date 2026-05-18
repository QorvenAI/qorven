// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"math"
	"strings"
	"unicode"
)

// containsWord returns true when s contains word as a whole-word match
// (not as a substring of another word). Used for light-dimension keywords
// to avoid false matches like "hi" inside "this" or "gn" inside "design".
func containsWord(s, word string) bool {
	wl := len(word)
	i := 0
	for i+wl <= len(s) {
		idx := strings.Index(s[i:], word)
		if idx < 0 {
			return false
		}
		abs := i + idx
		before := abs == 0 || !unicode.IsLetter(rune(s[abs-1])) && !unicode.IsDigit(rune(s[abs-1]))
		after := abs+wl >= len(s) || !unicode.IsLetter(rune(s[abs+wl])) && !unicode.IsDigit(rune(s[abs+wl]))
		if before && after {
			return true
		}
		i = abs + 1
	}
	return false
}

// Tier represents prompt complexity level.
type Tier int

const (
	TierLight    Tier = iota // simple greetings, definitions → cheapest model
	TierStandard             // normal questions → default model
	TierComplex              // multi-step, technical → capable model
	TierReasoning            // proofs, deep analysis → best model
)

// RoutingProfile controls which model tier→model mapping is used.
// Mirrors the ClawRouter free/eco/auto/premium profiles.
type RoutingProfile string

const (
	ProfileAuto    RoutingProfile = "auto"    // balanced cost/quality (default)
	ProfileEco     RoutingProfile = "eco"     // cheapest model per tier
	ProfilePremium RoutingProfile = "premium" // best model per tier regardless of cost
	ProfileFree    RoutingProfile = "free"    // light tier only; free-tier models
)

// ModelRouter picks the right model for each LLM call.
// 4-level system: agent default → process default → task override → complexity score.
type ModelRouter struct {
	// Defaults maps process_type → model (channel, worker, compactor, light)
	Defaults map[string]string
	// Overrides maps task_type → model (coding, summarization, etc.)
	Overrides map[string]string
	// Fallbacks maps model → ordered fallback chain on 429/5xx
	Fallbacks map[string][]string
	// Profile controls the tier→model mapping for auto-routing
	Profile RoutingProfile
}

func NewModelRouter() *ModelRouter {
	return &ModelRouter{
		Defaults:  make(map[string]string),
		Overrides: make(map[string]string),
		Fallbacks: make(map[string][]string),
		Profile:   ProfileAuto,
	}
}

// Resolve picks the best model for a given process type, task type, and message.
func (r *ModelRouter) Resolve(processType, taskType, userMessage, agentModel string) string {
	model := agentModel

	// Level 1: process-type default
	if m, ok := r.Defaults[processType]; ok && m != "" {
		model = m
	}

	// Level 2: task-type override (e.g. "coding" → coding model)
	if taskType != "" {
		if m, ok := r.Overrides[taskType]; ok && m != "" {
			model = m
		}
	}

	// Level 3: complexity scoring — downgrade simple or escalate heavy messages
	if processType == "channel" || processType == "branch" {
		tier := ScoreComplexity(userMessage)
		switch tier {
		case TierLight:
			if cheap, ok := r.Defaults["light"]; ok && cheap != "" {
				model = cheap
			}
		case TierComplex:
			if heavy, ok := r.Defaults["complex"]; ok && heavy != "" {
				model = heavy
			}
		case TierReasoning:
			if best, ok := r.Defaults["reasoning"]; ok && best != "" {
				model = best
			}
		}
	}

	return model
}

// GetFallbacks returns the fallback chain for a model.
func (r *ModelRouter) GetFallbacks(model string) []string {
	return r.Fallbacks[model]
}

// ── Complexity scorer ─────────────────────────────────────────────────────────
//
// 14 weighted dimensions, all evaluated on the user message only (excluding
// system prompts to avoid boilerplate bias — same approach as ClawRouter).
// Returns a continuous score; a sigmoid-calibrated confidence value drives
// boundary decisions so edge cases default to the safer middle tier.
//
// Hard overrides:
//   - Message token estimate >100K  → TierComplex minimum
//   - Structured output requested   → TierStandard minimum
//   - Agentic loop indicators       → TierComplex minimum

type dimension struct {
	weight   float64
	keywords []string
}

var (
	// Positive signals — each keyword match adds weight*1.0 to the score.
	// Only the first matching keyword per dimension is counted (no double-dipping).
	heavyDimensions = []dimension{
		// Reasoning / proof — highest weight; these tasks demand the best model
		{0.22, []string{"prove", "step by step", "explain why", "derive", "theorem", "proof", "infer", "deduce", "logical consequence", "formal proof"}},
		// Code presence — backticks, braces, language keywords
		{0.18, []string{"```", "{", "def ", "func ", "import ", "class ", "const ", "var ", "=>", "->", "#!/"}},
		// Multi-step sequences
		{0.14, []string{"step by step", "first,", "second,", "third,", "then ", "finally,", "step 1", "step 2", "1.", "2.", "3."}},
		// Technical domain vocabulary
		{0.12, []string{"algorithm", "complexity", "architecture", "distributed", "concurrency", "kubernetes", "microservice", "cryptography", "neural network", "transformer", "embedding", "goroutine", "multi-region", "zero downtime", "high availability", "generics", "allocations", "memory leak", "race condition", "deadlock", "type parameter"}},
		// Analytical / comparative
		{0.10, []string{"trade-off", "compare", "contrast", "versus", "pros and cons", "why does", "how does", "what is the difference", "analyze", "analyse", "evaluate"}},
		// Engineering actions — refactor, implement, debug, etc.
		{0.14, []string{"refactor", "implement", "debug", "troubleshoot", "migrate", "deploy", "optimize", "configure", "integrate", "comprehensive", "in-depth"}},
		// Agentic / workflow markers
		{0.08, []string{"retry", "loop", "iterate", "autonomous", "workflow", "pipeline", "orchestrat", "schedule", "trigger", "autonomous agent"}},
		// Constraint / optimization problems
		{0.07, []string{"minimize", "maximize", "constraint", "given that", "such that", "subject to", "optimal", "objective function"}},
		// Build / create directives
		{0.06, []string{"build", "create", "generate", "develop", "write a ", "make a ", "design"}},
		// Structured output request
		{0.05, []string{"json", "csv", "yaml", "xml", "markdown table", "structured output", "schema", "format as"}},
		// Creative tasks (slightly above trivial)
		{0.04, []string{"story", "brainstorm", "imagine", "fiction", "screenplay", "narrative", "worldbuild"}},
		// Context reference (multi-turn complexity)
		{0.03, []string{"as mentioned", "as we discussed", "from the context", "referring to", "in our previous"}},
		// Regulated domains
		{0.03, []string{"medical", "legal", "financial", "regulatory", "compliance", "clinical", "actuarial"}},
	}

	// Negative signals — each match subtracts weight (drives score toward TierLight).
	// Use whole-word matching (containsWord) to prevent "hi" matching "this", "gn" matching "design", etc.
	lightDimensions = []dimension{
		{0.20, []string{"hi", "hello", "hey", "thanks", "thank you", "okay", "ok", "sure", "cool", "nice", "great", "good", "bye", "goodbye", "sup", "yo", "lol", "haha", "thx"}},
		{0.12, []string{"what is ", "define ", "who is ", "when was ", "where is ", "spell ", "how do you say", "what does", "meaning of"}},
	}
)

// ScoreComplexity classifies a user message into Light/Standard/Complex/Reasoning.
// <1ms, no external calls. Drop-in replacement for the old 3-tier binary classifier.
func ScoreComplexity(message string) Tier {
	msg := strings.ToLower(strings.TrimSpace(message))

	// Hard override: token estimate (rough: 1 token ≈ 4 chars)
	if len(msg) > 400_000 { // ~100K tokens
		return TierComplex
	}

	// Hard override: structured output request
	for _, kw := range []string{"json", "csv", "yaml", "xml", "markdown table", "schema"} {
		if strings.Contains(msg, kw) {
			// At least standard, but continue scoring — may end up higher
			break
		}
	}

	score := 0.0
	wordCount := countWords(msg)

	// Heavy dimensions (positive contribution)
	for _, dim := range heavyDimensions {
		for _, kw := range dim.keywords {
			if strings.Contains(msg, kw) {
				score += dim.weight
				break // only count each dimension once
			}
		}
	}

	// Light dimensions (negative contribution).
	// Single-word keywords use containsWord to prevent "hi" matching "this",
	// "ok" matching "look", etc. Multi-word phrases use strings.Contains since
	// internal spaces already provide implicit word boundaries.
	for _, dim := range lightDimensions {
		for _, kw := range dim.keywords {
			var matched bool
			if strings.ContainsRune(kw, ' ') {
				matched = strings.Contains(msg, kw)
			} else {
				matched = containsWord(msg, kw)
			}
			if matched {
				score -= dim.weight
				break
			}
		}
	}

	// Token length bonus: longer messages tend to need more capability
	if wordCount > 150 {
		score += 0.10
	} else if wordCount > 50 {
		score += 0.04
	}

	// Confidence calibration via sigmoid — maps score to [0,1].
	// Low confidence near tier boundaries → default to the safer middle tier.
	conf := sigmoid(score * 4.0) // steepen the curve
	if conf < 0.35 || conf > 0.65 {
		// High confidence — map score to tier directly
		return scoreTier(score)
	}
	// Low confidence boundary — pull toward Standard
	if score < 0 {
		return TierLight // clearly simple even with low confidence
	}
	return TierStandard
}

func scoreTier(score float64) Tier {
	switch {
	case score < 0:
		return TierLight
	case score < 0.15:
		return TierStandard
	case score < 0.32:
		return TierComplex
	default:
		return TierReasoning
	}
}

func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

func countWords(s string) int {
	n := 0
	inWord := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			n++
		}
	}
	return n
}
