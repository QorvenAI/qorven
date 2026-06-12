// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"context"
	"strings"
	"testing"
)

// These exercise the no-network platform-fix branches added in Phase A: the
// metadata-required guards (Reddit subreddit, Pinterest board_id) and the
// GMB direct-unsupported message. They never hit a real API.

func TestPublishWithMeta_RedditRequiresSubreddit(t *testing.T) {
	p := NewPublisher()
	res, _ := p.PublishWithMeta(context.Background(), PlatformReddit, "tok", "hello", nil, nil)
	if res == nil || res.Success {
		t.Fatal("Reddit without a subreddit must fail")
	}
	if !strings.Contains(res.Error, "subreddit") {
		t.Fatalf("expected subreddit error, got %q", res.Error)
	}
}

func TestPublishWithMeta_PinterestRequiresMediaThenBoard(t *testing.T) {
	p := NewPublisher()
	// No media → media error (before any board check).
	res, _ := p.PublishWithMeta(context.Background(), PlatformPinterest, "tok", "pin", nil, map[string]string{"board_id": "b1"})
	if res == nil || res.Success || !strings.Contains(res.Error, "image") {
		t.Fatalf("Pinterest without media must fail with an image error, got %+v", res)
	}
	// Media present but no board_id → board error (no network call reached).
	res2, _ := p.PublishWithMeta(context.Background(), PlatformPinterest, "tok", "pin", []string{"https://x/y.jpg"}, nil)
	if res2 == nil || res2.Success || !strings.Contains(res2.Error, "board") {
		t.Fatalf("Pinterest without board_id must fail with a board error, got %+v", res2)
	}
}

func TestPublishWithMeta_GMBUnsupportedDirect(t *testing.T) {
	p := NewPublisher()
	res, _ := p.PublishWithMeta(context.Background(), PlatformGoogleMyBiz, "tok", "hi", nil, nil)
	if res == nil || res.Success || !strings.Contains(res.Error, "relay") {
		t.Fatalf("GMB direct must fail pointing to a relay, got %+v", res)
	}
}

// PublishWithMeta with nil meta must behave exactly like the legacy Publish
// (back-compat for the non-metadata platforms).
func TestPublish_DelegatesToPublishWithMeta(t *testing.T) {
	p := NewPublisher()
	// An unknown platform returns a clear not-supported result via either entry.
	a, _ := p.Publish(context.Background(), Platform("bogus"), "t", "c", nil)
	b, _ := p.PublishWithMeta(context.Background(), Platform("bogus"), "t", "c", nil, nil)
	if a == nil || b == nil || a.Success || b.Success {
		t.Fatal("unknown platform must fail via both entry points")
	}
}
