// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

// Stage order for an Org-mode project. Pure; no DB/HTTP.
var stageOrder = []string{"intake", "clarify", "prd", "spec", "design", "resource_plan", "approved", "building"}

// artifactTypes are the gated documents (including CFO-owned resource_plan), in pipeline order.
var artifactTypes = []string{"prd", "spec", "design", "resource_plan"}

// cascadeArtifacts are the user-approved documents that cascade-reopen on request-changes.
// resource_plan is owned by 8B (CFO) and is not cascaded here.
var cascadeArtifacts = []string{"prd", "spec", "design"}

func stageIndex(s string) int {
	for i, v := range stageOrder {
		if v == s {
			return i
		}
	}
	return -1
}

// NextStage returns the stage after cur, or cur if terminal/unknown.
func NextStage(cur string) string {
	if cur == "approved" {
		return "approved"
	}
	i := stageIndex(cur)
	if i < 0 || i >= len(stageOrder)-1 {
		return cur
	}
	return stageOrder[i+1]
}

// ArtifactStage maps an artifact type to the stage it gates (1:1 here).
func ArtifactStage(t string) string { return t }

// DownstreamArtifacts returns artifact types that come AFTER t in the cascade
// (used to cascade-reopen documents when changes are requested). Only covers
// user-approved doc artifacts [prd, spec, design]; resource_plan is excluded.
func DownstreamArtifacts(t string) []string {
	out := []string{}
	seen := false
	for _, a := range cascadeArtifacts {
		if seen {
			out = append(out, a)
		}
		if a == t {
			seen = true
		}
	}
	return out
}

// CanAdvanceTo reports whether a project may enter stage `target` given the
// current artifact statuses. Advancing to artifact stage X requires the
// previous artifact in the pipeline to be approved.
func CanAdvanceTo(target string, artifactStatus map[string]string) bool {
	idx := -1
	for i, a := range artifactTypes {
		if a == target {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return true
	}
	prev := artifactTypes[idx-1]
	return artifactStatus[prev] == "approved"
}
