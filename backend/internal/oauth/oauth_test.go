// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package oauth

import "testing"

func TestProviderConfig_Fields(t *testing.T) {
	cfg := ProviderConfig{ID: "google", ClientID: "xxx", ClientSecret: "yyy", AuthURL: "https://accounts.google.com/o/oauth2/auth", TokenURL: "https://oauth2.googleapis.com/token"}
	if cfg.ID != "google" { t.Error("wrong id") }
}

func TestManager_New(t *testing.T) {
	m := NewManager(nil, "https://app.qorven.io")
	if m == nil { t.Fatal("nil") }
}

func TestManager_RegisterProvider(t *testing.T) {
	m := NewManager(nil, "https://app.qorven.io")
	m.RegisterProvider(ProviderConfig{ID: "google", ClientID: "xxx"})
}

func TestManager_GetProvider_Found(t *testing.T) {
	m := NewManager(nil, "https://app.qorven.io")
	m.RegisterProvider(ProviderConfig{ID: "google"})
	_, ok := m.GetProvider("google")
	if !ok { t.Error("should find") }
}

func TestManager_GetProvider_NotFound(t *testing.T) {
	m := NewManager(nil, "https://app.qorven.io")
	_, ok := m.GetProvider("nonexistent")
	if ok { t.Error("should not find") }
}

func TestManager_ListProviders(t *testing.T) {
	m := NewManager(nil, "https://app.qorven.io")
	m.RegisterProvider(ProviderConfig{ID: "google"})
	m.RegisterProvider(ProviderConfig{ID: "github"})
	list := m.ListProviders()
	if len(list) != 2 { t.Errorf("expected 2, got %d", len(list)) }
}

func TestManager_RedirectURL(t *testing.T) {
	m := NewManager(nil, "https://app.qorven.io")
	url := m.RedirectURL("google")
	if url == "" { t.Error("empty redirect URL") }
}

func TestTokenResponse_Fields(t *testing.T) {
	tr := TokenResponse{AccessToken: "abc", RefreshToken: "def", ExpiresIn: 3600}
	if tr.AccessToken != "abc" { t.Error("wrong token") }
	if tr.ExpiresIn != 3600 { t.Error("wrong expiry") }
}
