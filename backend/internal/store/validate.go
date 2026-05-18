// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import "fmt"

// MaxUserIDLength is the maximum allowed length for user identifier strings
// (user_id, owner_id, granted_by, requested_by, reviewed_by, etc.).
// Matches the VARCHAR(255) constraint in the database schema.
const MaxUserIDLength = 255

// ValidateUserID checks that a user identifier does not exceed MaxUserIDLength.
func ValidateUserID(id string) error {
	if len(id) > MaxUserIDLength {
		return fmt.Errorf("user identifier too long: %d chars (max %d)", len(id), MaxUserIDLength)
	}
	return nil
}
