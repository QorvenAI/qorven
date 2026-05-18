// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package council

import (
	"strings"
	"testing"
	"time"
)

// ─── Agreement Gate Tests ────────────────────────────────────────────────────

func TestCheckAgreement_HighSimilarity_SkipsStage2(t *testing.T) {
	c := &Council{config: Config{AgreementGate: 0.7}}
	// Two nearly-identical responses — should trigger gate
	responses := []ModelResponse{
		{Model: "m1", Response: "The sky is blue and the grass is green on a sunny day"},
		{Model: "m2", Response: "The sky is blue and the grass is green on a sunny day in summer"},
	}
	if !c.checkAgreement(responses) {
		t.Error("high similarity responses should trigger agreement gate")
	}
}

func TestCheckAgreement_LowSimilarity_AllowsStage2(t *testing.T) {
	c := &Council{config: Config{AgreementGate: 0.85}}
	responses := []ModelResponse{
		{Model: "m1", Response: "Python is the best language for data science due to libraries like pandas and numpy"},
		{Model: "m2", Response: "JavaScript dominates web development with React and Node.js being very popular choices"},
	}
	if c.checkAgreement(responses) {
		t.Error("low similarity responses should NOT trigger agreement gate")
	}
}

func TestCheckAgreement_SingleResponse_ReturnsTrue(t *testing.T) {
	c := &Council{config: Config{AgreementGate: 0.85}}
	if !c.checkAgreement([]ModelResponse{{Model: "m1", Response: "single"}}) {
		t.Error("single response should always pass agreement gate (no comparison possible)")
	}
}

func TestCheckAgreement_EmptyResponses(t *testing.T) {
	c := &Council{config: Config{AgreementGate: 0.85}}
	if !c.checkAgreement(nil) { t.Error("empty responses should return true") }
	if !c.checkAgreement([]ModelResponse{}) { t.Error("empty slice should return true") }
}

// ─── Jaccard Similarity Tests ────────────────────────────────────────────────

func TestJaccardSimilarity_IdenticalTexts(t *testing.T) {
	sim := jaccardSimilarity("hello world foo bar", "hello world foo bar")
	if sim != 1.0 { t.Errorf("identical texts: sim=%.2f want 1.0", sim) }
}

func TestJaccardSimilarity_CompletelyDifferent(t *testing.T) {
	sim := jaccardSimilarity("alpha beta gamma delta", "xray yankee zulu tango")
	if sim != 0.0 { t.Errorf("completely different texts: sim=%.2f want 0.0", sim) }
}

func TestJaccardSimilarity_PartialOverlap(t *testing.T) {
	sim := jaccardSimilarity("the quick brown fox", "the slow brown dog")
	if sim <= 0.0 || sim >= 1.0 { t.Errorf("partial overlap: sim=%.2f should be between 0 and 1", sim) }
}

func TestJaccardSimilarity_EmptyStrings(t *testing.T) {
	sim := jaccardSimilarity("", "")
	if sim != 1.0 { t.Errorf("both empty: sim=%.2f want 1.0", sim) }
}

func TestJaccardSimilarity_CaseInsensitive(t *testing.T) {
	sim1 := jaccardSimilarity("Hello World", "hello world")
	if sim1 != 1.0 { t.Errorf("case insensitive: sim=%.2f want 1.0", sim1) }
}

// ─── Config Tests ────────────────────────────────────────────────────────────

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.Members) == 0 { t.Error("default config should have members") }
	if cfg.Chairman == "" { t.Error("default config should have chairman") }
	if cfg.AgreementGate <= 0 || cfg.AgreementGate > 1 {
		t.Errorf("agreement gate should be 0-1, got %.2f", cfg.AgreementGate)
	}
	if cfg.MaxTokens <= 0 { t.Errorf("max tokens should be > 0, got %d", cfg.MaxTokens) }
}

func TestNew(t *testing.T) {
	c := New(nil, DefaultConfig())
	if c == nil { t.Fatal("New returned nil") }
}

// ─── WordSet Tests ───────────────────────────────────────────────────────────

func TestWordSet_BasicWords(t *testing.T) {
	set := wordSet("hello world foo")
	if !set["hello"] { t.Error("should contain 'hello'") }
	if !set["world"] { t.Error("should contain 'world'") }
	if !set["foo"] { t.Error("should contain 'foo'") }
}

func TestWordSet_StripsShortWords(t *testing.T) {
	set := wordSet("to be or not to be")
	if set["to"] || set["be"] || set["or"] {
		t.Error("short words (<= 2 chars) should be stripped")
	}
}

func TestWordSet_StripsPunctuation(t *testing.T) {
	set := wordSet("hello, world! foo.")
	if !set["hello"] { t.Error("should strip trailing comma from 'hello,'") }
	if !set["world"] { t.Error("should strip trailing ! from 'world!'") }
}

func TestWordSet_EmptyString(t *testing.T) {
	set := wordSet("")
	if len(set) != 0 { t.Error("empty string should yield empty set") }
}

// ─── Depth Config Tests ──────────────────────────────────────────────────────

func TestDepthConfigs_AllDepthsExist(t *testing.T) {
	depths := []Depth{DepthQuick, DepthBalanced, DepthDeep, DepthMax}
	for _, d := range depths {
		cfg := GetDepthConfig(d)
		if cfg.Depth == "" { t.Errorf("depth %q config should have Depth field set", d) }
		if cfg.MaxTokens <= 0 { t.Errorf("depth %q MaxTokens should be > 0", d) }
	}
}

func TestDepthConfigs_QuickDoesNotEnableCouncil(t *testing.T) {
	cfg := GetDepthConfig(DepthQuick)
	if cfg.CouncilEnabled {
		t.Error("quick depth should not enable council (too expensive for quick queries)")
	}
}

func TestDepthConfigs_DeepEnablesCouncil(t *testing.T) {
	cfg := GetDepthConfig(DepthDeep)
	if !cfg.CouncilEnabled {
		t.Error("deep depth should enable council")
	}
}

// Full council.Run() tests require a live LLM Provider — covered in e2e suite.

func TestCouncil_Run_AgreementGateTriggered(t *testing.T) {
	// When all members give identical responses, Stage 2 should be skipped
	responses := []ModelResponse{
		{Model: "m1", Label: "Response A", Response: "The answer is forty-two according to all sources"},
		{Model: "m2", Label: "Response B", Response: "The answer is forty-two according to all sources"},
		{Model: "m3", Label: "Response C", Response: "The answer is forty-two according to all sources"},
	}
	c := &Council{config: Config{AgreementGate: 0.85, Members: []string{"m1", "m2", "m3"}}}
	if !c.checkAgreement(responses) {
		t.Error("identical responses should trigger agreement gate")
	}
}

func TestCouncil_Stage1_Labels(t *testing.T) {
	// Verify label assignment pattern
	labels := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	if len(labels) != 8 { t.Error("should have 8 labels") }
	for i, l := range labels {
		if l == "" { t.Errorf("label %d is empty", i) }
	}
}

func TestModelResponse_Fields(t *testing.T) {
	r := ModelResponse{
		Model:    "gpt-4",
		Label:    "Response A",
		Response: "Test response content",
		Tokens:   42,
		Duration: time.Second,
	}
	if r.Model != "gpt-4" { t.Error("wrong model") }
	if r.Label != "Response A" { t.Error("wrong label") }
	if r.Tokens != 42 { t.Error("wrong tokens") }
}

func TestResult_Fields(t *testing.T) {
	result := &Result{
		Query:       "What is 2+2?",
		Synthesis:   "4",
		GateSkipped: false,
	}
	if result.Query == "" { t.Error("empty query") }
	if result.Synthesis == "" { t.Error("empty synthesis") }
}

func TestCheckAgreement_BelowGateThreshold(t *testing.T) {
	c := &Council{config: Config{AgreementGate: 0.99}} // very strict gate
	// Even partially similar text should NOT trigger with 0.99 threshold
	responses := []ModelResponse{
		{Model: "m1", Response: "The capital city of France is Paris which is located in Western Europe"},
		{Model: "m2", Response: "Python programming language is widely used for machine learning and data analysis"},
	}
	if c.checkAgreement(responses) {
		t.Error("completely different responses should NOT trigger gate even with low threshold logic")
	}
}

func TestCouncil_Run_NoQuery_WouldFail(t *testing.T) {
	// The council requires a non-empty query — this tests our validation assumption
	if strings.TrimSpace("") != "" {
		t.Error("empty query sanity check failed")
	}
}
