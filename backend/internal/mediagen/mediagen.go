// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package mediagen manages image and video generation providers.
// It mirrors the voice package pattern: a DB-backed store, a Manager
// that holds the live provider instances, and a catalog of drivers.
package mediagen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/crypto"
)

// ─── Interfaces ──────────────────────────────────────────────────────────────

type ImageProvider interface {
	Name() string
	Generate(ctx context.Context, prompt string, opts ImageOptions) (*ImageResult, error)
}

type ImageOptions struct {
	Size    string // "1024x1024", "1792x1024", "1024x1792"
	Quality string // "standard", "hd"
	Style   string // "vivid", "natural"
	Model   string
	N       int
}

type ImageResult struct {
	URL      string `json:"url,omitempty"`
	B64JSON  string `json:"b64_json,omitempty"`
	MimeType string `json:"mime_type"`
}

// VideoProvider generates video from a text prompt.
type VideoProvider interface {
	Name() string
	Generate(ctx context.Context, prompt string, opts VideoOptions) (*VideoResult, error)
}

type VideoOptions struct {
	// Duration in seconds (provider-specific default if 0).
	Duration    int    `json:"duration,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"` // "16:9", "9:16", "1:1"
	Resolution  string `json:"resolution,omitempty"`   // "480p", "720p", "1080p"
	Model       string `json:"model,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`     // optional first-frame image
	EndImageURL string `json:"end_image_url,omitempty"` // optional last-frame image (Seedance)
}

type VideoResult struct {
	// URL to the generated video (may be a temporary pre-signed URL).
	URL      string `json:"url,omitempty"`
	// Polling task ID for async providers (Runway, Kling).
	TaskID   string `json:"task_id,omitempty"`
	// Polling endpoint to resolve TaskID into a final URL.
	PollURL  string `json:"poll_url,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

// ─── Manager ─────────────────────────────────────────────────────────────────

type Manager struct {
	imageProviders map[string]ImageProvider
	videoProviders map[string]VideoProvider
	primaryImage   string
	fallbackImage  []string
	primaryVideo   string
}

func NewManager() *Manager {
	return &Manager{
		imageProviders: make(map[string]ImageProvider),
		videoProviders: make(map[string]VideoProvider),
	}
}

func (m *Manager) RegisterImage(p ImageProvider) {
	m.imageProviders[p.Name()] = p
	if m.primaryImage == "" {
		m.primaryImage = p.Name()
	}
	slog.Info("mediagen: image provider registered", "name", p.Name())
}

func (m *Manager) RegisterVideo(p VideoProvider) {
	m.videoProviders[p.Name()] = p
	if m.primaryVideo == "" {
		m.primaryVideo = p.Name()
	}
	slog.Info("mediagen: video provider registered", "name", p.Name())
}

func (m *Manager) SetPrimaryImage(name string)    { m.primaryImage = name }
func (m *Manager) SetFallbackImage(names []string) { m.fallbackImage = names }
func (m *Manager) SetPrimaryVideo(name string)    { m.primaryVideo = name }
func (m *Manager) HasImage() bool                  { return len(m.imageProviders) > 0 }
func (m *Manager) HasVideo() bool                  { return len(m.videoProviders) > 0 }

func (m *Manager) GenerateImage(ctx context.Context, prompt string, opts ImageOptions) (*ImageResult, error) {
	order := []string{m.primaryImage}
	order = append(order, m.fallbackImage...)
	for _, name := range order {
		if name == "" {
			continue
		}
		p, ok := m.imageProviders[name]
		if !ok {
			continue
		}
		result, err := p.Generate(ctx, prompt, opts)
		if err != nil {
			slog.Warn("mediagen: image provider failed, trying fallback", "provider", name, "err", err)
			continue
		}
		return result, nil
	}
	if !m.HasImage() {
		return nil, fmt.Errorf("no image generation provider configured — add one in Settings → Models Hub → Media")
	}
	return nil, fmt.Errorf("all image providers failed for this request")
}

func (m *Manager) GenerateVideo(ctx context.Context, prompt string, opts VideoOptions) (*VideoResult, error) {
	if m.primaryVideo == "" {
		return nil, fmt.Errorf("no video generation provider configured — add one in Settings → Models Hub → Media")
	}
	p, ok := m.videoProviders[m.primaryVideo]
	if !ok {
		return nil, fmt.Errorf("video provider %q not found", m.primaryVideo)
	}
	return p.Generate(ctx, prompt, opts)
}

func (m *Manager) ListProviders() map[string]any {
	imageNames := make([]string, 0, len(m.imageProviders))
	for n := range m.imageProviders {
		imageNames = append(imageNames, n)
	}
	videoNames := make([]string, 0, len(m.videoProviders))
	for n := range m.videoProviders {
		videoNames = append(videoNames, n)
	}
	return map[string]any{
		"image":         imageNames,
		"primary_image": m.primaryImage,
		"fallback":      m.fallbackImage,
		"video":         videoNames,
		"primary_video": m.primaryVideo,
	}
}

// ─── OpenAI DALL-E ───────────────────────────────────────────────────────────

type OpenAIImageProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewOpenAIImageProvider(apiKey, baseURL, model string) *OpenAIImageProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "dall-e-3"
	}
	return &OpenAIImageProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAIImageProvider) Name() string { return "openai_image" }

func (p *OpenAIImageProvider) Generate(ctx context.Context, prompt string, opts ImageOptions) (*ImageResult, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}
	size := opts.Size
	if size == "" {
		size = "1024x1024"
	}
	quality := opts.Quality
	if quality == "" {
		quality = "standard"
	}
	n := opts.N
	if n <= 0 {
		n = 1
	}

	body, _ := json.Marshal(map[string]any{
		"model":           model,
		"prompt":          prompt,
		"n":               n,
		"size":            size,
		"quality":         quality,
		"response_format": "url",
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai image: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openai image %d: %s", resp.StatusCode, string(data[:min(len(data), 300)]))
	}

	var result struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil || len(result.Data) == 0 {
		return nil, fmt.Errorf("openai image: unexpected response")
	}

	return &ImageResult{
		URL:      result.Data[0].URL,
		B64JSON:  result.Data[0].B64JSON,
		MimeType: "image/png",
	}, nil
}

// ─── Stability AI ────────────────────────────────────────────────────────────

type StabilityImageProvider struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewStabilityImageProvider(apiKey, baseURL, model string) *StabilityImageProvider {
	if baseURL == "" {
		baseURL = "https://api.stability.ai/v2beta"
	}
	if model == "" {
		model = "stable-image/generate/core"
	}
	return &StabilityImageProvider{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *StabilityImageProvider) Name() string { return "stability" }

func (p *StabilityImageProvider) Generate(ctx context.Context, prompt string, opts ImageOptions) (*ImageResult, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}

	// Stability AI v2beta requires multipart/form-data, not JSON.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("prompt", prompt)
	mw.WriteField("output_format", "png")
	mw.Close()

	req, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/"+model, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "image/*")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stability: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("stability %d: %s", resp.StatusCode, string(data[:min(len(data), 300)]))
	}

	return &ImageResult{
		B64JSON:  base64.StdEncoding.EncodeToString(data),
		MimeType: "image/png",
	}, nil
}

// ─── OpenAI-compatible image (Groq, Together, etc.) ─────────────────────────

type OpenAICompatImageProvider struct {
	name    string
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewOpenAICompatImageProvider(name, apiKey, baseURL, model string) *OpenAICompatImageProvider {
	return &OpenAICompatImageProvider{
		name:    name,
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAICompatImageProvider) Name() string { return p.name }

func (p *OpenAICompatImageProvider) Generate(ctx context.Context, prompt string, opts ImageOptions) (*ImageResult, error) {
	model := opts.Model
	if model == "" {
		model = p.model
	}
	size := opts.Size
	if size == "" {
		size = "1024x1024"
	}

	body, _ := json.Marshal(map[string]any{
		"model":           model,
		"prompt":          prompt,
		"n":               1,
		"size":            size,
		"response_format": "url",
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s image: %w", p.name, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s image %d: %s", p.name, resp.StatusCode, string(data[:min(len(data), 300)]))
	}

	var result struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil || len(result.Data) == 0 {
		return nil, fmt.Errorf("%s image: unexpected response", p.name)
	}

	return &ImageResult{
		URL:      result.Data[0].URL,
		B64JSON:  result.Data[0].B64JSON,
		MimeType: "image/png",
	}, nil
}

// ─── DB Store ────────────────────────────────────────────────────────────────

type ProviderRow struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id,omitempty"`
	Name          string          `json:"name"`
	Kind          string          `json:"kind"`   // image | video | audio_gen
	Driver        string          `json:"driver"` // catalog id
	APIBase       string          `json:"api_base"`
	APIKey        string          `json:"-"`
	Settings      json.RawMessage `json:"settings"`
	Enabled       bool            `json:"enabled"`
	IsDefault     bool            `json:"is_default"`
	FallbackOrder int             `json:"fallback_order"`
}

type Store struct {
	pool   *pgxpool.Pool
	encKey string
}

func NewStore(pool *pgxpool.Pool, encryptionKey string) *Store {
	return &Store{pool: pool, encKey: encryptionKey}
}

func (s *Store) encrypt(plain string) (string, error) {
	if plain == "" || s.encKey == "" {
		return plain, nil
	}
	return crypto.EncryptString(plain, s.encKey)
}

func (s *Store) decrypt(cipher string) string {
	if cipher == "" || s.encKey == "" {
		return cipher
	}
	out, err := crypto.DecryptString(cipher, s.encKey)
	if err != nil {
		return ""
	}
	return out
}

func (s *Store) List(ctx context.Context, tenantID string) ([]ProviderRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, name, kind, driver, api_base, api_key_enc,
		        settings, enabled, is_default, fallback_order
		 FROM media_providers WHERE tenant_id=$1 ORDER BY fallback_order, created_at`,
		tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProviderRow{}
	for rows.Next() {
		var r ProviderRow
		var keyEnc string
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Kind, &r.Driver,
			&r.APIBase, &keyEnc, &r.Settings, &r.Enabled, &r.IsDefault, &r.FallbackOrder); err != nil {
			return nil, err
		}
		r.APIKey = s.decrypt(keyEnc)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Create(ctx context.Context, tenantID string, r ProviderRow) (ProviderRow, error) {
	enc, err := s.encrypt(r.APIKey)
	if err != nil {
		return ProviderRow{}, err
	}
	settings := r.Settings
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}
	var id string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO media_providers (tenant_id, name, kind, driver, api_base, api_key_enc, settings, enabled, is_default, fallback_order)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		tenantID, r.Name, r.Kind, r.Driver, r.APIBase, enc, settings, r.Enabled, r.IsDefault, r.FallbackOrder,
	).Scan(&id)
	if err != nil {
		return ProviderRow{}, err
	}
	r.ID = id
	return r, nil
}

func (s *Store) SetDefault(ctx context.Context, tenantID, id, kind string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE media_providers SET is_default=FALSE WHERE tenant_id=$1 AND kind=$2`, tenantID, kind)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE media_providers SET is_default=TRUE WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	return err
}

func (s *Store) Delete(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM media_providers WHERE id=$1 AND tenant_id=$2`, id, tenantID)
	return err
}

func (s *Store) GetByID(ctx context.Context, tenantID, id string) (*ProviderRow, error) {
	var r ProviderRow
	var keyEnc string
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, kind, driver, api_base, api_key_enc, settings, enabled, is_default, fallback_order
		 FROM media_providers WHERE id=$1 AND tenant_id=$2`, id, tenantID).
		Scan(&r.ID, &r.TenantID, &r.Name, &r.Kind, &r.Driver, &r.APIBase, &keyEnc,
			&r.Settings, &r.Enabled, &r.IsDefault, &r.FallbackOrder)
	if err != nil {
		return nil, err
	}
	r.APIKey = s.decrypt(keyEnc)
	return &r, nil
}

func (s *Store) Update(ctx context.Context, tenantID string, r ProviderRow) error {
	enc, err := s.encrypt(r.APIKey)
	if err != nil {
		return err
	}
	if enc == "" {
		// Don't overwrite key if not provided
		_, err = s.pool.Exec(ctx,
			`UPDATE media_providers SET name=$1, driver=$2, api_base=$3, settings=$4, enabled=$5, is_default=$6, fallback_order=$7, updated_at=NOW()
			 WHERE id=$8 AND tenant_id=$9`,
			r.Name, r.Driver, r.APIBase, r.Settings, r.Enabled, r.IsDefault, r.FallbackOrder, r.ID, tenantID)
	} else {
		_, err = s.pool.Exec(ctx,
			`UPDATE media_providers SET name=$1, driver=$2, api_base=$3, api_key_enc=$4, settings=$5, enabled=$6, is_default=$7, fallback_order=$8, updated_at=NOW()
			 WHERE id=$9 AND tenant_id=$10`,
			r.Name, r.Driver, r.APIBase, enc, r.Settings, r.Enabled, r.IsDefault, r.FallbackOrder, r.ID, tenantID)
	}
	return err
}

// ─── Builder ─────────────────────────────────────────────────────────────────

// BuildProvider builds an image provider from a ProviderRow (kind=image).
func BuildProvider(r ProviderRow) (ImageProvider, error) {
	settings := parseSettings(r.Settings)
	switch r.Driver {
	case "openai_dalle", "openai":
		base := r.APIBase
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		return NewOpenAIImageProvider(r.APIKey, base, settings.String("model")), nil
	case "stability":
		return NewStabilityImageProvider(r.APIKey, r.APIBase, settings.String("model")), nil
	case "openai_compat":
		model := settings.String("model")
		name := r.Name
		if name == "" {
			name = r.Driver
		}
		return NewOpenAICompatImageProvider(name, r.APIKey, r.APIBase, model), nil
	case "together_image":
		base := firstNonEmpty(r.APIBase, "https://api.together.xyz/v1")
		return NewOpenAICompatImageProvider("together_image", r.APIKey, base, firstNonEmpty(settings.String("model"), "black-forest-labs/FLUX.1-schnell-Free")), nil
	case "fal":
		base := firstNonEmpty(r.APIBase, "https://fal.run")
		return NewOpenAICompatImageProvider("fal", r.APIKey, base, settings.String("model")), nil
	}
	return nil, fmt.Errorf("mediagen: unknown image driver %q", r.Driver)
}

// BuildVideoProvider builds a video provider from a ProviderRow (kind=video).
func BuildVideoProvider(r ProviderRow) (VideoProvider, error) {
	settings := parseSettings(r.Settings)
	switch r.Driver {
	case "runway":
		return NewRunwayVideoProvider(r.APIKey, settings.String("model")), nil
	case "kling":
		return NewKlingVideoProvider(r.APIKey, settings.String("model")), nil
	case "fal_video":
		return NewFalVideoProvider(r.APIKey, firstNonEmpty(settings.String("model"), "bytedance/seedance-2.0/text-to-video")), nil
	case "openrouter_video":
		return NewOpenRouterVideoProvider(r.APIKey, firstNonEmpty(settings.String("model"), "bytedance/seedance-2.0")), nil
	case "seedance":
		return NewSeedanceVideoProvider(r.APIKey, firstNonEmpty(settings.String("model"), "seedance-2.0")), nil
	}
	return nil, fmt.Errorf("mediagen: unknown video driver %q", r.Driver)
}

// ─── Catalog ─────────────────────────────────────────────────────────────────

type CatalogEntry struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Hosting      string       `json:"hosting"` // cloud | local
	Hint         string       `json:"hint"`
	Fields       []CatalogField `json:"fields"`
	DefaultBase  string       `json:"default_base,omitempty"`
	Models       []string     `json:"models,omitempty"`
}

type CatalogField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"`   // text | password
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
}

func Catalog() []CatalogEntry {
	apiKey := func(label, placeholder string) []CatalogField {
		return []CatalogField{
			{Name: "api_key", Label: label, Type: "password", Required: true, Placeholder: placeholder},
		}
	}
	return []CatalogEntry{
		{
			ID: "openai_dalle", Name: "OpenAI DALL-E", Kind: "image", Hosting: "cloud",
			Hint:        "DALL-E 3 and DALL-E 2. High quality, creative images. ~$0.04/image.",
			DefaultBase: "https://api.openai.com/v1",
			Models:      []string{"dall-e-3", "dall-e-2"},
			Fields: append(apiKey("OpenAI API Key", "sk-..."), CatalogField{
				Name: "model", Label: "Default model", Type: "text", Placeholder: "dall-e-3",
			}),
		},
		{
			ID: "stability", Name: "Stability AI", Kind: "image", Hosting: "cloud",
			Hint:        "Stable Diffusion and SDXL. Ultra-photorealistic, fast. ~$0.01/image.",
			DefaultBase: "https://api.stability.ai/v2beta",
			Models:      []string{"stable-image/generate/core", "stable-image/generate/sd3", "stable-image/generate/ultra"},
			Fields: append(apiKey("Stability API Key", "sk-..."), CatalogField{
				Name: "model", Label: "Model endpoint", Type: "text", Placeholder: "stable-image/generate/core",
			}),
		},
		{
			ID: "together_image", Name: "Together AI (FLUX)", Kind: "image", Hosting: "cloud",
			Hint:        "FLUX.1-schnell-Free is free, FLUX.1-pro is high quality. Fast generation.",
			DefaultBase: "https://api.together.xyz/v1",
			Models:      []string{"black-forest-labs/FLUX.1-schnell-Free", "black-forest-labs/FLUX.1-pro", "black-forest-labs/FLUX.1.1-pro"},
			Fields: append(apiKey("Together AI API Key", ""), CatalogField{
				Name: "model", Label: "Model", Type: "text", Placeholder: "black-forest-labs/FLUX.1-schnell-Free",
			}),
		},
		// ── Video providers ──
		{
			ID: "runway", Name: "Runway ML", Kind: "video", Hosting: "cloud",
			Hint:        "Gen-4 Turbo — state-of-the-art text-to-video and image-to-video. 5–10s clips.",
			DefaultBase: "https://api.dev.runwayml.com/v1",
			Models:      []string{"gen4_turbo", "gen3a_turbo"},
			Fields: []CatalogField{
				{Name: "api_key", Label: "Runway API Key", Type: "password", Required: true, Placeholder: "rw-..."},
				{Name: "model", Label: "Model", Type: "text", Required: false, Placeholder: "gen4_turbo"},
			},
		},
		{
			ID: "kling", Name: "Kling AI", Kind: "video", Hosting: "cloud",
			Hint:        "High-quality 5s/10s video generation from text or image. Fast and cost-effective.",
			DefaultBase: "https://api.klingai.com/v1",
			Models:      []string{"kling-v1-5", "kling-v1", "kling-v2-master"},
			Fields: []CatalogField{
				{Name: "api_key", Label: "Kling API Key", Type: "password", Required: true, Placeholder: "..."},
				{Name: "model", Label: "Model", Type: "text", Required: false, Placeholder: "kling-v1-5"},
			},
		},
		// ── Seedance & multi-model video ──
		{
			ID: "seedance", Name: "Seedance (ByteDance official)", Kind: "video", Hosting: "cloud",
			Hint:        "Direct BytePlus API — Seedance 2.0 (#2 globally, ELO 1346). Requires BytePlus account.",
			DefaultBase: "https://visual.byteplus.com",
			Models:      []string{"seedance-2.0", "seedance-1.5", "seedance-1.5-lite"},
			Fields: []CatalogField{
				{Name: "api_key", Label: "BytePlus Access Key", Type: "password", Required: true, Placeholder: "AccessKey from console.byteplus.com"},
				{Name: "model", Label: "Model", Type: "text", Required: false, Placeholder: "seedance-2.0"},
			},
		},
		{
			ID: "fal_video", Name: "Fal.ai Video (Seedance / PixVerse / HappyHorse)", Kind: "video", Hosting: "cloud",
			Hint:        "600+ video models via one API. Seedance 2.0, HappyHorse (#1), PixVerse V6. Fast queue, pay-per-video.",
			DefaultBase: "https://queue.fal.run",
			Models: []string{
				"bytedance/seedance-2.0/text-to-video",
				"bytedance/seedance-2.0/image-to-video",
				"bytedance/seedance-2.0/fast/text-to-video",
				"bytedance/seedance-2.0/fast/image-to-video",
				"pixverse/pixverse-v6/text-to-video",
			},
			Fields: []CatalogField{
				{Name: "api_key", Label: "Fal.ai API Key", Type: "password", Required: true, Placeholder: "fal-..."},
				{Name: "model", Label: "Model ID", Type: "text", Required: false, Placeholder: "bytedance/seedance-2.0/text-to-video"},
			},
		},
		{
			ID: "openrouter_video", Name: "OpenRouter Video (Veo 3.1 / Seedance / Hailuo / Wan)", Kind: "video", Hosting: "cloud",
			Hint:        "One API key for all top video models. Veo 3.1 Fast, Seedance 2.0, Hailuo 2.3, Wan 2.7, Kling v3 Pro.",
			DefaultBase: "https://openrouter.ai/api/v1",
			Models: []string{
				"bytedance/seedance-2.0",
				"bytedance/seedance-2.0-fast",
				"google/veo-3.1-fast",
				"google/veo-3.1-lite",
				"minimax/hailuo-2.3",
				"alibaba/wan-2.7",
				"kwaivgi/kling-v3-pro",
			},
			Fields: []CatalogField{
				{Name: "api_key", Label: "OpenRouter API Key", Type: "password", Required: true, Placeholder: "sk-or-v1-..."},
				{Name: "model", Label: "Default Model", Type: "text", Required: false, Placeholder: "bytedance/seedance-2.0"},
			},
		},
		{
			ID: "openai_compat", Name: "OpenAI-Compatible (custom)", Kind: "image", Hosting: "cloud",
			Hint:   "Any image generation API that speaks the OpenAI /v1/images/generations format.",
			Fields: []CatalogField{
				{Name: "api_key", Label: "API Key", Type: "password", Required: false, Placeholder: "(optional)"},
				{Name: "api_base", Label: "API Base URL", Type: "text", Required: true, Placeholder: "https://your-provider.com/v1"},
				{Name: "model", Label: "Model ID", Type: "text", Required: true, Placeholder: "flux-schnell"},
			},
		},
	}
}

// ─── Runway ML video provider ────────────────────────────────────────────────

// RunwayVideoProvider uses Runway Gen-4 Turbo (and compatible Gen-3/4 models)
// via the Runway API v1. Generation is async: the initial request returns a
// task ID which must be polled until status == "SUCCEEDED".
type RunwayVideoProvider struct {
	apiKey  string
	model   string
	client  *http.Client
}

func NewRunwayVideoProvider(apiKey, model string) *RunwayVideoProvider {
	if model == "" {
		model = "gen4_turbo"
	}
	return &RunwayVideoProvider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *RunwayVideoProvider) Name() string { return "runway" }

func (p *RunwayVideoProvider) Generate(ctx context.Context, prompt string, opts VideoOptions) (*VideoResult, error) {
	model := firstNonEmpty(opts.Model, p.model)
	dur := opts.Duration
	if dur <= 0 {
		dur = 5
	}

	reqBody := map[string]any{
		"model":       model,
		"promptText":  prompt,
		"duration":    dur,
	}
	if opts.ImageURL != "" {
		reqBody["promptImage"] = []map[string]string{{"uri": opts.ImageURL, "position": "first"}}
	}
	if opts.AspectRatio != "" {
		reqBody["ratio"] = opts.AspectRatio
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.dev.runwayml.com/v1/image_to_video", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("X-Runway-Version", "2024-11-06")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runway: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("runway %d: %s", resp.StatusCode, string(data[:min(len(data), 300)]))
	}

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.ID == "" {
		return nil, fmt.Errorf("runway: unexpected response: %s", string(data[:min(len(data), 200)]))
	}

	// Poll for completion (up to 5 min).
	deadline := time.Now().Add(5 * time.Minute)
	pollURL := "https://api.dev.runwayml.com/v1/tasks/" + result.ID
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return &VideoResult{TaskID: result.ID, PollURL: pollURL}, nil
		case <-time.After(5 * time.Second):
		}
		pollReq, _ := http.NewRequestWithContext(ctx, "GET", pollURL, nil)
		pollReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		pollReq.Header.Set("X-Runway-Version", "2024-11-06")
		pollResp, err := p.client.Do(pollReq)
		if err != nil {
			continue
		}
		pollData, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()
		var task struct {
			Status string   `json:"status"`
			Output []string `json:"output"`
		}
		if json.Unmarshal(pollData, &task) == nil {
			if task.Status == "SUCCEEDED" && len(task.Output) > 0 {
				return &VideoResult{URL: task.Output[0], TaskID: result.ID, Duration: dur}, nil
			}
			if task.Status == "FAILED" {
				return nil, fmt.Errorf("runway: generation failed")
			}
		}
	}
	// Return task ID so caller can poll manually.
	return &VideoResult{TaskID: result.ID, PollURL: pollURL}, nil
}

// ─── Kling AI video provider ─────────────────────────────────────────────────

// KlingVideoProvider uses the Kling AI API (via kwaikolors.com / official endpoint).
// Like Runway it's async: submit a task, poll until done.
type KlingVideoProvider struct {
	apiKey  string
	model   string
	client  *http.Client
}

func NewKlingVideoProvider(apiKey, model string) *KlingVideoProvider {
	if model == "" {
		model = "kling-v1-5"
	}
	return &KlingVideoProvider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *KlingVideoProvider) Name() string { return "kling" }

func (p *KlingVideoProvider) Generate(ctx context.Context, prompt string, opts VideoOptions) (*VideoResult, error) {
	model := firstNonEmpty(opts.Model, p.model)
	dur := opts.Duration
	if dur <= 0 {
		dur = 5
	}
	ratio := firstNonEmpty(opts.AspectRatio, "16:9")

	reqBody := map[string]any{
		"model":        model,
		"prompt":       prompt,
		"duration":     dur,
		"aspect_ratio": ratio,
		"cfg_scale":    0.5,
		"mode":         "std",
	}
	if opts.ImageURL != "" {
		reqBody["image_url"] = opts.ImageURL
	}

	body, _ := json.Marshal(reqBody)
	baseURL := "https://api.klingai.com"
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/videos/text2video", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kling: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("kling %d: %s", resp.StatusCode, string(data[:min(len(data), 300)]))
	}

	var result struct {
		Code int `json:"code"`
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil || result.Data.TaskID == "" {
		return nil, fmt.Errorf("kling: unexpected response: %s", string(data[:min(len(data), 200)]))
	}

	taskID := result.Data.TaskID
	pollURL := baseURL + "/v1/videos/" + taskID

	// Poll for completion (up to 5 min).
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return &VideoResult{TaskID: taskID, PollURL: pollURL}, nil
		case <-time.After(5 * time.Second):
		}
		pollReq, _ := http.NewRequestWithContext(ctx, "GET", pollURL, nil)
		pollReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		pollResp, err := p.client.Do(pollReq)
		if err != nil {
			continue
		}
		pollData, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var task struct {
			Data struct {
				TaskStatus string `json:"task_status"`
				TaskResult struct {
					Videos []struct {
						URL      string `json:"url"`
						Duration string `json:"duration"`
					} `json:"videos"`
				} `json:"task_result"`
			} `json:"data"`
		}
		if json.Unmarshal(pollData, &task) == nil {
			switch task.Data.TaskStatus {
			case "succeed":
				if len(task.Data.TaskResult.Videos) > 0 {
					return &VideoResult{URL: task.Data.TaskResult.Videos[0].URL, TaskID: taskID, Duration: dur}, nil
				}
			case "failed":
				return nil, fmt.Errorf("kling: generation failed")
			}
		}
	}
	return &VideoResult{TaskID: taskID, PollURL: pollURL}, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

type settingsBag map[string]any

func parseSettings(raw json.RawMessage) settingsBag {
	if len(raw) == 0 {
		return settingsBag{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return settingsBag{}
	}
	return settingsBag(m)
}

func (b settingsBag) String(key string) string {
	if v, ok := b[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}


