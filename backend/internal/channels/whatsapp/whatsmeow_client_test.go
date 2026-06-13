// Copyright 2026 Qorven AI. All rights reserved.
package whatsapp

import "testing"

func TestParseWhatsmeowJID_BareNumber(t *testing.T) {
	jid, err := parseWhatsmeowJID("15551234567")
	if err != nil {
		t.Fatal(err)
	}
	if jid.User != "15551234567" || jid.Server == "" {
		t.Fatalf("unexpected JID: %+v", jid)
	}
}
func TestParseWhatsmeowJID_Empty(t *testing.T) {
	if _, err := parseWhatsmeowJID(""); err == nil {
		t.Fatal("expected error for empty recipient")
	}
}
func TestChunkText_SplitsAt4096(t *testing.T) {
	long := make([]byte, 9000)
	for i := range long {
		long[i] = 'a'
	}
	chunks := chunkText(string(long), 4096)
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		if len(c) > 4096 {
			t.Fatalf("chunk too long: %d", len(c))
		}
	}
}
func TestQRHub_FanOutAndReplay(t *testing.T) {
	w := &WhatsAppChannel{}
	got := make(chan string, 2)
	unsub := w.SubscribeQREvents(func(s string) { got <- s })
	defer unsub()
	w.publishQR("CODE1")
	if s := <-got; s != "CODE1" {
		t.Fatalf("want CODE1, got %q", s)
	}
	got2 := make(chan string, 1)
	unsub2 := w.SubscribeQREvents(func(s string) { got2 <- s })
	defer unsub2()
	if s := <-got2; s != "CODE1" {
		t.Fatalf("late subscriber want CODE1, got %q", s)
	}
}
