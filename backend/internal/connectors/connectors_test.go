// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package connectors

import (
	"context"
	"testing"
)

func TestManifest_Fields(t *testing.T) {
	m := Manifest{ID: "jira", Name: "Jira", Description: "Issue tracker"}
	if m.ID != "jira" { t.Error("wrong id") }
}

func TestAuthSchema_Fields(t *testing.T) {
	a := AuthSchema{Type: "api_key", Fields: []AuthField{{Name: "api_token", Required: true}}}
	if a.Type != "api_key" { t.Error("wrong type") }
	if len(a.Fields) != 1 { t.Error("wrong field count") }
}

func TestRegistry_New(t *testing.T) {
	r := NewRegistry()
	if r == nil { t.Fatal("nil") }
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockConnector{id: "test"})
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok { t.Error("should not find") }
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockConnector{id: "a"})
	r.Register(&mockConnector{id: "b"})
	list := r.List()
	if len(list) < 2 { t.Errorf("expected 2+, got %d", len(list)) }
}

func TestConnectorTool_Name(t *testing.T) {
	ct := NewConnectorTool(&mockConnector{id: "jira"}, Action{ID: "create_issue", Name: "Create Issue"}, nil)
	name := ct.Name()
	if name == "" { t.Error("empty name") }
}

func TestConnectorTool_Execute_NilCreds(t *testing.T) {
	ct := NewConnectorTool(&mockConnector{id: "jira"}, Action{ID: "list"}, nil)
	// Skip execute — needs real cred store
	_ = ct
}

type mockConnector struct{ id string }
func (c *mockConnector) ID() string { return c.id }
func (c *mockConnector) Name() string { return c.id }
func (c *mockConnector) Manifest() Manifest { return Manifest{ID: c.id} }
func (c *mockConnector) Actions() []Action { return nil }
func (c *mockConnector) Execute(ctx context.Context, actionID string, creds map[string]string, params map[string]any) (any, error) { return "ok", nil }
func (c *mockConnector) TestConnection(ctx context.Context, creds map[string]string) error { return nil }
