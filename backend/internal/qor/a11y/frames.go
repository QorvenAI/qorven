// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package a11y

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// frames.go — Collect frame contexts for multi-frame accessibility tree capture.

// FrameInfo describes a browser frame.
type FrameInfo struct {
	FrameID  string `json:"frame_id"`
	ParentID string `json:"parent_id,omitempty"`
	URL      string `json:"url"`
	Name     string `json:"name,omitempty"`
}

// CollectFrames returns all frames in the current page's frame tree.
func CollectFrames(ctx context.Context) ([]FrameInfo, error) {
	var tree *page.FrameTree
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		tree, err = page.GetFrameTree().Do(ctx)
		return err
	})); err != nil {
		return nil, fmt.Errorf("a11y.frames: %w", err)
	}

	var frames []FrameInfo
	collectFrameTree(tree, "", &frames)
	return frames, nil
}

func collectFrameTree(tree *page.FrameTree, parentID string, out *[]FrameInfo) {
	if tree == nil || tree.Frame == nil { return }
	f := FrameInfo{
		FrameID:  string(tree.Frame.ID),
		ParentID: parentID,
		URL:      tree.Frame.URL,
		Name:     tree.Frame.Name,
	}
	*out = append(*out, f)
	for _, child := range tree.ChildFrames {
		collectFrameTree(child, f.FrameID, out)
	}
}

// FetchTreeForFrame fetches the accessibility tree for a specific frame.
func FetchTreeForFrame(ctx context.Context, frameID string) (*DOMState, error) {
	// For now, delegate to the main FetchTree — chromedp handles frame context
	// In the future, we can use Page.getFrameTree + per-frame accessibility queries
	return FetchTree(ctx)
}
