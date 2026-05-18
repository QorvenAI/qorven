// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package session

import "testing"

func TestSession_Fields(t *testing.T) {
	s := Session{ID: "s1", AgentID: "a1", UserID: "u1", Channel: "telegram"}
	if s.ID != "s1" { t.Error("wrong id") }
	if s.Channel != "telegram" { t.Error("wrong channel") }
}

func TestMessage_Fields(t *testing.T) {
	m := Message{Role: "user", Content: "hello"}
	if m.Role != "user" { t.Error("wrong role") }
}

func TestMessage_Roles(t *testing.T) {
	roles := []string{"user", "assistant", "system", "tool"}
	for _, r := range roles {
		m := Message{Role: r}
		if m.Role == "" { t.Error("empty role") }
	}
}

func TestStore_New_NilPool(t *testing.T) {
	s := NewStore(nil)
	if s == nil { t.Fatal("nil store") }
}
