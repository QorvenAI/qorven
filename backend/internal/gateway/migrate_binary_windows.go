// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

// migrateBinaryToOpt is a no-op on Windows — the binary lives in
// C:\Program Files\Qorven\ which is writable by LocalSystem.
func migrateBinaryToOpt() {}
