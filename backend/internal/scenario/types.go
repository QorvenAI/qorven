// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scenario

import "time"

type Status string
const (
	StatusCreated   Status = "created"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Project struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	Name        string     `json:"name"`
	Seed        string     `json:"seed"`
	AgentCount  int        `json:"agent_count"`
	Rounds      int        `json:"rounds"`
	Status      Status     `json:"status"`
	Report      string     `json:"report,omitempty"`
	Agents      []Agent    `json:"agents,omitempty"`
	RoundsData  []Round    `json:"rounds_data,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type Agent struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Bio    string `json:"bio"`
	Stance string `json:"stance"`
	Traits string `json:"traits"`
}

type Round struct {
	Number    int       `json:"number"`
	AgentID   string    `json:"agent_id"`
	AgentName string    `json:"agent_name"`
	Content   string    `json:"content"`
	ReplyTo   string    `json:"reply_to,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
