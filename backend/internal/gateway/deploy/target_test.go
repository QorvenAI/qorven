// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package deploy

import (
	"context"
	"errors"
	"testing"
)

func TestRegistry_DispatchAndUnknown(t *testing.T) {
	reg := NewRegistry()
	called := false
	reg.Register("local", targetFunc(func(ctx context.Context, s Spec) (Result, error) {
		called = true
		return Result{URL: "http://x", Target: "local"}, nil
	}))
	res, err := reg.Deploy(context.Background(), "local", Spec{Slug: "demo"})
	if err != nil || !called || res.URL != "http://x" {
		t.Fatalf("dispatch failed: %v %v", res, err)
	}
	if _, err := reg.Deploy(context.Background(), "nope", Spec{}); !errors.Is(err, ErrUnknownTarget) {
		t.Errorf("unknown target should error: %v", err)
	}
}

func TestRegistry_AutoFillsTargetName(t *testing.T) {
	reg := NewRegistry()
	reg.Register("hosted", targetFunc(func(ctx context.Context, s Spec) (Result, error) {
		return Result{URL: "http://y"}, nil // Target intentionally empty
	}))
	res, _ := reg.Deploy(context.Background(), "hosted", Spec{})
	if res.Target != "hosted" {
		t.Errorf("registry should auto-fill Target: %q", res.Target)
	}
}
