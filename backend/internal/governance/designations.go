// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Designation struct {
	ID                  string    `json:"id"`
	TenantID            string    `json:"tenant_id"`
	PositionName        string    `json:"position_name"`
	Department          string    `json:"department"`
	OrgLayer            int       `json:"org_layer"`
	NatureOfWork        string    `json:"nature_of_work"`
	ReportsTo           string    `json:"reports_to_designation,omitempty"`
	SkillFamily         string    `json:"skill_family"`
	ModelTier           string    `json:"model_tier"`
	ToolsAllowed        []string  `json:"tools_allowed"`
	ToolsDenied         []string  `json:"tools_denied"`
	MaxBudgetUSD        float64   `json:"max_budget_usd"`
	CanCreateSubagents  bool      `json:"can_create_subagents"`
	CanApproveActions   bool      `json:"can_approve_actions"`
	RequiresApproval    bool      `json:"requires_approval"`
	UserCreatable       bool      `json:"user_creatable"`
	KnowledgePacks      []string  `json:"knowledge_packs"`
	ApprovalScope       []string  `json:"approval_scope"`
	CreatedAt           time.Time `json:"created_at"`
}

type SkillFamily struct {
	ID               string   `json:"id"`
	TenantID         string   `json:"tenant_id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Capabilities     []string `json:"capabilities"`
	ModelSuggestions  []string `json:"model_suggestions"`
	ToolPermissions  []string `json:"tool_permissions"`
}

type DesignationStore struct {
	db *pgxpool.Pool
}

func NewDesignationStore(db *pgxpool.Pool) *DesignationStore {
	return &DesignationStore{db: db}
}

func (s *DesignationStore) List(ctx context.Context, tenantID string) ([]Designation, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, tenant_id, position_name, department, org_layer, COALESCE(nature_of_work,''),
		       COALESCE(reports_to_designation::text,''), COALESCE(skill_family,''), COALESCE(model_tier,'balanced'),
		       COALESCE(tools_allowed,'{}'), COALESCE(tools_denied,'{}'), COALESCE(max_budget_usd,0),
		       can_create_subagents, can_approve_actions, requires_approval, user_creatable,
		       COALESCE(knowledge_packs,'{}'), COALESCE(approval_scope,'{}'), created_at
		FROM designations WHERE tenant_id = $1
		ORDER BY org_layer ASC, department, position_name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Designation
	for rows.Next() {
		var d Designation
		if err := rows.Scan(&d.ID, &d.TenantID, &d.PositionName, &d.Department, &d.OrgLayer, &d.NatureOfWork,
			&d.ReportsTo, &d.SkillFamily, &d.ModelTier, &d.ToolsAllowed, &d.ToolsDenied, &d.MaxBudgetUSD,
			&d.CanCreateSubagents, &d.CanApproveActions, &d.RequiresApproval, &d.UserCreatable,
			&d.KnowledgePacks, &d.ApprovalScope, &d.CreatedAt); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *DesignationStore) Get(ctx context.Context, tenantID, id string) (*Designation, error) {
	var d Designation
	err := s.db.QueryRow(ctx, `
		SELECT id, tenant_id, position_name, department, org_layer, COALESCE(nature_of_work,''),
		       COALESCE(reports_to_designation::text,''), COALESCE(skill_family,''), COALESCE(model_tier,'balanced'),
		       COALESCE(tools_allowed,'{}'), COALESCE(tools_denied,'{}'), COALESCE(max_budget_usd,0),
		       can_create_subagents, can_approve_actions, requires_approval, user_creatable,
		       COALESCE(knowledge_packs,'{}'), COALESCE(approval_scope,'{}'), created_at
		FROM designations WHERE tenant_id = $1 AND id = $2
	`, tenantID, id).Scan(&d.ID, &d.TenantID, &d.PositionName, &d.Department, &d.OrgLayer, &d.NatureOfWork,
		&d.ReportsTo, &d.SkillFamily, &d.ModelTier, &d.ToolsAllowed, &d.ToolsDenied, &d.MaxBudgetUSD,
		&d.CanCreateSubagents, &d.CanApproveActions, &d.RequiresApproval, &d.UserCreatable,
		&d.KnowledgePacks, &d.ApprovalScope, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *DesignationStore) Upsert(ctx context.Context, d Designation) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO designations (tenant_id, position_name, department, org_layer, nature_of_work,
		    reports_to_designation, skill_family, model_tier, tools_allowed, tools_denied, max_budget_usd,
		    can_create_subagents, can_approve_actions, requires_approval, user_creatable, knowledge_packs, approval_scope)
		VALUES ($1,$2,$3,$4,$5, NULLIF($6,'')::uuid, $7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (tenant_id, position_name) DO UPDATE SET
		    department=EXCLUDED.department, org_layer=EXCLUDED.org_layer, nature_of_work=EXCLUDED.nature_of_work,
		    reports_to_designation=EXCLUDED.reports_to_designation, skill_family=EXCLUDED.skill_family,
		    model_tier=EXCLUDED.model_tier, tools_allowed=EXCLUDED.tools_allowed, tools_denied=EXCLUDED.tools_denied,
		    max_budget_usd=EXCLUDED.max_budget_usd, can_create_subagents=EXCLUDED.can_create_subagents,
		    can_approve_actions=EXCLUDED.can_approve_actions, requires_approval=EXCLUDED.requires_approval,
		    user_creatable=EXCLUDED.user_creatable, knowledge_packs=EXCLUDED.knowledge_packs,
		    approval_scope=EXCLUDED.approval_scope, updated_at=now()
	`, d.TenantID, d.PositionName, d.Department, d.OrgLayer, d.NatureOfWork,
		d.ReportsTo, d.SkillFamily, d.ModelTier, d.ToolsAllowed, d.ToolsDenied, d.MaxBudgetUSD,
		d.CanCreateSubagents, d.CanApproveActions, d.RequiresApproval, d.UserCreatable, d.KnowledgePacks, d.ApprovalScope)
	return err
}

func (s *DesignationStore) Delete(ctx context.Context, tenantID, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM designations WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	return err
}

func (s *DesignationStore) ListSkillFamilies(ctx context.Context, tenantID string) ([]SkillFamily, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), COALESCE(capabilities,'{}'),
		       COALESCE(model_suggestions,'{}'), COALESCE(tool_permissions,'{}')
		FROM skill_families WHERE tenant_id = $1 ORDER BY name
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SkillFamily
	for rows.Next() {
		var sf SkillFamily
		if err := rows.Scan(&sf.ID, &sf.TenantID, &sf.Name, &sf.Description, &sf.Capabilities, &sf.ModelSuggestions, &sf.ToolPermissions); err != nil {
			continue
		}
		out = append(out, sf)
	}
	return out, nil
}
