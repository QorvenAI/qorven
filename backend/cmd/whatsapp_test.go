package cmd

import (
	"strings"
	"testing"
)

func TestRenderQRBitmap_OutputContainsBlockChars(t *testing.T) {
	bitmap := [][]bool{
		{true, true, true},
		{true, false, true},
		{true, true, true},
	}
	out := renderQRBitmap(bitmap)
	if !strings.ContainsAny(out, "▀▄█") {
		t.Errorf("expected block characters in ANSI QR output, got:\n%s", out)
	}
}

func TestExtractQRBase64_ParsesDataURL(t *testing.T) {
	json := `{"type":"qr","qr":"data:image/png;base64,iVBORw=="}`
	got := extractQRBase64(json)
	if got != "iVBORw==" {
		t.Errorf("expected 'iVBORw==', got %q", got)
	}
}

func TestExtractQRBase64_ReturnsEmptyIfNotQR(t *testing.T) {
	json := `{"type":"connected","phone":"91XX"}`
	got := extractQRBase64(json)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
