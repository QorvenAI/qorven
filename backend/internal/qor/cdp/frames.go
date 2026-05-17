// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cdp

import (
	"sync"
	"time"
)

// frames.go — Frame hierarchy tracking for multi-frame pages (iframes).

// FrameRecord represents a browser frame in the CDP frame tree.
type FrameRecord struct {
	FrameID       string `json:"frame_id"`
	ParentFrameID string `json:"parent_frame_id"`
	URL           string `json:"url,omitempty"`
	Name          string `json:"name,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	LastUpdated   int64  `json:"last_updated"`
}

// FrameGraph tracks the browser frame hierarchy for CDP sessions.
type FrameGraph struct {
	mu       sync.RWMutex
	frames   map[string]*FrameRecord
	children map[string][]string // parentFrameID → child frameIDs
	indexMap  map[int]string      // index → frameID
	idIndex  map[string]int      // frameID → index
}

func NewFrameGraph() *FrameGraph {
	return &FrameGraph{
		frames:   make(map[string]*FrameRecord),
		children: make(map[string][]string),
		indexMap:  make(map[int]string),
		idIndex:  make(map[string]int),
	}
}

func (g *FrameGraph) Get(frameID string) *FrameRecord {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.frames[frameID]
}

func (g *FrameGraph) GetChildren(parentID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]string, len(g.children[parentID]))
	copy(out, g.children[parentID])
	return out
}

func (g *FrameGraph) AllFrames() []*FrameRecord {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]*FrameRecord, 0, len(g.frames))
	for _, f := range g.frames { out = append(out, f) }
	return out
}

// Upsert adds or updates a frame, maintaining parent-child relationships.
func (g *FrameGraph) Upsert(r FrameRecord) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if r.LastUpdated == 0 { r.LastUpdated = time.Now().UnixMilli() }

	existing := g.frames[r.FrameID]
	if existing != nil {
		if r.URL == "" { r.URL = existing.URL }
		if r.Name == "" { r.Name = existing.Name }
		if r.SessionID == "" { r.SessionID = existing.SessionID }
		prevParent := existing.ParentFrameID
		g.frames[r.FrameID] = &r
		g.updateParent(r.FrameID, prevParent, r.ParentFrameID)
	} else {
		g.frames[r.FrameID] = &r
		g.updateParent(r.FrameID, "", r.ParentFrameID)
	}
}

// Remove removes a frame and all its children recursively.
func (g *FrameGraph) Remove(frameID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.removeLocked(frameID)
}

func (g *FrameGraph) removeLocked(frameID string) {
	rec := g.frames[frameID]
	if rec == nil { return }
	delete(g.frames, frameID)

	if kids, ok := g.children[rec.ParentFrameID]; ok {
		filtered := kids[:0]
		for _, id := range kids { if id != frameID { filtered = append(filtered, id) } }
		g.children[rec.ParentFrameID] = filtered
	}

	if idx, ok := g.idIndex[frameID]; ok {
		delete(g.indexMap, idx)
		delete(g.idIndex, frameID)
	}

	childIDs := g.children[frameID]
	delete(g.children, frameID)
	for _, childID := range childIDs { g.removeLocked(childID) }
}

// AssignIndex maps a frame to a numeric index for reference.
func (g *FrameGraph) AssignIndex(frameID string, index int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.indexMap[index] = frameID
	g.idIndex[frameID] = index
}

// FrameByIndex returns the frame ID for a given index.
func (g *FrameGraph) FrameByIndex(index int) (string, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	id, ok := g.indexMap[index]
	return id, ok
}

// Clear resets all frame tracking state.
func (g *FrameGraph) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.frames = make(map[string]*FrameRecord)
	g.children = make(map[string][]string)
	g.indexMap = make(map[int]string)
	g.idIndex = make(map[string]int)
}

// Count returns the number of tracked frames.
func (g *FrameGraph) Count() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.frames)
}

func (g *FrameGraph) updateParent(frameID, prevParent, nextParent string) {
	if prevParent == nextParent { return }
	if kids, ok := g.children[prevParent]; ok {
		filtered := kids[:0]
		for _, id := range kids { if id != frameID { filtered = append(filtered, id) } }
		g.children[prevParent] = filtered
	}
	kids := g.children[nextParent]
	for _, id := range kids { if id == frameID { return } }
	g.children[nextParent] = append(kids, frameID)
}
