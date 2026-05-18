// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/scheduler"
)

// makeSchedulerRunFunc creates the RunFunc that bridges the scheduler to the agent loop.
// Extracts agentID from the RunRequest and routes to the correct agent.
func (gw *Gateway) makeSchedulerRunFunc() scheduler.RunFunc {
	return func(ctx context.Context, req agent.RunRequest) (*agent.RunResult, error) {
		if gw.agentLoop == nil {
			return nil, fmt.Errorf("agent loop not initialized")
		}
		if req.AgentID == "" {
			return nil, fmt.Errorf("no agent ID in run request")
		}
		result, err := gw.agentLoop.Run(ctx, req, nil)
		return result, err
	}
}
