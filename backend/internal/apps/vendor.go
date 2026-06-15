// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package apps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/config"
)

// vendorMu serializes first-time downloads so concurrent requests don't race.
var vendorMu sync.Mutex

// cachedScriptValid reports whether already-cached bytes may be served. When a
// pin (wantSHA256) is set, the cached copy is re-verified against it on every
// read, so a file cached during an earlier trust-on-first-use window (or a
// tampered cache) is rejected and re-downloaded rather than served blindly.
func cachedScriptValid(b []byte, wantSHA256 string) bool {
	if wantSHA256 == "" {
		return true // no pin configured — trust the cached same-origin copy
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]) == wantSHA256
}

// EnsureVendorScript returns the bytes of a vendored public-host script (e.g.
// React UMD), downloading it once from a PINNED HTTPS url into the data dir and
// caching it there. The cached copy is then served SAME-ORIGIN from Qorven, so
// external visitors never load from a third-party CDN (no SRI / CDN-compromise
// exposure at serve time). If wantSHA256 is non-empty, the one-time download is
// verified against it before caching.
func EnsureVendorScript(ctx context.Context, name, url, wantSHA256 string) ([]byte, error) {
	// Restrict name to a simple slug to keep the cache path safe.
	for _, c := range name {
		if !(c == '-' || (c >= 'a' && c <= 'z')) {
			return nil, fmt.Errorf("invalid vendor name %q", name)
		}
	}
	dir := config.Sub("public-vendor")
	path := filepath.Join(dir, name+".js")

	if b, err := os.ReadFile(path); err == nil && len(b) > 0 && cachedScriptValid(b, wantSHA256) {
		return b, nil
	}

	vendorMu.Lock()
	defer vendorMu.Unlock()
	// Re-check after acquiring the lock (another goroutine may have fetched it).
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 && cachedScriptValid(b, wantSHA256) {
		return b, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MB cap
	if err != nil {
		return nil, err
	}
	if wantSHA256 != "" {
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != wantSHA256 {
			return nil, fmt.Errorf("vendor %s sha256 mismatch: got %s", name, got)
		}
	}
	// Atomic write into the cache.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, err
	}
	return data, nil
}
