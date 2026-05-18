// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package connectors

import (
	"testing"
)

func TestHard_Registry_CRUD(t *testing.T) {
	r := NewRegistry()
	c1 := &mockConnector{id: "jira"}
	c2 := &mockConnector{id: "github"}
	c3 := &mockConnector{id: "slack"}

	r.Register(c1)
	r.Register(c2)
	r.Register(c3)

	if _, ok := r.Get("jira"); !ok { t.Error("jira not found") }
	if _, ok := r.Get("github"); !ok { t.Error("github not found") }
	if _, ok := r.Get("nonexistent"); ok { t.Error("should not find") }

	list := r.List()
	if len(list) < 3 { t.Errorf("list=%d", len(list)) }
}

func TestHard_ConnectorTool_AllActions(t *testing.T) {
	c := &mockConnector{id: "test"}
	actions := []Action{
		{ID: "list", Name: "List Items"},
		{ID: "create", Name: "Create Item"},
		{ID: "delete", Name: "Delete Item"},
	}
	for _, action := range actions {
		tool := NewConnectorTool(c, action, nil)
		if tool.Name() == "" { t.Error("empty name") }
		if tool.Description() == "" { t.Error("empty desc") }
		if tool.Parameters() == nil { t.Error("nil params") }
	}
}

func TestHard_Manifest_Fields(t *testing.T) {
	m := Manifest{ID: "jira", Name: "Jira", Description: "Issue tracker"}
	if m.ID != "jira" { t.Error("id") }
	if m.Name != "Jira" { t.Error("name") }
	auth := AuthSchema{Type: "api_key", Fields: []AuthField{
		{Name: "token", Label: "Token", Type: "password"},
	}}
	if auth.Type != "api_key" { t.Error("auth type") }
	if len(auth.Fields) != 1 { t.Error("fields") }
}
