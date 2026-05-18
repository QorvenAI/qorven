// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/providers"
)

// MemberTaskInfo holds cached task metadata for mid-loop progress nudges.
type MemberTaskInfo struct {
	Subject    string
	TaskNumber int
}

// TeamTaskForReminder represents a team task for reminders (subset of TeamTask).
type TeamTaskForReminder struct {
	ID              uuid.UUID
	TaskNumber      int
	Subject         string
	Status          string
	ProgressPercent int
	ProgressStep    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// TeamReminderStore interface for team task operations needed by reminders.
type TeamReminderStore interface {
	GetTeamForAgent(ctx context.Context, agentID uuid.UUID) (*TeamInfo, error)
	ListTasksForReminder(ctx context.Context, teamID uuid.UUID, userID string) ([]TeamTaskForReminder, error)
	GetTaskForReminder(ctx context.Context, taskID uuid.UUID) (*TeamTaskForReminder, error)
}

// TeamInfo represents basic team info for reminders.
type TeamInfo struct {
	ID          uuid.UUID
	LeadAgentID uuid.UUID
}

// InjectTeamTaskReminders adds leader pending-task reminders and
// member progress context to the message list before the first LLM call.
func InjectTeamTaskReminders(ctx context.Context, messages []providers.Message, agentID uuid.UUID, teamTaskID, userID string, teamStore TeamReminderStore) ([]providers.Message, MemberTaskInfo) {
	var info MemberTaskInfo

	if teamStore == nil || agentID == uuid.Nil {
		return messages, info
	}

	// Leader reminders
	if team, _ := teamStore.GetTeamForAgent(ctx, agentID); team != nil && team.LeadAgentID == agentID {
		if tasks, err := teamStore.ListTasksForReminder(ctx, team.ID, userID); err == nil {
			var stale []string
			var inProgress []string
			for _, t := range tasks {
				if t.Status == "pending" {
					age := time.Since(t.CreatedAt).Truncate(time.Minute)
					stale = append(stale, fmt.Sprintf("- %s: \"%s\" (pending %s)", t.ID, t.Subject, age))
				}
				if t.Status == "in_progress" {
					age := time.Since(t.UpdatedAt).Truncate(time.Minute)
					progressInfo := fmt.Sprintf("in progress %s", age)
					if t.ProgressPercent > 0 {
						if t.ProgressStep != "" {
							progressInfo = fmt.Sprintf("%d%% — %s, %s", t.ProgressPercent, t.ProgressStep, age)
						} else {
							progressInfo = fmt.Sprintf("%d%%, %s", t.ProgressPercent, age)
						}
					}
					inProgress = append(inProgress, fmt.Sprintf("- %s: \"%s\" (%s)", t.ID, t.Subject, progressInfo))
				}
			}
			var parts []string
			if len(stale) > 0 {
				parts = append(parts, fmt.Sprintf(
					"You have %d pending team task(s) awaiting dispatch:\n%s\n"+
						"These tasks will be auto-dispatched to available team members.",
					len(stale), strings.Join(stale, "\n")))
			}
			if len(inProgress) > 0 {
				parts = append(parts, fmt.Sprintf(
					"You have %d in-progress team task(s) being handled by team members:\n%s\n"+
						"Their results will arrive automatically.",
					len(inProgress), strings.Join(inProgress, "\n")))
			}
			if len(parts) > 0 && len(messages) > 0 {
				reminder := "[System] " + strings.Join(parts, "\n\n")
				userMsg := messages[len(messages)-1]
				messages[len(messages)-1] = providers.Message{
					Role:    "user",
					Content: "[Active team tasks]\n" + reminder + "\n[/Active team tasks]\n\n" + userMsg.Content,
				}
			}
		}
	}

	// Member task reminder
	if teamTaskID != "" {
		if taskUUID, err := uuid.Parse(teamTaskID); err == nil {
			if task, err := teamStore.GetTaskForReminder(ctx, taskUUID); err == nil && task != nil {
				info.Subject = task.Subject
				info.TaskNumber = task.TaskNumber
				reminder := fmt.Sprintf(
					"[System] You are working on team task #%d: %q. "+
						"Stay focused on this task. Your final response becomes the task result.",
					task.TaskNumber, task.Subject)
				if len(messages) > 0 {
					userMsg := messages[len(messages)-1]
					messages[len(messages)-1] = providers.Message{
						Role:    "user",
						Content: "[Task context]\n" + reminder + "\n[/Task context]\n\n" + userMsg.Content,
					}
				}
			}
		}
	}

	return messages, info
}
