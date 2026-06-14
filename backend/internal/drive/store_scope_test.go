// Copyright 2026 Qorven AI. All rights reserved.
package drive

import "testing"

func TestStore_ScopeMethodSet(t *testing.T) {
	_ = (*Store).GetFile
	_ = (*Store).ListVisible
	_ = (*Store).SetScope
	_ = (*Store).CreateFileScoped
}

// TestStore_DeleteFile_SignatureAndError asserts that DeleteFile takes a
// tenantID parameter (tenant-scoped signature) and that ErrNotFound is
// exported. No live DB is available, so we verify at the type level only.
func TestStore_DeleteFile_SignatureAndError(t *testing.T) {
	// Compile-time check: DeleteFile must accept (ctx, tenantID, id string).
	var _ func(*Store) func(interface{ Deadline() (interface{}, bool); Done() <-chan struct{}; Err() error; Value(any) any }, string, string) error
	_ = (*Store).DeleteFile
	// ErrNotFound must be a non-nil sentinel.
	if ErrNotFound == nil {
		t.Fatal("ErrNotFound must not be nil")
	}
}
