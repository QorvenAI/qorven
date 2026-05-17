// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package bus

import "github.com/google/uuid"

// BroadcastForTenant broadcasts an event with explicit tenant scoping.
func BroadcastForTenant(pub EventPublisher, name string, tenantID uuid.UUID, payload any) {
	pub.Broadcast(Event{Name: name, TenantID: tenantID, Payload: payload})
}
