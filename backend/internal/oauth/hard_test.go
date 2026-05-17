// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package oauth

import "testing"

func TestHard_OAuth_ManagerLifecycle(t *testing.T) {
	m := NewManager(nil, "https://app.qorven.io")
	providers := []ProviderConfig{
		{ID: "google", ClientID: "goog-id", ClientSecret: "goog-secret", AuthURL: "https://accounts.google.com/o/oauth2/auth", TokenURL: "https://oauth2.googleapis.com/token", Scopes: []string{"email", "profile"}},
		{ID: "github", ClientID: "gh-id", ClientSecret: "gh-secret", AuthURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", Scopes: []string{"repo", "user"}},
		{ID: "slack", ClientID: "sl-id", ClientSecret: "sl-secret", AuthURL: "https://slack.com/oauth/v2/authorize", TokenURL: "https://slack.com/api/oauth.v2.access"},
	}
	for _, p := range providers { m.RegisterProvider(p) }

	list := m.ListProviders()
	if len(list) != 3 { t.Errorf("providers=%d", len(list)) }

	for _, id := range []string{"google", "github", "slack"} {
		p, ok := m.GetProvider(id)
		if !ok { t.Errorf("%s not found", id) }
		if p.ClientID == "" { t.Errorf("%s empty client_id", id) }
		url := m.RedirectURL(id)
		if url == "" { t.Errorf("%s empty redirect", id) }
	}
}

func TestHard_OAuth_AuthorizeURL(t *testing.T) {
	m := NewManager(nil, "https://app.qorven.io")
	m.RegisterProvider(ProviderConfig{
		ID: "google", ClientID: "test-id",
		AuthURL: "https://accounts.google.com/o/oauth2/auth",
		Scopes: []string{"email"},
	})
	url, err := m.AuthorizeURL("google", "random-state-123")
	if err != nil { t.Fatal(err) }
	if url == "" { t.Error("empty URL") }
	t.Logf("authorize URL: %s", url[:min10(len(url), 100)])
}

func min10(a, b int) int { if a < b { return a }; return b }
