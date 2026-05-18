// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package llm

import "testing"

func TestRegistry_New(t *testing.T) {
	r := NewRegistry()
	if r == nil { t.Fatal("nil registry") }
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	r.Register("test", nil)
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok { t.Error("should not find") }
}
