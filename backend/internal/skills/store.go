// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package skills

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store handles skill CRUD in PostgreSQL with grants.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

func (s *Store) Create(ctx context.Context, tenantID string, name, slug, description, filePath, fileHash string, tags []string) (string, error) {
	if !SlugRegexp.MatchString(slug) {
		return "", fmt.Errorf("invalid slug: %s", slug)
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO skills (tenant_id, name, slug, description, file_path, file_hash, tags, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'active') RETURNING id`,
		tenantID, name, slug, description, filePath, fileHash, tags,
	).Scan(&id)
	return id, err
}

func (s *Store) List(ctx context.Context, tenantID string) ([]Info, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, slug, description, file_path, status FROM skills
		 WHERE tenant_id = $1 AND status = 'active' ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	skills := []Info{}
	for rows.Next() {
		var id, name, slug, desc, path string
		var status string
		rows.Scan(&id, &name, &slug, &desc, &path, &status)
		skills = append(skills, Info{Name: name, Slug: slug, Description: desc, Path: path, Source: "managed"})
	}
	return skills, nil
}

func (s *Store) Delete(ctx context.Context, tenantID, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE skills SET status = 'archived', updated_at = NOW() WHERE tenant_id = $1 AND id = $2`,
		tenantID, id)
	return err
}

// --- BM25 Search ---

// SearchResult is a scored skill from search.
type SearchResult struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
}

// Search performs BM25-style keyword search across skills.
func Search(skills []Info, query string, maxResults int) []SearchResult {
	query = strings.ToLower(query)
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return nil
	}

	type scored struct {
		info  Info
		score float64
	}
	results := []scored{}

	for _, s := range skills {
		doc := strings.ToLower(s.Name + " " + s.Description + " " + s.Slug)
		score := bm25Score(doc, terms)
		if score > 0 {
			results = append(results, scored{info: s, score: score})
		}
	}

	// Sort by score descending
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	if len(results) > maxResults {
		results = results[:maxResults]
	}

	out := make([]SearchResult, len(results))
	for i, r := range results {
		out[i] = SearchResult{Name: r.info.Name, Slug: r.info.Slug, Description: r.info.Description, Score: r.score}
	}
	return out
}

// bm25Score computes a simplified BM25 score for a document against query terms.
func bm25Score(doc string, terms []string) float64 {
	words := strings.Fields(doc)
	docLen := float64(len(words))
	avgLen := 50.0 // approximate average
	k1 := 1.2
	b := 0.75

	var score float64
	for _, term := range terms {
		tf := 0.0
		for _, w := range words {
			if strings.Contains(w, term) {
				tf++
			}
		}
		if tf == 0 {
			continue
		}
		// Simplified IDF (assume term appears in ~30% of docs)
		idf := math.Log(1 + (10-3+0.5)/(3+0.5))
		numerator := tf * (k1 + 1)
		denominator := tf + k1*(1-b+b*docLen/avgLen)
		score += idf * numerator / denominator
	}
	return score
}

// --- Marketplace Methods ---

// SkillDetail is the full skill record for marketplace.
type SkillDetail struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Author       string   `json:"author"`
	Category     string   `json:"category"`
	Tags         []string `json:"tags"`
	RequiresTools []string `json:"requires_tools"`
	RequiresKeys  []string `json:"requires_keys"`
	SourceURL    string   `json:"source_url"`
	SkillMD      string   `json:"skill_md"`
	InstallCount int      `json:"install_count"`
	Rating       float64  `json:"rating"`
}

func (s *Store) GetBySlug(ctx context.Context, slug string) (*SkillDetail, error) {
	var d SkillDetail
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, COALESCE(description,''), COALESCE(version,'1.0.0'), COALESCE(author,''), COALESCE(category,''),
		        COALESCE(tags,'{}'), COALESCE(requires_tools,'{}'), COALESCE(requires_keys,'{}'),
		        COALESCE(source_url,''), COALESCE(skill_md,''), COALESCE(install_count,0), COALESCE(rating,0)
		 FROM skills WHERE slug = $1 AND status = 'active'`, slug,
	).Scan(&d.ID, &d.Name, &d.Slug, &d.Description, &d.Version, &d.Author, &d.Category,
		&d.Tags, &d.RequiresTools, &d.RequiresKeys, &d.SourceURL, &d.SkillMD, &d.InstallCount, &d.Rating)
	if err != nil { return nil, err }
	return &d, nil
}

func (s *Store) ListMarketplace(ctx context.Context, category, search string) ([]SkillDetail, error) {
	q := `SELECT id, name, slug, COALESCE(description,''), COALESCE(version,'1.0.0'), COALESCE(author,''),
	             COALESCE(category,''), COALESCE(tags,'{}'), COALESCE(install_count,0), COALESCE(rating,0)
	      FROM skills WHERE status = 'active'`
	args := []any{}
	n := 0
	if category != "" { n++; q += fmt.Sprintf(" AND category = $%d", n); args = append(args, category) }
	if search != "" { n++; q += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d OR slug ILIKE $%d)", n, n, n); args = append(args, "%"+search+"%") }
	q += " ORDER BY install_count DESC, name LIMIT 100"

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []SkillDetail{}
	for rows.Next() {
		var d SkillDetail
		rows.Scan(&d.ID, &d.Name, &d.Slug, &d.Description, &d.Version, &d.Author, &d.Category, &d.Tags, &d.InstallCount, &d.Rating)
		out = append(out, d)
	}
	return out, nil
}

func (s *Store) Publish(ctx context.Context, d SkillDetail) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO skills (name, slug, description, version, author, category, tags, requires_tools, requires_keys, source_url, skill_md, file_path, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'',  'active')
		 ON CONFLICT (slug) DO UPDATE SET name=$1, description=$3, version=$4, author=$5, category=$6, tags=$7, requires_tools=$8, requires_keys=$9, source_url=$10, skill_md=$11, updated_at=NOW()
		 RETURNING id`,
		d.Name, d.Slug, d.Description, d.Version, d.Author, d.Category, d.Tags, d.RequiresTools, d.RequiresKeys, d.SourceURL, d.SkillMD,
	).Scan(&id)
	return id, err
}

func (s *Store) Install(ctx context.Context, agentID, slug string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO soul_skills (agent_id, skill_id) SELECT $1, id FROM skills WHERE slug = $2
		 ON CONFLICT DO NOTHING`, agentID, slug)
	if err == nil {
		s.pool.Exec(ctx, `UPDATE skills SET install_count = install_count + 1 WHERE slug = $1`, slug)
	}
	return err
}

func (s *Store) Uninstall(ctx context.Context, agentID, slug string) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM soul_skills WHERE agent_id = $1 AND skill_id = (SELECT id FROM skills WHERE slug = $2)`,
		agentID, slug)
	return err
}

func (s *Store) AgentSkills(ctx context.Context, agentID string) ([]SkillDetail, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT s.id, s.name, s.slug, COALESCE(s.description,''), COALESCE(s.version,''), COALESCE(s.category,''), COALESCE(s.skill_md,'')
		 FROM soul_skills ss JOIN skills s ON ss.skill_id = s.id WHERE ss.agent_id = $1 AND s.status = 'active'`, agentID)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []SkillDetail{}
	for rows.Next() {
		var d SkillDetail
		rows.Scan(&d.ID, &d.Name, &d.Slug, &d.Description, &d.Version, &d.Category, &d.SkillMD)
		out = append(out, d)
	}
	return out, nil
}

func (s *Store) Rate(ctx context.Context, skillSlug string, rating int, review string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO skill_reviews (skill_id, rating, review) SELECT id, $2, $3 FROM skills WHERE slug = $1`,
		skillSlug, rating, review)
	if err == nil {
		// Update average rating
		s.pool.Exec(ctx, `UPDATE skills SET rating = (SELECT AVG(rating) FROM skill_reviews WHERE skill_id = skills.id) WHERE slug = $1`, skillSlug)
	}
	return err
}

func (s *Store) GetSkillByID(ctx context.Context, skillID string) (*SkillDetail, error) {
	var sk SkillDetail
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, slug, COALESCE(description,''), COALESCE(skill_md,''), COALESCE(version,'1.0.0'), COALESCE(author,''), COALESCE(category,'')
		 FROM skills WHERE id = $1`, skillID).Scan(
		&sk.ID, &sk.Name, &sk.Slug, &sk.Description, &sk.SkillMD, &sk.Version, &sk.Author, &sk.Category)
	if err != nil { return nil, err }
	return &sk, nil
}

func (s *Store) UpdateSkill(ctx context.Context, skillID string, name, description, skillMD string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE skills SET name = COALESCE(NULLIF($1,''), name), description = COALESCE(NULLIF($2,''), description),
		 skill_md = COALESCE(NULLIF($3,''), skill_md), updated_at = NOW() WHERE id = $4`,
		name, description, skillMD, skillID)
	return err
}

func (s *Store) ToggleSkill(ctx context.Context, skillID string, enabled bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE skills SET published = $1 WHERE id = $2`, enabled, skillID)
	return err
}

func (s *Store) SearchByEmbedding(ctx context.Context, embedding []float32, limit int) ([]SkillDetail, error) {
	if limit <= 0 { limit = 10 }
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, slug, COALESCE(description,''), COALESCE(skill_md,''), COALESCE(version,'1.0.0'), COALESCE(author,''), COALESCE(category,'')
		 FROM skills WHERE embedding IS NOT NULL
		 ORDER BY embedding <=> $1 LIMIT $2`, embedding, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	results := []SkillDetail{}
	for rows.Next() {
		var sk SkillDetail
		rows.Scan(&sk.ID, &sk.Name, &sk.Slug, &sk.Description, &sk.SkillMD, &sk.Version, &sk.Author, &sk.Category)
		results = append(results, sk)
	}
	return results, nil
}

func (s *Store) BumpVersion(ctx context.Context, skillID string) (string, error) {
	var v string
	err := s.pool.QueryRow(ctx,
		`UPDATE skills SET updated_at = NOW() WHERE id = $1 RETURNING version`, skillID).Scan(&v)
	return v, err
}

// IsPinned returns true when a skill exists and has pinned=true.
func (s *Store) IsPinned(ctx context.Context, tenantID, id string) bool {
	var pinned bool
	s.pool.QueryRow(ctx, `SELECT pinned FROM skills WHERE tenant_id = $1 AND id = $2`, tenantID, id).Scan(&pinned)
	return pinned
}

// IsPinnedBySlug returns true when a skill with the given slug exists and has pinned=true.
func (s *Store) IsPinnedBySlug(ctx context.Context, tenantID, slug string) bool {
	var pinned bool
	s.pool.QueryRow(ctx, `SELECT pinned FROM skills WHERE tenant_id = $1 AND slug = $2`, tenantID, slug).Scan(&pinned)
	return pinned
}

// SetPinned sets or clears the pinned flag on a skill.
func (s *Store) SetPinned(ctx context.Context, tenantID, id string, pinned bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE skills SET pinned = $1, updated_at = NOW() WHERE tenant_id = $2 AND id = $3`,
		pinned, tenantID, id)
	return err
}
