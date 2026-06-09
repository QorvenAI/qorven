// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Package budgets owns the budget hierarchy: departments, projects, and the
// per-scope caps with carved-vs-fresh allocation semantics.
package budgets

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrOverAllocated is returned when a carved allocation would exceed the
// parent pool's available budget.
var ErrOverAllocated = errors.New("allocation exceeds the parent budget's available pool")

// validateAllocation enforces the carved-vs-fresh rule. For "fresh" the child
// is additive (no draw-down). For "carved" the sum of carved children must not
// exceed the parent cap; a parent cap of 0 means unlimited.
func validateAllocation(mode string, parentCapUSD, existingCarvedUSD, newCapUSD float64) error {
	if mode == "fresh" {
		return nil
	}
	return validateCarved(parentCapUSD, existingCarvedUSD, newCapUSD)
}

// validateCarved checks a carved child against the parent's available pool.
func validateCarved(parentCapUSD, existingCarvedUSD, newCapUSD float64) error {
	if parentCapUSD <= 0 {
		return nil // unlimited parent
	}
	if existingCarvedUSD+newCapUSD > parentCapUSD {
		return fmt.Errorf("%w: parent cap $%.2f, already allocated $%.2f, requested $%.2f",
			ErrOverAllocated, parentCapUSD, existingCarvedUSD, newCapUSD)
	}
	return nil
}

// Store is the budget-hierarchy data access layer.
type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

// Department / Project DTOs.
type Department struct {
	ID                 string `json:"id"`
	TenantID           string `json:"tenant_id"`
	Name               string `json:"name"`
	HeadAgentID        string `json:"head_agent_id,omitempty"`
	ParentDepartmentID string `json:"parent_department_id,omitempty"`
}
type Project struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Name         string `json:"name"`
	DepartmentID string `json:"department_id,omitempty"`
	Status       string `json:"status"`
}

// CreateDepartment inserts a department and returns its id.
func (s *Store) CreateDepartment(ctx context.Context, tenantID, name, headAgentID string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx,
		`INSERT INTO departments (tenant_id, name, head_agent_id)
		 VALUES ($1, $2, NULLIF($3,'')::uuid) RETURNING id::text`,
		tenantID, name, headAgentID).Scan(&id)
	return id, err
}

// ListDepartments returns all departments for a tenant.
func (s *Store) ListDepartments(ctx context.Context, tenantID string) ([]Department, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id::text, tenant_id::text, name, COALESCE(head_agent_id::text,''), COALESCE(parent_department_id::text,'')
		 FROM departments WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Department
	for rows.Next() {
		var d Department
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Name, &d.HeadAgentID, &d.ParentDepartmentID); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CreateProject inserts a project under an optional department.
func (s *Store) CreateProject(ctx context.Context, tenantID, name, departmentID string) (string, error) {
	var id string
	err := s.db.QueryRow(ctx,
		`INSERT INTO projects (tenant_id, name, department_id)
		 VALUES ($1, $2, NULLIF($3,'')::uuid) RETURNING id::text`,
		tenantID, name, departmentID).Scan(&id)
	return id, err
}

// ListProjects returns all projects for a tenant.
func (s *Store) ListProjects(ctx context.Context, tenantID string) ([]Project, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id::text, tenant_id::text, name, COALESCE(department_id::text,''), status
		 FROM projects WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.DepartmentID, &p.Status); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
