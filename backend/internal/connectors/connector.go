// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package connectors

import "context"

// Connector is the interface every integration must implement.
type Connector interface {
	Manifest() Manifest
	TestConnection(ctx context.Context, creds map[string]string) error
	Execute(ctx context.Context, action string, creds map[string]string, params map[string]any) (any, error)
}

// Manifest describes a connector's capabilities.
type Manifest struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Icon        string       `json:"icon"`
	Category    string       `json:"category"` // developer, workplace, data, commerce, infra, productivity
	Status      string       `json:"status"`      // active | coming_soon | beta
	AuthSchema  AuthSchema   `json:"auth_schema"`
	Actions     []Action     `json:"actions"`
	Triggers    []Trigger    `json:"triggers,omitempty"`
}

// AuthSchema defines what credentials a connector needs.
type AuthSchema struct {
	Type   string      `json:"type"` // api_key, oauth2, basic, none
	Fields []AuthField `json:"fields"`
}

type AuthField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"` // string, password, url
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
}

// Action is something a connector can do (e.g., "create_issue", "send_email").
type Action struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Parameters  []ActionParam    `json:"parameters"`
}

type ActionParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string, number, boolean, json
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// Trigger is an event a connector can watch for.
type Trigger struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Registry holds all registered connectors.
type Registry struct {
	connectors map[string]Connector
}

func NewRegistry() *Registry { return &Registry{connectors: make(map[string]Connector)} }

func (r *Registry) Register(c Connector) { r.connectors[c.Manifest().ID] = c }

func (r *Registry) Get(id string) (Connector, bool) {
	c, ok := r.connectors[id]
	return c, ok
}

func (r *Registry) List() []Manifest {
	manifests := make([]Manifest, 0, len(r.connectors))
	for _, c := range r.connectors {
		manifests = append(manifests, c.Manifest())
	}
	return manifests
}

func (r *Registry) Execute(ctx context.Context, connectorID, action string, creds map[string]string, params map[string]any) (any, error) {
	c, ok := r.connectors[connectorID]
	if !ok {
		return nil, ErrNotFound
	}
	return c.Execute(ctx, action, creds, params)
}

var ErrNotFound = &ConnectorError{Message: "connector not found"}

type ConnectorError struct{ Message string }

func (e *ConnectorError) Error() string { return e.Message }
