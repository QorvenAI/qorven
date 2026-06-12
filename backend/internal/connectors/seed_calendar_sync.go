// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package connectors

import "context"

// SeedCalendarSyncTargets registers Zoho Calendar (Google Calendar is already
// seeded) so calendars can sync OUT to it. create_event runs through the
// Pipedream relay; the PipedreamActionID follows Pipedream's component
// convention and is UNVERIFIED against the live catalog (failures record as
// errors, not silent no-ops).
func SeedCalendarSyncTargets(ctx context.Context, ks *KnowledgeStore) {
	_ = ks.UpsertPlatform(ctx, Platform{
		ID: "zoho-calendar", Name: "Zoho Calendar", Category: "productivity",
		Description: "Create and manage Zoho Calendar events", Icon: "calendar",
		AuthType: "oauth2",
		AuthConfig: j(`{"auth_url":"https://accounts.zoho.com/oauth/v2/auth","token_url":"https://accounts.zoho.com/oauth/v2/token"}`),
		BaseURL: "https://calendar.zoho.com/api/v1", DocsURL: "https://www.zoho.com/calendar/help/api/", Enabled: true,
	})
	_ = ks.UpsertAction(ctx, ActionDef{
		PlatformID: "zoho-calendar", ActionKey: "create_event",
		Name: "Create Event", Description: "Create a Zoho Calendar event",
		WhenToUse:        "When syncing a Qorven calendar item out to Zoho Calendar",
		Method:           "POST", Path: "/calendars/events",
		ExecutionBackend: "pipedream", PipedreamActionID: "zoho_calendar-create-event",
		ResponseDesc:     "Created event with id",
	})
}
