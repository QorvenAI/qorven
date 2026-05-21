// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !windows

package gateway

// stopWindowsService is a no-op on non-Windows platforms.
func stopWindowsService() {}
