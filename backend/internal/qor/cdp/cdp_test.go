// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cdp

import (
	"sync"
	"testing"
	"time"
)

// Hard tests — concurrency, frame hierarchy, edge cases.

func TestFrameGraph_New(t *testing.T) {
	g := NewFrameGraph()
	if g.Count() != 0 { t.Error("new graph should be empty") }
}

func TestFrameGraph_Upsert(t *testing.T) {
	g := NewFrameGraph()
	g.Upsert(FrameRecord{FrameID: "f1", URL: "https://example.com"})
	if g.Count() != 1 { t.Errorf("count=%d", g.Count()) }
	f := g.Get("f1")
	if f == nil { t.Fatal("frame not found") }
	if f.URL != "https://example.com" { t.Errorf("url=%q", f.URL) }
}

func TestFrameGraph_Upsert_Update(t *testing.T) {
	g := NewFrameGraph()
	g.Upsert(FrameRecord{FrameID: "f1", URL: "https://old.com", Name: "old"})
	g.Upsert(FrameRecord{FrameID: "f1", URL: "https://new.com"})
	f := g.Get("f1")
	if f.URL != "https://new.com" { t.Error("URL not updated") }
	if f.Name != "old" { t.Error("Name should be preserved when empty in update") }
}

func TestFrameGraph_ParentChild(t *testing.T) {
	g := NewFrameGraph()
	g.Upsert(FrameRecord{FrameID: "parent"})
	g.Upsert(FrameRecord{FrameID: "child1", ParentFrameID: "parent"})
	g.Upsert(FrameRecord{FrameID: "child2", ParentFrameID: "parent"})
	children := g.GetChildren("parent")
	if len(children) != 2 { t.Errorf("children=%d", len(children)) }
}

func TestFrameGraph_Remove_Recursive(t *testing.T) {
	g := NewFrameGraph()
	g.Upsert(FrameRecord{FrameID: "root"})
	g.Upsert(FrameRecord{FrameID: "child", ParentFrameID: "root"})
	g.Upsert(FrameRecord{FrameID: "grandchild", ParentFrameID: "child"})
	if g.Count() != 3 { t.Errorf("before remove: %d", g.Count()) }
	g.Remove("root")
	if g.Count() != 0 { t.Errorf("after remove root: %d (should cascade)", g.Count()) }
}

func TestFrameGraph_Remove_LeafOnly(t *testing.T) {
	g := NewFrameGraph()
	g.Upsert(FrameRecord{FrameID: "root"})
	g.Upsert(FrameRecord{FrameID: "child", ParentFrameID: "root"})
	g.Remove("child")
	if g.Count() != 1 { t.Errorf("root should remain: %d", g.Count()) }
	if len(g.GetChildren("root")) != 0 { t.Error("root should have no children") }
}

func TestFrameGraph_Remove_Nonexistent(t *testing.T) {
	g := NewFrameGraph()
	g.Remove("nonexistent") // should not panic
}

func TestFrameGraph_AssignIndex(t *testing.T) {
	g := NewFrameGraph()
	g.Upsert(FrameRecord{FrameID: "f1"})
	g.AssignIndex("f1", 0)
	id, ok := g.FrameByIndex(0)
	if !ok { t.Error("index not found") }
	if id != "f1" { t.Errorf("id=%q", id) }
}

func TestFrameGraph_FrameByIndex_NotFound(t *testing.T) {
	g := NewFrameGraph()
	_, ok := g.FrameByIndex(99)
	if ok { t.Error("should not find nonexistent index") }
}

func TestFrameGraph_Clear(t *testing.T) {
	g := NewFrameGraph()
	g.Upsert(FrameRecord{FrameID: "f1"})
	g.Upsert(FrameRecord{FrameID: "f2"})
	g.AssignIndex("f1", 0)
	g.Clear()
	if g.Count() != 0 { t.Error("should be empty after clear") }
	if _, ok := g.FrameByIndex(0); ok { t.Error("index should be cleared") }
}

func TestFrameGraph_AllFrames(t *testing.T) {
	g := NewFrameGraph()
	g.Upsert(FrameRecord{FrameID: "f1"})
	g.Upsert(FrameRecord{FrameID: "f2"})
	g.Upsert(FrameRecord{FrameID: "f3"})
	all := g.AllFrames()
	if len(all) != 3 { t.Errorf("expected 3, got %d", len(all)) }
}

func TestFrameGraph_ReparentFrame(t *testing.T) {
	g := NewFrameGraph()
	g.Upsert(FrameRecord{FrameID: "parent1"})
	g.Upsert(FrameRecord{FrameID: "parent2"})
	g.Upsert(FrameRecord{FrameID: "child", ParentFrameID: "parent1"})
	if len(g.GetChildren("parent1")) != 1 { t.Error("child should be under parent1") }
	// Reparent
	g.Upsert(FrameRecord{FrameID: "child", ParentFrameID: "parent2"})
	if len(g.GetChildren("parent1")) != 0 { t.Error("parent1 should have no children") }
	if len(g.GetChildren("parent2")) != 1 { t.Error("parent2 should have 1 child") }
}

func TestFrameGraph_LastUpdated_AutoSet(t *testing.T) {
	g := NewFrameGraph()
	g.Upsert(FrameRecord{FrameID: "f1"})
	f := g.Get("f1")
	if f.LastUpdated == 0 { t.Error("LastUpdated should be auto-set") }
}

func TestFrameGraph_ConcurrentAccess(t *testing.T) {
	g := NewFrameGraph()
	var wg sync.WaitGroup
	// 50 goroutines upserting
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			fid := "f" + string(rune('0'+id%10))
			g.Upsert(FrameRecord{FrameID: fid, URL: "https://example.com"})
			g.Get(fid)
			g.AllFrames()
			g.Count()
		}(i)
	}
	// 50 goroutines reading
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.AllFrames()
			g.GetChildren("")
			g.Count()
		}()
	}
	wg.Wait()
}

func TestFrameGraph_DeepHierarchy(t *testing.T) {
	g := NewFrameGraph()
	// Create 10-level deep hierarchy
	prev := ""
	for i := 0; i < 10; i++ {
		fid := "f" + string(rune('0'+i))
		g.Upsert(FrameRecord{FrameID: fid, ParentFrameID: prev})
		prev = fid
	}
	if g.Count() != 10 { t.Errorf("count=%d", g.Count()) }
	// Remove root should cascade all
	g.Remove("f0")
	if g.Count() != 0 { t.Errorf("after cascade: %d", g.Count()) }
}

func TestScriptInjector_New(t *testing.T) {
	si := NewScriptInjector()
	if si == nil { t.Fatal("nil injector") }
}

func TestScriptInjector_Reset(t *testing.T) {
	si := NewScriptInjector()
	si.registered["test"] = true
	si.evaluated["test"] = map[string]bool{"default": true}
	si.Reset()
	if len(si.registered) != 0 { t.Error("registered not cleared") }
	if len(si.evaluated) != 0 { t.Error("evaluated not cleared") }
}

func TestContextToken(t *testing.T) {
	if contextToken(0) != "default" { t.Error("0 should be default") }
	if contextToken(1) == "default" { t.Error("1 should not be default") }
}

func TestElement_Fields(t *testing.T) {
	el := Element{ObjectID: "obj1", NodeID: 42, Selector: "#btn", X: 100, Y: 200, Width: 50, Height: 30}
	if el.Selector != "#btn" { t.Error("wrong selector") }
	if el.X+el.Width/2 != 125 { t.Error("wrong center X") }
}

func TestFrameRecord_Timestamp(t *testing.T) {
	now := time.Now().UnixMilli()
	r := FrameRecord{FrameID: "f1", LastUpdated: now}
	if r.LastUpdated != now { t.Error("timestamp mismatch") }
}
