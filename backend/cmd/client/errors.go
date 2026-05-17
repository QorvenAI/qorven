// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package client

import "fmt"

// APIError represents a structured error from the Qorven gateway.
type APIError struct {
	StatusCode int    `json:"status_code,omitempty"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("[%d] %s: %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Sentinel errors.
var ErrNotAuthenticated = &APIError{Code: "not_authenticated", Message: "not authenticated - run 'qorven auth login' or set QORVEN_TOKEN"}
var ErrServerRequired = &APIError{Code: "server_required", Message: "server URL required - use --server or QORVEN_SERVER"}
