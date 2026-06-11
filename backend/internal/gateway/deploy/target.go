// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package deploy

import (
	"context"
	"errors"
	"fmt"
)

// ErrUnknownTarget is returned when a deploy is requested for an unregistered target.
var ErrUnknownTarget = errors.New("unknown deploy target")

// Spec is everything a target needs to deploy a built project.
type Spec struct {
	ProjectID   string
	ProjectPath string
	Slug        string
	Framework   string
	ReleaseTag  string
	RepoOwner   string
	RepoName    string
}

// Result is the outcome of a deploy. URL is the REAL reachable URL — never a
// fabricated domain.
type Result struct {
	URL    string
	Target string
	Detail string
}

// Target deploys a project somewhere (local, hosted, cloud, export).
type Target interface {
	Deploy(ctx context.Context, s Spec) (Result, error)
}

// targetFunc adapts a plain func to the Target interface.
type targetFunc func(context.Context, Spec) (Result, error)

func (f targetFunc) Deploy(ctx context.Context, s Spec) (Result, error) { return f(ctx, s) }

// Registry maps a target name → implementation.
type Registry struct{ targets map[string]Target }

// NewRegistry returns an empty deploy-target registry.
func NewRegistry() *Registry { return &Registry{targets: map[string]Target{}} }

// Register adds (or replaces) a named target.
func (r *Registry) Register(name string, t Target) { r.targets[name] = t }

// Deploy dispatches to the named target; the result's Target is auto-filled with
// the dispatched name when the implementation leaves it blank.
func (r *Registry) Deploy(ctx context.Context, name string, s Spec) (Result, error) {
	t, ok := r.targets[name]
	if !ok {
		return Result{}, fmt.Errorf("%w: %q", ErrUnknownTarget, name)
	}
	res, err := t.Deploy(ctx, s)
	if err == nil && res.Target == "" {
		res.Target = name
	}
	return res, err
}
