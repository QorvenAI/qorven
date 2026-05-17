// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package mail

import "testing"

func TestAgentMailProvider_New(t *testing.T) {
	p := NewAgentMailProvider("test-key")
	if p == nil { t.Fatal("nil") }
}

func TestAgentMailProvider_NewEmpty(t *testing.T) {
	p := NewAgentMailProvider("")
	if p == nil { t.Fatal("nil even with empty key") }
}
