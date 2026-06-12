// Copyright 2026 Qorven AI. All rights reserved.
package drive

import "testing"

func TestStore_ScopeMethodSet(t *testing.T) {
	_ = (*Store).GetFile
	_ = (*Store).ListVisible
	_ = (*Store).SetScope
	_ = (*Store).CreateFileScoped
}
