// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package connectors

import "context"

// SeedStorageUploadActions registers the "upload_file" action for storage
// providers so Drive can mirror files OUT to a user's cloud drive. Uploads run
// through the Pipedream relay (multipart/resumable cloud upload APIs don't fit
// the direct-HTTP JSON path). PipedreamActionID values follow Pipedream's
// component convention — confirm against the live catalog before relying on a
// real push (the param shape is the integration's soft spot).
//
// ID convention (from audit of existing seeds):
//   - Google apps use underscore in the app slug: google_drive, google_calendar, etc.
//   - Single-word apps use all-hyphens: dropbox-upload-file
//   - microsoft_onedrive follows the google_ prefix pattern by analogy — unverified
//     against the live Pipedream catalog; verify before production push.
func SeedStorageUploadActions(ctx context.Context, ks *KnowledgeStore) {
	actions := []ActionDef{
		{
			PlatformID: "google-drive", ActionKey: "upload_file",
			Name: "Upload File", Description: "Upload a file to Google Drive",
			WhenToUse:        "When mirroring a Qorven Drive file out to Google Drive",
			Method:           "POST", Path: "/upload/drive/v3/files",
			ExecutionBackend: "pipedream",
			// google_drive follows the google_* underscore pattern seen in
			// google_analytics, google_calendar, google_search_console seeds.
			PipedreamActionID: "google_drive-upload-file",
			ResponseDesc:      "Created file object with id",
		},
		{
			PlatformID: "microsoft-onedrive", ActionKey: "upload_file",
			Name: "Upload File", Description: "Upload a file to OneDrive",
			WhenToUse:        "When mirroring a Qorven Drive file out to OneDrive",
			Method:           "PUT", Path: "/me/drive/root:/{name}:/content",
			ExecutionBackend: "pipedream",
			// microsoft_onedrive: underscore pattern by analogy with google_* —
			// unverified against live Pipedream catalog.
			PipedreamActionID: "microsoft_onedrive-upload-file",
			ResponseDesc:      "Created file object with id",
		},
		{
			PlatformID: "dropbox", ActionKey: "upload_file",
			Name: "Upload File", Description: "Upload a file to Dropbox",
			WhenToUse:        "When mirroring a Qorven Drive file out to Dropbox",
			Method:           "POST", Path: "/files/upload",
			ExecutionBackend: "pipedream",
			// dropbox: single-word app slug, all-hyphens pattern (dropbox-upload-file).
			PipedreamActionID: "dropbox-upload-file",
			ResponseDesc:      "Created file metadata",
		},
	}
	for _, a := range actions {
		_ = ks.UpsertAction(ctx, a)
	}
}
