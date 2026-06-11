package lsp

import "testing"

func TestResolveServer_UnknownLang(t *testing.T) {
	if _, _, ok := resolveServer("cobol"); ok {
		t.Error("unknown lang should be unavailable")
	}
}
func TestLanguageForExt(t *testing.T) {
	if LanguageForExt(".go") != "go" {
		t.Error("go")
	}
	if LanguageForExt(".tsx") != "typescript" {
		t.Error("tsx")
	}
	if LanguageForExt(".txt") != "" {
		t.Error("unknown ext → empty")
	}
}
