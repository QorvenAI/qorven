// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tunnel

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeProvider is a test Provider that returns canned results from Start.
type fakeProvider struct {
	name      string
	url       string
	startErr  error
	startGate chan struct{} // if non-nil, Start blocks until closed
	stopped   chan struct{}
}

func newFakeProvider(name, url string, startErr error) *fakeProvider {
	return &fakeProvider{
		name:     name,
		url:      url,
		startErr: startErr,
		stopped:  make(chan struct{}, 1),
	}
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Start(ctx context.Context, localURL string) (string, error) {
	if f.startGate != nil {
		select {
		case <-f.startGate:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.startErr != nil {
		return "", f.startErr
	}
	return f.url, nil
}

func (f *fakeProvider) Stop() error {
	select {
	case f.stopped <- struct{}{}:
	default:
	}
	return nil
}

// fixedNow returns a nowFn yielding a constant time.
func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// waitForState polls the manager until it reaches want or the deadline passes.
func waitForState(t *testing.T, m *Manager, want State) Status {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := m.Status()
		if s.State == want {
			return s
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("manager did not reach state %q; last status: %+v", want, m.Status())
	return Status{}
}

func TestManager_InitialState(t *testing.T) {
	m := NewManager()
	s := m.Status()
	if s.State != StateOff {
		t.Fatalf("initial state = %q, want %q", s.State, StateOff)
	}
	if s.PublicURL != "" || s.Provider != "" {
		t.Fatalf("unexpected initial status: %+v", s)
	}
}

func TestManager_EnableConnected(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	m := NewManager()
	m.nowFn = fixedNow(now)

	p := newFakeProvider("fake", "https://abc-def.trycloudflare.com", nil)
	if err := m.Enable(p, "http://localhost:8487"); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	s := waitForState(t, m, StateConnected)
	if s.PublicURL != p.url {
		t.Fatalf("PublicURL = %q, want %q", s.PublicURL, p.url)
	}
	if s.Provider != "fake" {
		t.Fatalf("Provider = %q, want %q", s.Provider, "fake")
	}
	if s.Error != "" {
		t.Fatalf("Error = %q, want empty", s.Error)
	}
	if s.Since != now.Format(time.RFC3339) {
		t.Fatalf("Since = %q, want %q", s.Since, now.Format(time.RFC3339))
	}
}

func TestManager_EnableError(t *testing.T) {
	m := NewManager()
	wantErr := errors.New("boom")
	p := newFakeProvider("fake", "", wantErr)

	if err := m.Enable(p, "http://localhost:8487"); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	s := waitForState(t, m, StateError)
	if s.Error != wantErr.Error() {
		t.Fatalf("Error = %q, want %q", s.Error, wantErr.Error())
	}
	if s.PublicURL != "" {
		t.Fatalf("PublicURL = %q, want empty", s.PublicURL)
	}
}

func TestManager_Disable(t *testing.T) {
	m := NewManager()
	p := newFakeProvider("fake", "https://x-y.trycloudflare.com", nil)

	if err := m.Enable(p, "http://localhost:8487"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	waitForState(t, m, StateConnected)

	if err := m.Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	s := m.Status()
	if s.State != StateOff {
		t.Fatalf("state after Disable = %q, want %q", s.State, StateOff)
	}
	if s.Provider != "" || s.PublicURL != "" {
		t.Fatalf("status not cleared after Disable: %+v", s)
	}

	select {
	case <-p.stopped:
	case <-time.After(time.Second):
		t.Fatal("provider.Stop was not called by Disable")
	}
}

func TestManager_DisableWhenOff(t *testing.T) {
	m := NewManager()
	if err := m.Disable(); err != nil {
		t.Fatalf("Disable on idle manager: %v", err)
	}
	if m.Status().State != StateOff {
		t.Fatalf("state = %q, want %q", m.Status().State, StateOff)
	}
}

func TestManager_EnableReplacesActive(t *testing.T) {
	m := NewManager()

	first := newFakeProvider("first", "https://first.trycloudflare.com", nil)
	if err := m.Enable(first, "http://localhost:8487"); err != nil {
		t.Fatalf("Enable first: %v", err)
	}
	waitForState(t, m, StateConnected)

	second := newFakeProvider("second", "https://second.trycloudflare.com", nil)
	if err := m.Enable(second, "http://localhost:8488"); err != nil {
		t.Fatalf("Enable second: %v", err)
	}

	// First provider should have been stopped.
	select {
	case <-first.stopped:
	case <-time.After(time.Second):
		t.Fatal("first provider was not stopped when replaced")
	}

	s := waitForState(t, m, StateConnected)
	if s.Provider != "second" || s.PublicURL != second.url {
		t.Fatalf("active provider not replaced: %+v", s)
	}
}

func TestManager_EnableNilProvider(t *testing.T) {
	m := NewManager()
	if err := m.Enable(nil, "http://localhost:8487"); err == nil {
		t.Fatal("Enable(nil) = nil error, want error")
	}
}

func TestExtractQuickTunnelURL(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "boxed banner line",
			line: "2026-06-10T12:00:00Z INF |  https://random-words-here.trycloudflare.com  |",
			want: "https://random-words-here.trycloudflare.com",
		},
		{
			name: "plain url",
			line: "https://abc-123-def.trycloudflare.com",
			want: "https://abc-123-def.trycloudflare.com",
		},
		{
			name: "registered tunnel log line",
			line: "INF Registered tunnel connection at https://foo-bar-baz.trycloudflare.com edge=1",
			want: "https://foo-bar-baz.trycloudflare.com",
		},
		{
			name: "no url present",
			line: "INF Requesting new quick Tunnel on trycloudflare.com...",
			want: "",
		},
		{
			name: "non-quick-tunnel host ignored",
			line: "INF connecting to https://example.com",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractQuickTunnelURL(tc.line); got != tc.want {
				t.Fatalf("extractQuickTunnelURL(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

func TestCloudflaredAssetName(t *testing.T) {
	cases := []struct {
		goos, goarch string
		want         string
		wantErr      bool
	}{
		{"linux", "amd64", "cloudflared-linux-amd64", false},
		{"linux", "arm64", "cloudflared-linux-arm64", false},
		{"darwin", "amd64", "cloudflared-darwin-amd64", false},
		{"darwin", "arm64", "cloudflared-darwin-arm64", false},
		{"windows", "amd64", "cloudflared-windows-amd64.exe", false},
		{"windows", "arm64", "cloudflared-windows-arm64.exe", false},
		{"linux", "386", "", true},
		{"plan9", "amd64", "", true},
	}
	for _, tc := range cases {
		got, err := cloudflaredAssetName(tc.goos, tc.goarch)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("cloudflaredAssetName(%q,%q) err = nil, want error", tc.goos, tc.goarch)
			}
			continue
		}
		if err != nil {
			t.Fatalf("cloudflaredAssetName(%q,%q) unexpected err: %v", tc.goos, tc.goarch, err)
		}
		if got != tc.want {
			t.Fatalf("cloudflaredAssetName(%q,%q) = %q, want %q", tc.goos, tc.goarch, got, tc.want)
		}
	}
}
