// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

// ensureWebsiteProfilesTable creates the website_profiles table if it does not exist.
func (gw *Gateway) ensureWebsiteProfilesTable(ctx context.Context) error {
	_, err := gw.db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS website_profiles (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id TEXT NOT NULL DEFAULT 'default',
			url TEXT NOT NULL,
			product_name TEXT,
			tagline TEXT,
			audience TEXT,
			competitors JSONB DEFAULT '[]',
			brand_voice TEXT,
			value_props JSONB DEFAULT '[]',
			keywords JSONB DEFAULT '[]',
			raw_content TEXT,
			analysis JSONB,
			created_at TIMESTAMPTZ DEFAULT now(),
			updated_at TIMESTAMPTZ DEFAULT now(),
			UNIQUE(tenant_id, url)
		)`)
	return err
}

// handleAnalyzeWebsite crawls a URL and generates a structured product profile via LLM.
// POST /v1/onboarding/analyze
func (gw *Gateway) handleAnalyzeWebsite(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url must start with http:// or https://"})
		return
	}

	// Ensure table exists
	if err := gw.ensureWebsiteProfilesTable(r.Context()); err != nil {
		slog.Error("onboarding.ensure_table", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to initialize storage"})
		return
	}

	// Step 1: Crawl the website
	crawlContent, err := gw.crawlWebsite(r.Context(), req.URL)
	if err != nil {
		slog.Warn("onboarding.crawl_failed", "url", req.URL, "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "failed to crawl website",
			"details": err.Error(),
		})
		return
	}

	// Step 2: Call LLM to analyze the content
	analysis, err := gw.analyzeWebsiteContent(r.Context(), req.URL, crawlContent)
	if err != nil {
		slog.Error("onboarding.analysis_failed", "url", req.URL, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to analyze website content"})
		return
	}

	// Step 3: Upsert into database
	profile, err := gw.upsertWebsiteProfile(r.Context(), req.URL, crawlContent, analysis)
	if err != nil {
		slog.Error("onboarding.upsert_failed", "url", req.URL, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save profile"})
		return
	}

	slog.Info("onboarding.analyze.done", "url", req.URL, "product", analysis.ProductName)
	writeJSON(w, http.StatusOK, profile)
}

// handleGetProfile returns the current website profile for the tenant.
// GET /v1/onboarding/profile
func (gw *Gateway) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	if err := gw.ensureWebsiteProfilesTable(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to initialize storage"})
		return
	}

	var profile websiteProfile
	err := gw.db.Pool.QueryRow(r.Context(), `
		SELECT id, tenant_id, url, COALESCE(product_name,''), COALESCE(tagline,''),
		       COALESCE(audience,''), COALESCE(competitors,'[]'), COALESCE(brand_voice,''),
		       COALESCE(value_props,'[]'), COALESCE(keywords,'[]'),
		       COALESCE(raw_content,''), COALESCE(analysis,'{}'),
		       created_at, updated_at
		FROM website_profiles
		WHERE tenant_id = 'default'
		ORDER BY updated_at DESC
		LIMIT 1
	`).Scan(
		&profile.ID, &profile.TenantID, &profile.URL,
		&profile.ProductName, &profile.Tagline, &profile.Audience,
		&profile.CompetitorsJSON, &profile.BrandVoice,
		&profile.ValuePropsJSON, &profile.KeywordsJSON,
		&profile.RawContent, &profile.AnalysisJSON,
		&profile.CreatedAt, &profile.UpdatedAt,
	)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no profile found"})
		return
	}

	writeJSON(w, http.StatusOK, profile.toResponse())
}

// handleUpdateProfile partially updates the website profile.
// PUT /v1/onboarding/profile
func (gw *Gateway) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	if err := gw.ensureWebsiteProfilesTable(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to initialize storage"})
		return
	}

	var req struct {
		ProductName *string  `json:"product_name"`
		Tagline     *string  `json:"tagline"`
		Audience    *string  `json:"audience"`
		BrandVoice  *string  `json:"brand_voice"`
		Competitors []string `json:"competitors"`
		ValueProps  []string `json:"value_props"`
		Keywords    []string `json:"keywords"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Build dynamic UPDATE
	sets := []string{"updated_at = now()"}
	args := []any{}
	argN := 1

	if req.ProductName != nil {
		sets = append(sets, fmt.Sprintf("product_name = $%d", argN))
		args = append(args, *req.ProductName)
		argN++
	}
	if req.Tagline != nil {
		sets = append(sets, fmt.Sprintf("tagline = $%d", argN))
		args = append(args, *req.Tagline)
		argN++
	}
	if req.Audience != nil {
		sets = append(sets, fmt.Sprintf("audience = $%d", argN))
		args = append(args, *req.Audience)
		argN++
	}
	if req.BrandVoice != nil {
		sets = append(sets, fmt.Sprintf("brand_voice = $%d", argN))
		args = append(args, *req.BrandVoice)
		argN++
	}
	if req.Competitors != nil {
		compJSON, _ := json.Marshal(req.Competitors)
		sets = append(sets, fmt.Sprintf("competitors = $%d", argN))
		args = append(args, string(compJSON))
		argN++
	}
	if req.ValueProps != nil {
		vpJSON, _ := json.Marshal(req.ValueProps)
		sets = append(sets, fmt.Sprintf("value_props = $%d", argN))
		args = append(args, string(vpJSON))
		argN++
	}
	if req.Keywords != nil {
		kwJSON, _ := json.Marshal(req.Keywords)
		sets = append(sets, fmt.Sprintf("keywords = $%d", argN))
		args = append(args, string(kwJSON))
		argN++
	}

	if len(args) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	query := fmt.Sprintf(
		"UPDATE website_profiles SET %s WHERE tenant_id = 'default' RETURNING id",
		strings.Join(sets, ", "),
	)

	var id string
	err := gw.db.Pool.QueryRow(r.Context(), query, args...).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no profile found to update"})
		return
	}

	// Return the updated profile
	gw.handleGetProfile(w, r)
}

// --- Internal helpers ---

type websiteAnalysis struct {
	ProductName string   `json:"product_name"`
	Tagline     string   `json:"tagline"`
	Audience    string   `json:"audience"`
	Competitors []string `json:"competitors"`
	BrandVoice  string   `json:"brand_voice"`
	ValueProps  []string `json:"value_propositions"`
	Keywords    []string `json:"keywords"`
}

type websiteProfile struct {
	ID              string
	TenantID        string
	URL             string
	ProductName     string
	Tagline         string
	Audience        string
	CompetitorsJSON string
	BrandVoice      string
	ValuePropsJSON  string
	KeywordsJSON    string
	RawContent      string
	AnalysisJSON    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (p *websiteProfile) toResponse() map[string]any {
	var competitors []string
	var valueProps []string
	var keywords []string
	var analysis map[string]any

	json.Unmarshal([]byte(p.CompetitorsJSON), &competitors) //nolint:errcheck
	json.Unmarshal([]byte(p.ValuePropsJSON), &valueProps)   //nolint:errcheck
	json.Unmarshal([]byte(p.KeywordsJSON), &keywords)       //nolint:errcheck
	json.Unmarshal([]byte(p.AnalysisJSON), &analysis)       //nolint:errcheck

	return map[string]any{
		"id":           p.ID,
		"tenant_id":    p.TenantID,
		"url":          p.URL,
		"product_name": p.ProductName,
		"tagline":      p.Tagline,
		"audience":     p.Audience,
		"competitors":  competitors,
		"brand_voice":  p.BrandVoice,
		"value_props":  valueProps,
		"keywords":     keywords,
		"raw_content":  p.RawContent,
		"analysis":     analysis,
		"created_at":   p.CreatedAt,
		"updated_at":   p.UpdatedAt,
	}
}

// crawlWebsite calls the crawl service to scrape the homepage.
func (gw *Gateway) crawlWebsite(ctx context.Context, url string) (string, error) {
	crawlURL := "https://api.qorven.ai/crawl/v1"
	if env := os.Getenv("QOR_CRAWL_URL"); env != "" {
		crawlURL = env
	}

	apiToken := os.Getenv("CRAWL4AI_API_TOKEN")
	if apiToken == "" {
		return "", fmt.Errorf("crawl service not configured (CRAWL4AI_API_TOKEN not set)")
	}

	body, _ := json.Marshal(map[string]any{
		"url":     url,
		"formats": []string{"markdown"},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", crawlURL+"/scrape", strings.NewReader(string(body)))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiToken)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("crawl service unavailable: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("crawl service returned %d: %s", resp.StatusCode, string(data[:min(len(data), 200)]))
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Markdown string `json:"markdown"`
		} `json:"data"`
	}
	json.Unmarshal(data, &result) //nolint:errcheck

	if !result.Success || result.Data.Markdown == "" {
		return "", fmt.Errorf("crawl returned no content")
	}

	content := result.Data.Markdown
	if len(content) > 15000 {
		content = content[:15000]
	}

	return content, nil
}

// analyzeWebsiteContent uses the prime agent to extract structured product info.
func (gw *Gateway) analyzeWebsiteContent(ctx context.Context, url, content string) (*websiteAnalysis, error) {
	if gw.agentLoop == nil {
		return nil, fmt.Errorf("agent loop not initialized")
	}

	agentID := gw.agentLoop.PrimeID
	if agentID == "" {
		return nil, fmt.Errorf("prime agent not configured")
	}

	prompt := fmt.Sprintf(`Analyze this website content and extract a structured product profile.
Website URL: %s

Content:
%s

Extract the following and return ONLY valid JSON (no markdown, no explanation):
{
  "product_name": "the product/company name",
  "tagline": "their main tagline or value proposition in one sentence",
  "audience": "description of target audience (who they serve)",
  "competitors": ["competitor1", "competitor2", "competitor3"],
  "brand_voice": "description of their brand voice/tone (professional, casual, technical, etc.)",
  "value_propositions": ["value prop 1", "value prop 2", "value prop 3"],
  "keywords": ["keyword1", "keyword2", "keyword3", "keyword4", "keyword5"]
}

Be specific and factual based on the content. If something cannot be determined, use a reasonable inference or empty string.`, url, truncateStr(content, 8000))

	resp, err := gw.agentLoop.Chat(ctx, agentID, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM analysis failed: %w", err)
	}

	// Parse JSON from the response
	var analysis websiteAnalysis
	start := strings.Index(resp, "{")
	end := strings.LastIndex(resp, "}") + 1
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(resp[start:end]), &analysis); err != nil {
			return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
		}
	} else {
		return nil, fmt.Errorf("LLM response did not contain valid JSON")
	}

	return &analysis, nil
}

// upsertWebsiteProfile inserts or updates the profile in the database.
func (gw *Gateway) upsertWebsiteProfile(ctx context.Context, url, rawContent string, analysis *websiteAnalysis) (map[string]any, error) {
	competitorsJSON, _ := json.Marshal(analysis.Competitors)
	valuePropsJSON, _ := json.Marshal(analysis.ValueProps)
	keywordsJSON, _ := json.Marshal(analysis.Keywords)
	analysisJSON, _ := json.Marshal(analysis)

	var profile websiteProfile
	err := gw.db.Pool.QueryRow(ctx, `
		INSERT INTO website_profiles (tenant_id, url, product_name, tagline, audience, competitors, brand_voice, value_props, keywords, raw_content, analysis)
		VALUES ('default', $1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (tenant_id, url) DO UPDATE SET
			product_name = EXCLUDED.product_name,
			tagline = EXCLUDED.tagline,
			audience = EXCLUDED.audience,
			competitors = EXCLUDED.competitors,
			brand_voice = EXCLUDED.brand_voice,
			value_props = EXCLUDED.value_props,
			keywords = EXCLUDED.keywords,
			raw_content = EXCLUDED.raw_content,
			analysis = EXCLUDED.analysis,
			updated_at = now()
		RETURNING id, tenant_id, url, product_name, tagline, audience, competitors::text, brand_voice, value_props::text, keywords::text, raw_content, analysis::text, created_at, updated_at
	`, url, analysis.ProductName, analysis.Tagline, analysis.Audience,
		string(competitorsJSON), analysis.BrandVoice,
		string(valuePropsJSON), string(keywordsJSON),
		rawContent, string(analysisJSON),
	).Scan(
		&profile.ID, &profile.TenantID, &profile.URL,
		&profile.ProductName, &profile.Tagline, &profile.Audience,
		&profile.CompetitorsJSON, &profile.BrandVoice,
		&profile.ValuePropsJSON, &profile.KeywordsJSON,
		&profile.RawContent, &profile.AnalysisJSON,
		&profile.CreatedAt, &profile.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return profile.toResponse(), nil
}
