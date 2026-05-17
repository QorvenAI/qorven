// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"os/exec"
	"time"
)

type SelfTest struct{ root string }

func NewSelfTest(root string) *SelfTest { return &SelfTest{root: root} }
func (s *SelfTest) Name() string        { return "self_test" }
func (s *SelfTest) Description() string { return "Run build + test suite on agent's own codebase" }
func (s *SelfTest) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}}
}

func (s *SelfTest) Execute(ctx context.Context, args map[string]any) *Result {
	// Build
	buildOut, buildErr := exec.CommandContext(ctx, "go", "build", "-o", "/dev/null", ".").CombinedOutput()
	if buildErr != nil {
		return TextResult("BUILD FAILED:\n" + string(buildOut))
	}
	// Test
	ctx2, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	testOut, testErr := exec.CommandContext(ctx2, "go", "test", "-count=1", "-timeout=60s", "./...").CombinedOutput()
	result := string(testOut)
	if len(result) > 4000 { result = result[:4000] }
	if testErr != nil {
		return TextResult("BUILD OK, TESTS FAILED:\n" + result)
	}
	return TextResult("BUILD OK, ALL TESTS PASSED:\n" + result)
}
