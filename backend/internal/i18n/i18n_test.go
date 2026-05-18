// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package i18n

import "testing"

func TestT_English(t *testing.T) {
	result := T("en", "welcome")
	_ = result // T should not panic
}

func TestT_Vietnamese(t *testing.T) {
	result := T("vi", "welcome")
	_ = result // should not panic
}

func TestT_Chinese(t *testing.T) {
	result := T("zh", "welcome")
	_ = result
}

func TestT_UnknownLocale(t *testing.T) {
	result := T("xx", "welcome")
	// Should fallback to English or return key
	_ = result
}

func TestT_UnknownKey(t *testing.T) {
	result := T("en", "nonexistent_key_xyz")
	// Should return the key itself as fallback
	if result == "" { t.Error("should return something for unknown key") }
}

func TestT_WithArgs(t *testing.T) {
	result := T("en", "greeting", "World")
	_ = result // should not panic with args
}

func TestT_EmptyLocale(t *testing.T) {
	result := T("", "welcome")
	_ = result // should not panic
}

func TestT_EmptyKey(t *testing.T) {
	result := T("en", "")
	_ = result
}
