// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package config

import "testing"

func TestToolPolicySpec_Fields(t *testing.T) {
	p := ToolPolicySpec{AllowedTools: []string{"web_search", "exec"}, DeniedTools: []string{"delete_file"}}
	if len(p.AllowedTools) != 2 { t.Error("wrong allowed count") }
	if len(p.DeniedTools) != 1 { t.Error("wrong denied count") }
}

func TestCompactionConfig_Fields(t *testing.T) {
	c := CompactionConfig{MaxHistoryShare: 0.8, KeepLastMessages: 5}
	if c.MaxHistoryShare != 0.8 { t.Error("wrong share") }
}

func TestMemoryConfig_Fields(t *testing.T) {
	m := MemoryConfig{EmbeddingModel: "text-embedding-3-small", SharedMemory: true}
	if m.EmbeddingModel == "" { t.Error("empty model") }
}

func TestSandboxConfig_Fields(t *testing.T) {
	s := SandboxConfig{Enabled: true, ContainerImage: "ubuntu:22.04"}
	if s.ContainerImage != "ubuntu:22.04" { t.Error("wrong image") }
}

func TestModelPricing_Fields(t *testing.T) {
	p := ModelPricing{InputPer1K: 0.003, OutputPer1K: 0.015}
	if p.InputPer1K != 0.003 { t.Error("wrong price") }
}
