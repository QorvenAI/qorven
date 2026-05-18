// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package heartbeat

import "testing"

func TestStore_New(t *testing.T) {
	s := NewStore(nil)
	if s == nil { t.Fatal("nil store") }
}
