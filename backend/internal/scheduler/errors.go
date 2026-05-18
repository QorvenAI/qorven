// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scheduler

import "errors"

var (
	ErrQueueFull       = errors.New("session queue is full")
	ErrQueueDropped    = errors.New("message dropped from queue")
	ErrMessageStale    = errors.New("message stale: enqueued before abort")
	ErrGatewayDraining = errors.New("gateway is shutting down, please retry shortly")
	ErrLaneCleared     = errors.New("session queue cleared during reset")
)
