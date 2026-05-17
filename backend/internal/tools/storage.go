// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/qorvenai/qorven/internal/storage"
)

// Storage tools wrap rclone via internal/storage.Manager. Six verbs,
// each a separate tool so the LLM doesn't need to learn a verb arg:
//
//   storage_remotes  — list configured remotes
//   storage_list     — list files at a remote path
//   storage_read     — read a single file
//   storage_write    — write a small payload (<100 MiB)
//   storage_copy     — cross-backend copy (src → dst)
//   storage_sync     — mirror src → dst (dangerous; gated)
//
// Every tool returns a clear error when rclone isn't installed so the
// LLM can tell the user "install rclone from rclone.org/downloads and
// run `rclone config`".

// --- storage_remotes ---

type StorageRemotesTool struct{ mgr *storage.Manager }

func NewStorageRemotesTool(m *storage.Manager) *StorageRemotesTool {
	return &StorageRemotesTool{mgr: m}
}

func (t *StorageRemotesTool) Name() string { return "storage_remotes" }
func (t *StorageRemotesTool) Description() string {
	return "List configured cloud storage remotes (S3, Google Drive, Dropbox, OneDrive, " +
		"Azure Blob, Backblaze B2, SFTP, WebDAV, etc). Returns each remote's name and type. " +
		"Users configure remotes by running `rclone config` on the host. " +
		"No credentials are stored by Qorven — rclone owns them."
}
func (t *StorageRemotesTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *StorageRemotesTool) Execute(ctx context.Context, args map[string]any) *Result {
	remotes, err := t.mgr.ListRemotes(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if len(remotes) == 0 {
		return TextResult("No remotes configured. Run `rclone config` on the host to add one.")
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Configured remotes (%d):\n", len(remotes)))
	for _, r := range remotes {
		sb.WriteString(fmt.Sprintf("  - %s: (%s)\n", r.Name, r.Type))
	}
	return TextResult(sb.String())
}

// --- storage_list ---

type StorageListTool struct{ mgr *storage.Manager }

func NewStorageListTool(m *storage.Manager) *StorageListTool {
	return &StorageListTool{mgr: m}
}

func (t *StorageListTool) Name() string { return "storage_list" }
func (t *StorageListTool) Description() string {
	return "List files and directories at a cloud storage path. " +
		"Path format: `remote:path` (e.g. `gdrive:Documents`, `s3:my-bucket/logs/`). " +
		"Use storage_remotes first to discover configured remote names. " +
		"Non-recursive by default; pass recursive=true for up to 3 levels deep."
}
func (t *StorageListTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Remote path, e.g. `gdrive:Documents/invoices`. Must include a configured remote name and a colon.",
			},
			"recursive": map[string]any{
				"type":        "boolean",
				"description": "Recurse into subdirectories (up to 3 levels). Default false.",
			},
		},
		"required": []string{"path"},
	}
}
func (t *StorageListTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	if path == "" {
		return ErrorResult("path is required — e.g. \"gdrive:Documents\"")
	}
	recursive, _ := args["recursive"].(bool)

	entries, err := t.mgr.LSJSON(ctx, path, recursive)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if len(entries) == 0 {
		return TextResult(fmt.Sprintf("No entries at %s", path))
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d entries at %s:\n", len(entries), path))
	for _, e := range entries {
		kind := "file"
		if e.IsDir {
			kind = "dir "
		}
		size := ""
		if !e.IsDir {
			size = humanSize(e.Size)
		}
		sb.WriteString(fmt.Sprintf("  [%s] %-10s %s\n", kind, size, e.Path))
	}
	return TextResult(sb.String())
}

// --- storage_read ---

type StorageReadTool struct{ mgr *storage.Manager }

func NewStorageReadTool(m *storage.Manager) *StorageReadTool { return &StorageReadTool{mgr: m} }

func (t *StorageReadTool) Name() string { return "storage_read" }
func (t *StorageReadTool) Description() string {
	return "Read a file from cloud storage. Path format: `remote:path/to/file`. " +
		"Capped at 5 MiB — for larger files, use storage_copy to bring it to the local workspace first."
}
func (t *StorageReadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Remote file path, e.g. `s3:bucket/logs/app.log`.",
			},
			"max_bytes": map[string]any{
				"type":        "integer",
				"description": "Max bytes to read (default 5 MiB, hard cap 5 MiB).",
			},
		},
		"required": []string{"path"},
	}
}
func (t *StorageReadTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	if path == "" {
		return ErrorResult("path is required")
	}
	maxBytes := 0
	if n, ok := toInt(args["max_bytes"]); ok {
		maxBytes = n
	}

	data, err := t.mgr.Cat(ctx, path, maxBytes)
	if err != nil {
		return ErrorResult(err.Error())
	}
	// Quick binary-content heuristic: if the first 1 KiB contains a
	// null byte, the file probably isn't text. Surface a warning
	// instead of dumping nulls into the context.
	sniff := data
	if len(sniff) > 1024 {
		sniff = sniff[:1024]
	}
	for _, b := range sniff {
		if b == 0 {
			return TextResult(fmt.Sprintf(
				"Binary file (%s); refusing to inline. Use storage_copy to download it locally, then a dedicated tool to parse.",
				humanSize(int64(len(data)))))
		}
	}
	return TextResult(string(data))
}

// --- storage_write ---

type StorageWriteTool struct{ mgr *storage.Manager }

func NewStorageWriteTool(m *storage.Manager) *StorageWriteTool {
	return &StorageWriteTool{mgr: m}
}

func (t *StorageWriteTool) Name() string { return "storage_write" }
func (t *StorageWriteTool) Description() string {
	return "Write a small payload (< 100 MiB) to cloud storage. " +
		"Disabled by default; administrators enable it via Settings → Storage. " +
		"Path format: `remote:path/to/file`. Content is the bytes to upload."
}
func (t *StorageWriteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Remote file path to create/overwrite.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Bytes to write. UTF-8 encoded.",
			},
		},
		"required": []string{"path", "content"},
	}
}
func (t *StorageWriteTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" || content == "" {
		return ErrorResult("path and content are required")
	}
	if err := t.mgr.WriteFile(ctx, path, []byte(content)); err != nil {
		return ErrorResult(err.Error())
	}
	return TextResult(fmt.Sprintf("Wrote %s to %s", humanSize(int64(len(content))), path))
}

// --- storage_copy ---

type StorageCopyTool struct{ mgr *storage.Manager }

func NewStorageCopyTool(m *storage.Manager) *StorageCopyTool { return &StorageCopyTool{mgr: m} }

func (t *StorageCopyTool) Name() string { return "storage_copy" }
func (t *StorageCopyTool) Description() string {
	return "Copy a file or directory between cloud storage remotes (or between local and remote). " +
		"At least one of src/dst must be a `remote:path`. Non-destructive: existing dst files " +
		"are overwritten but none are deleted. Disabled by default — requires write access."
}
func (t *StorageCopyTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"src": map[string]any{"type": "string", "description": "Source path (remote:path or local)."},
			"dst": map[string]any{"type": "string", "description": "Destination path (remote:path or local)."},
		},
		"required": []string{"src", "dst"},
	}
}
func (t *StorageCopyTool) Execute(ctx context.Context, args map[string]any) *Result {
	src, _ := args["src"].(string)
	dst, _ := args["dst"].(string)
	if src == "" || dst == "" {
		return ErrorResult("src and dst are required")
	}
	status, err := t.mgr.Copy(ctx, src, dst)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return TextResult("Copied: " + status)
}

// --- storage_sync ---

type StorageSyncTool struct{ mgr *storage.Manager }

func NewStorageSyncTool(m *storage.Manager) *StorageSyncTool { return &StorageSyncTool{mgr: m} }

func (t *StorageSyncTool) Name() string { return "storage_sync" }
func (t *StorageSyncTool) Description() string {
	return "MIRROR src to dst — deletes files on dst that aren't in src. " +
		"DESTRUCTIVE. Requires confirm=\"YES-DELETE-DIVERGENT\" to proceed. " +
		"Use storage_copy if you don't need divergent files deleted on the destination."
}
func (t *StorageSyncTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"src":     map[string]any{"type": "string", "description": "Source (remote:path or local)."},
			"dst":     map[string]any{"type": "string", "description": "Destination to overwrite (remote:path or local)."},
			"confirm": map[string]any{"type": "string", "description": "Must equal YES-DELETE-DIVERGENT to proceed."},
		},
		"required": []string{"src", "dst", "confirm"},
	}
}
func (t *StorageSyncTool) Execute(ctx context.Context, args map[string]any) *Result {
	src, _ := args["src"].(string)
	dst, _ := args["dst"].(string)
	confirm, _ := args["confirm"].(string)
	if src == "" || dst == "" {
		return ErrorResult("src and dst are required")
	}
	status, err := t.mgr.Sync(ctx, src, dst, confirm)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return TextResult("Synced: " + status)
}

// --- helpers ---

func humanSize(n int64) string {
	const (
		K = 1024
		M = K * 1024
		G = M * 1024
	)
	switch {
	case n >= G:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(G))
	case n >= M:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(M))
	case n >= K:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(K))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// suppress unused-import warning when we only use encoding/json indirectly via storage package
var _ = json.Marshal
