// Copyright 2026 Qorven AI. All rights reserved.
package agent

import "testing"

func TestContextVersionMethodSet(t *testing.T) {
	_ = (*Store).ListContextFileVersions
	_ = (*Store).GetContextFileVersion
	_ = (*Store).SetAgentContextFile
}
