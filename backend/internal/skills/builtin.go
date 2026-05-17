// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// BuiltInSkill is a ship-with-the-binary prompt template. Users get
// these pre-installed on fresh setup so they're not staring at an
// empty skill library. Categories chosen for breadth, not depth —
// the goal is "every agent has at least one skill that matches the
// user's first few requests".
//
// Each builtin materializes as a SKILL.md file on disk at install
// time; the normal ParseSkillFile path then picks it up. Keeping them
// on disk (rather than in-memory) means users can edit them without
// rebuilding the binary.
type BuiltInSkill struct {
	Slug        string
	Name        string
	Description string
	WhenToUse   string
	// Category is metadata only — used by Settings UI to group the
	// installer toggles. Not part of SKILL.md.
	Category string
	// Body is the markdown content that drives the LLM. The first
	// line of Body is the "you are X" instruction; everything after
	// is either template variables ({{input}}) or step-by-step rules.
	Body string
}

// BuiltInCatalog is the flat list. Keep entries short, imperative,
// and self-contained — every skill should be useful on first run
// without the user editing anything.
var BuiltInCatalog = []BuiltInSkill{
	// ── Writing & Communication ──────────────────────────────────
	{
		Slug:        "summarize",
		Name:        "Summarize text",
		Description: "Produce a tight, accurate summary of any text.",
		WhenToUse:   "User pastes a long article, document, transcript, or thread and asks for a summary.",
		Category:    "writing",
		Body: `You are a careful summariser. Given the user's text, produce:

1. **One-sentence summary** — the single most important takeaway.
2. **Three bullets** — the key points, ordered by importance.
3. **Open questions** — anything the text raises but doesn't answer.

Do not add information that isn't in the source. If the text is already short, say so and skip the bullets.

Input to summarize:
{{input}}`,
	},
	{
		Slug:        "clarify-request",
		Name:        "Clarify a vague request",
		Description: "Turn a vague user message into a specific, actionable brief.",
		WhenToUse:   "User says something like \"help me with X\" without detail, OR when the request has multiple plausible interpretations.",
		Category:    "writing",
		Body: `The user has given you an underspecified request. Your job is NOT to attempt the task yet — it's to produce a short, numbered list of clarifying questions that will let you do a great job.

Rules:
- At most 5 questions.
- Each question should be answerable in one sentence.
- Group related sub-questions with letters (1a, 1b) instead of stretching to 10 top-level items.
- If the request is specific enough, say so and proceed directly; don't ask filler questions.

Request:
{{input}}`,
	},
	{
		Slug:        "reply-to-email",
		Name:        "Draft email reply",
		Description: "Draft a professional reply to an email thread.",
		WhenToUse:   "User wants to reply to an email they pasted or forwarded.",
		Category:    "writing",
		Body: `Draft a reply to the email below. Match the tone of the sender (formal / casual). Respond to every explicit ask, acknowledge any that need time ("I'll check and get back to you by Friday"), and close with a clear next step.

Structure:
- Opening line (personal if the sender was personal).
- Body — one short paragraph per point.
- Close with the next step and sign-off.

Do NOT invent facts. If the email asks for data you don't have, say so in the draft.

Email thread:
{{input}}`,
	},
	{
		Slug:        "rewrite-clearer",
		Name:        "Rewrite clearer",
		Description: "Rewrite text to be more direct and less jargon-heavy.",
		WhenToUse:   "User has draft text and wants it tightened.",
		Category:    "writing",
		Body: `Rewrite the user's text to be:

- Shorter — cut filler like "it should be noted that" and "in order to".
- More direct — prefer active voice and concrete nouns.
- Jargon-free — unless the audience explicitly needs the jargon.

Keep the tone appropriate for the audience. Never remove facts or caveats. If the text is already tight, say so and offer minor polish only.

Text:
{{input}}`,
	},

	// ── Analysis & Reasoning ──────────────────────────────────────
	{
		Slug:        "pros-cons",
		Name:        "Pros vs cons analysis",
		Description: "Structured pros/cons breakdown for a decision.",
		WhenToUse:   "User is weighing a choice and asks \"should I X\", \"is X a good idea\", or \"compare A vs B\".",
		Category:    "analysis",
		Body: `Analyse the user's decision. Output:

1. **The decision (one sentence).**
2. **Pros** — 3-5 bullets, strongest first.
3. **Cons** — 3-5 bullets, strongest first.
4. **What would change my mind** — one sentence per side.
5. **Recommendation** — pick one option, explain the single biggest reason.

Don't hedge. "It depends" is only acceptable if you then explain what it depends on.

Decision:
{{input}}`,
	},
	{
		Slug:        "find-the-lie",
		Name:        "Find the weakness",
		Description: "Find the strongest counter-argument against a claim.",
		WhenToUse:   "User proposes a plan, argument, or claim and wants it stress-tested.",
		Category:    "analysis",
		Body: `Your job is to disagree — in good faith, with specifics.

Given the user's claim/plan, produce:
1. **The single biggest weakness.**
2. **Three scenarios where this fails** — concrete, not abstract.
3. **The strongest defense** — how would a proponent respond to (2)?

Be rigorous, not performatively contrarian. If the claim is genuinely solid, say so — and explain what would be required to break it.

Claim/plan:
{{input}}`,
	},
	{
		Slug:        "decision-journal",
		Name:        "Decision journal entry",
		Description: "Structured decision record for retrospectives.",
		WhenToUse:   "User has just made or is about to make an important decision.",
		Category:    "analysis",
		Body: `Produce a decision journal entry with these fields:

- **Date**: today's date.
- **Decision**: one sentence, active voice.
- **Context**: 2-3 bullets on the situation.
- **Options considered**: list with one-line reasoning for each rejection.
- **Expected outcome**: what success looks like, by when.
- **Reversibility**: one-way door / two-way door?
- **Confidence**: low / medium / high — and what would shift it.
- **Review date**: when to look back at this entry.

Decision to journal:
{{input}}`,
	},

	// ── Code Review & Engineering ─────────────────────────────────
	{
		Slug:        "code-review",
		Name:        "Code review",
		Description: "Focused review: correctness, readability, maintainability.",
		WhenToUse:   "User pastes code and asks for review, feedback, or improvements.",
		Category:    "code",
		Body: `Review the code with three lenses, in order:

1. **Correctness** — bugs, races, off-by-ones, null-deref risks, logic errors. Cite line numbers.
2. **Readability** — naming, function size, comment quality. Cite specific changes.
3. **Maintainability** — test coverage, coupling, hidden global state.

Rules:
- One concrete suggestion per issue. Don't list "improve naming" without naming WHICH name.
- Skip style nits that a linter would catch — focus on things linters miss.
- If the code is already good, say so. Don't invent problems.
- Finish with a one-line overall verdict: ship / revise / rethink.

Code:
{{input}}`,
	},
	{
		Slug:        "explain-code",
		Name:        "Explain code",
		Description: "Walk through code line-by-line at the caller's level.",
		WhenToUse:   "User pastes code and asks \"what does this do\" or \"explain this\".",
		Category:    "code",
		Body: `Explain the code so that someone who didn't write it can maintain it.

Structure:
1. **One-sentence summary** — what the code does.
2. **Data flow** — what comes in, what goes out, what mutates.
3. **Walkthrough** — block by block. Name functions, cite branching.
4. **Edge cases / gotchas** — anything non-obvious, e.g. off-by-ones, silent error swallows, implicit assumptions.

Match the explanation depth to the code's apparent audience: simpler code gets less line-by-line, tricky code gets more.

Code:
{{input}}`,
	},
	{
		Slug:        "debug-help",
		Name:        "Debug an error",
		Description: "Systematic debug walkthrough from symptom to likely cause.",
		WhenToUse:   "User pastes an error message, stack trace, or broken output and asks for help.",
		Category:    "code",
		Body: `Debug the problem the user described. Structure:

1. **Restate the symptom** in one sentence. If the symptom is ambiguous, list the possibilities.
2. **Most likely cause** and why (cite lines or error details).
3. **Second most likely cause** — don't stop at one; real bugs often have multiple candidates.
4. **Diagnostic steps** — ordered, cheapest-first, that will distinguish the causes.
5. **Fix** for each candidate, once we know which.

Never guess silently. Say "I need to see X to be sure".

Problem:
{{input}}`,
	},
	{
		Slug:        "write-test",
		Name:        "Write test cases",
		Description: "Produce test cases for a function or behaviour.",
		WhenToUse:   "User wants unit/integration test scaffolding for something they've described or pasted.",
		Category:    "code",
		Body: `Write tests that actually exercise the code. For each test:

- **Name** — describes the scenario, not the function.
- **Setup** — what's needed.
- **Action** — the call.
- **Assertion** — what must be true.

Cover: happy path, boundary values (empty, zero, max), error paths, concurrency if relevant. Skip trivial tests that only re-assert the language spec.

Target:
{{input}}`,
	},
	{
		Slug:        "commit-message",
		Name:        "Write commit message",
		Description: "Turn a diff into a clear commit message.",
		WhenToUse:   "User pastes a diff / describes a change and wants a commit message.",
		Category:    "code",
		Body: `Write a commit message in this exact shape:

` + "```" + `
<type>(<scope>): <subject line in imperative mood, ≤ 72 chars>

<body — optional, but required when WHY isn't obvious from diff>
<body — wrap at 80 chars>
` + "```" + `

Types: feat / fix / refactor / docs / test / chore / perf / ci / build.

Rules:
- Subject: what changed, imperative ("add", not "added").
- Body: explain the WHY, not the WHAT — the diff already shows the what.
- No ticket numbers unless the user provided one.

Change:
{{input}}`,
	},

	// ── Data & Research ───────────────────────────────────────────
	{
		Slug:        "extract-structured",
		Name:        "Extract structured data",
		Description: "Pull a typed JSON structure out of free text.",
		WhenToUse:   "User has messy text (receipt, email, log, chat) and wants specific fields extracted.",
		Category:    "data",
		Body: `Extract the fields the user specified into clean JSON. Rules:

- Output ONLY the JSON object — no preamble, no code fences, no trailing prose.
- Unknown fields → null. Never invent.
- Preserve the original formatting for dates, currencies, IDs unless the user asks otherwise.
- If the user didn't name the fields, ask (don't guess a schema).

Input:
{{input}}`,
	},
	{
		Slug:        "research-topic",
		Name:        "Research a topic",
		Description: "Decompose a research question and run sub-queries.",
		WhenToUse:   "User asks \"research X\", \"what's the state of Y\", or wants a briefing on a topic they don't know well.",
		Category:    "data",
		Body: `Research the topic. Approach:

1. Decompose the question into 3-5 sub-queries.
2. Use the research tool (or web_search + web_fetch) to answer each.
3. Synthesise into a briefing with inline citations [1], [2] etc.

Output:
- **TL;DR** (2-3 sentences).
- **Key findings** — bulleted, each with citation.
- **Open questions** — what's still unclear and why.
- **Sources** — numbered list of URLs.

Do not speculate beyond what sources support. If findings conflict, say so.

Topic:
{{input}}`,
	},

	// ── DevOps / Ops ──────────────────────────────────────────────
	{
		Slug:        "sre-post-mortem",
		Name:        "Write a post-mortem",
		Description: "Blameless post-mortem template from incident notes.",
		WhenToUse:   "User describes an outage or incident and wants a structured write-up.",
		Category:    "devops",
		Body: `Write a blameless post-mortem using these sections:

- **Summary** — what happened in one paragraph.
- **Impact** — who was affected, how, for how long.
- **Timeline** — UTC timestamps, one event per line.
- **Root cause** — the thing that actually broke, not just the user-visible symptom.
- **Contributing factors** — things that made it worse or slowed detection.
- **What went well** — tools, people, processes that worked.
- **Action items** — SMART; assign to a team, not a person, unless named.

Blameless: no "Alice should have known better". Focus on systems.

Incident:
{{input}}`,
	},
	{
		Slug:        "deploy-checklist",
		Name:        "Deploy checklist",
		Description: "Generate a deploy checklist for a change.",
		WhenToUse:   "User is about to deploy something and wants a pre-flight checklist.",
		Category:    "devops",
		Body: `Produce a deploy checklist tuned to the change described. Cover:

1. **Pre-flight** — env vars, migrations, feature flags, dependencies.
2. **Smoke tests** — specific endpoints / flows to exercise post-deploy.
3. **Rollback plan** — single command or procedure.
4. **Observability** — dashboards / alerts to watch for N minutes after.

Be specific. "Check dashboards" is not enough; name the panels.

Change:
{{input}}`,
	},

	// ── Meta / Self ───────────────────────────────────────────────
	{
		Slug:        "plan-task",
		Name:        "Plan before acting",
		Description: "Produce a step-by-step plan for a multi-step task.",
		WhenToUse:   "Task has 3+ moving parts, or when ambiguity means the wrong first step is expensive to undo.",
		Category:    "meta",
		Body: `Plan the task before you start. Output:

- **Goal** — one sentence, user-observable.
- **Steps** — numbered, smallest-first-deliverable ordering.
- **Risks** — what could go wrong, and the checkpoint you'll use to detect it.
- **Stop-ask points** — places the plan should pause for user input.

Don't start the work. Hand the plan back for approval first.

Task:
{{input}}`,
	},
}

// InstallBuiltIns writes every BuiltInCatalog entry to disk as a
// SKILL.md in builtinRoot/<slug>/SKILL.md and registers it in the
// skill store. Idempotent: re-installing updates content without
// duplicating the row.
//
// Errors are returned but installation continues — a single corrupt
// skill shouldn't block the other 15.
func (s *Store) InstallBuiltIns(ctx context.Context, tenantID, builtinRoot string) (installed int, skipped int, errs []error) {
	if err := os.MkdirAll(builtinRoot, 0o755); err != nil {
		return 0, 0, []error{fmt.Errorf("mkdir %s: %w", builtinRoot, err)}
	}
	for _, b := range BuiltInCatalog {
		dir := filepath.Join(builtinRoot, b.Slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			errs = append(errs, fmt.Errorf("mkdir %s: %w", dir, err))
			continue
		}
		path := filepath.Join(dir, "SKILL.md")
		content := renderBuiltInMD(b)
		// Skip write if the file exists AND its content hash matches —
		// users are allowed to edit their installed skills, so don't
		// silently overwrite their changes on every gateway boot.
		sum := sha256.Sum256([]byte(content))
		hash := hex.EncodeToString(sum[:])
		if existing, err := os.ReadFile(path); err == nil {
			existingSum := sha256.Sum256(existing)
			if hex.EncodeToString(existingSum[:]) == hash {
				skipped++
				continue
			}
			// Content hash differs from our canonical form. Leave the
			// user's edits alone; they wanted them. Just make sure
			// the skill is registered in the DB.
		} else {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				errs = append(errs, fmt.Errorf("write %s: %w", path, err))
				continue
			}
		}
		_, err := s.Create(ctx, tenantID, b.Name, b.Slug, b.Description,
			path, hash, []string{b.Category})
		if err != nil {
			// ON CONFLICT unique violation is expected on reinstall.
			// Only flag the ones that aren't "already exists".
			if !isUniqueViolation(err) {
				errs = append(errs, fmt.Errorf("register %s: %w", b.Slug, err))
				continue
			}
		}
		installed++
	}
	return installed, skipped, errs
}

// renderBuiltInMD turns a BuiltInSkill into the SKILL.md format that
// ParseSkillFile expects. The parser only strips leading/trailing
// quotes — it doesn't unescape interior `\"` — so we must pick a
// quote character that doesn't appear inside our values.
//
// Strategy: pick single-vs-double quote per field based on what the
// value contains. If it has a double quote, wrap in single quotes and
// vice versa. If it has both (very rare in our curated catalog), fall
// back to no quotes and let the parser accept it as a bare scalar.
func renderBuiltInMD(b BuiltInSkill) string {
	return fmt.Sprintf(`---
name: %s
description: %s
when_to_use: %s
context: "inline"
---

%s
`, yamlQuote(b.Name), yamlQuote(b.Description), yamlQuote(b.WhenToUse), b.Body)
}

// yamlQuote picks a safe wrapping for a one-line scalar value so that
// the parser (which only strips outer quotes) round-trips correctly.
func yamlQuote(s string) string {
	hasDouble := false
	hasSingle := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			hasDouble = true
		case '\'':
			hasSingle = true
		}
	}
	switch {
	case !hasDouble:
		return `"` + s + `"`
	case !hasSingle:
		return `'` + s + `'`
	default:
		// Both quote types present — very rare. Fall back to bare
		// scalar; ParseSkillFile's Trim is a no-op on bare values.
		return s
	}
}

// escapeYAML is retained as a no-op wrapper for callers that expect
// a function with this name. Deprecated; prefer yamlQuote.
func escapeYAML(s string) string { return s }

// isUniqueViolation reports whether err is a Postgres unique-key
// conflict (SQLSTATE 23505). Used during reinstall to avoid spamming
// the error log with "already exists" noise.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return (len(s) >= 5 && s[:5] == "23505") ||
		(len(s) >= 32 && s[:6] == "SQLSTA") || // pgx wraps "SQLSTATE 23505" sometimes
		contains(s, "duplicate key") || contains(s, "unique constraint")
}

// tiny in-package substring check so we don't pull in strings in a
// hot-ish loop on every reinstall.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
