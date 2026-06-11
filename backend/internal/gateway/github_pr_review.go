// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// patchLine is one rendered diff line with its true file line numbers.
type patchLine struct {
	Type    string `json:"type"` // "add" | "del" | "eq"
	Content string `json:"content"`
	OldLine int    `json:"old_line"`
	NewLine int    `json:"new_line"`
}

// parseUnifiedPatch turns a GitHub file `patch` (unified diff) into line-numbered
// rows. Pure: the basis for both rendering and comment anchoring.
func parseUnifiedPatch(patch string) []patchLine {
	out := []patchLine{}
	oldN, newN := 0, 0
	for _, raw := range strings.Split(patch, "\n") {
		if strings.HasPrefix(raw, "@@") {
			// Header form: "@@ -oldStart,oldCount +newStart,newCount @@ [context]".
			// The -/+ ranges are always the 2nd and 3rd fields; scan ONLY those so
			// a trailing function context (which GitHub includes and may itself
			// contain +/- tokens, e.g. "func foo(-x, +y)") can't corrupt counters.
			fields := strings.Fields(raw)
			for i := 1; i < len(fields) && i <= 2; i++ {
				f := fields[i]
				if strings.HasPrefix(f, "-") {
					oldN = atoiStart(f[1:]) - 1
				} else if strings.HasPrefix(f, "+") {
					newN = atoiStart(f[1:]) - 1
				}
			}
			continue
		}
		if raw == "" {
			continue
		}
		switch raw[0] {
		case '+':
			newN++
			out = append(out, patchLine{Type: "add", Content: raw[1:], NewLine: newN})
		case '-':
			oldN++
			out = append(out, patchLine{Type: "del", Content: raw[1:], OldLine: oldN})
		case '\\':
			continue
		default:
			oldN++
			newN++
			out = append(out, patchLine{Type: "eq", Content: strings.TrimPrefix(raw, " "), OldLine: oldN, NewLine: newN})
		}
	}
	return out
}

// atoiStart parses the leading integer of "12,3" → 12.
func atoiStart(s string) int {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	return n
}

// reviewAnchor returns the (line, side) a GitHub review comment uses for a line.
func reviewAnchor(l patchLine) (int, string) {
	if l.Type == "del" {
		return l.OldLine, "LEFT"
	}
	return l.NewLine, "RIGHT"
}

// ghPost forwards a POST request to the GitHub REST API and returns the raw JSON.
// Mirrors ghProxy's auth, headers, base URL, and HTTP client exactly.
func (gw *Gateway) ghPost(ctx context.Context, path string, body any) (json.RawMessage, int, error) {
	tok := gw.ghVaultToken(ctx)
	if tok == "" {
		return nil, 401, fmt.Errorf("no GitHub token configured — add one in Settings → Provider Keys → GitHub")
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, 500, fmt.Errorf("marshal body: %w", err)
	}

	u := "https://api.github.com" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 500, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 500, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))

	if resp.StatusCode >= 400 {
		var ghErr struct {
			Message string `json:"message"`
		}
		json.Unmarshal(respBody, &ghErr)
		msg := ghErr.Message
		if msg == "" {
			msg = fmt.Sprintf("GitHub API error %d", resp.StatusCode)
		}
		return nil, resp.StatusCode, fmt.Errorf("%s", msg)
	}

	return json.RawMessage(respBody), resp.StatusCode, nil
}

// handleGitHubPRFiles proxies GET /repos/{owner}/{repo}/pulls/{n}/files and
// enriches each file entry with parsed unified-diff line data.
func (gw *Gateway) handleGitHubPRFiles(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num := chi.URLParam(r, "n")

	data, _, err := gw.ghProxy(r.Context(), fmt.Sprintf("/repos/%s/%s/pulls/%s/files", owner, repo, num), url.Values{"per_page": {"100"}})
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}

	var ghFiles []struct {
		Filename  string `json:"filename"`
		Status    string `json:"status"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Patch     string `json:"patch"`
	}
	_ = json.Unmarshal(data, &ghFiles)

	files := []map[string]any{}
	for _, f := range ghFiles {
		files = append(files, map[string]any{
			"path":      f.Filename,
			"status":    f.Status,
			"additions": f.Additions,
			"deletions": f.Deletions,
			"lines":     parseUnifiedPatch(f.Patch),
			"binary":    f.Patch == "",
		})
	}
	writeJSON(w, 200, map[string]any{"files": files})
}

// handleGitHubPRReview proxies POST /repos/{owner}/{repo}/pulls/{n}/reviews —
// submits a review (body, event, comments) to GitHub on behalf of the caller.
func (gw *Gateway) handleGitHubPRReview(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	num := chi.URLParam(r, "n")

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}

	data, status, err := gw.ghPost(r.Context(), fmt.Sprintf("/repos/%s/%s/pulls/%s/reviews", owner, repo, num), body)
	if err != nil {
		// Surface GitHub's real status (e.g. 422 "invalid event") so the review
		// UI can branch on it; fall back to 502 only for transport failures
		// where ghPost couldn't reach GitHub at all.
		code := status
		if code < 400 {
			code = http.StatusBadGateway
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}
