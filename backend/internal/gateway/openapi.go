// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import "net/http"

// HandleOpenAPI serves a basic OpenAPI 3.0 spec for the Qorven API.
func HandleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(openAPISpec))
}

const openAPISpec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "Qorven API",
    "description": "Multi-agent AI platform API — agents, chat, memory, tools, workflows",
    "version": "2.0.0",
    "contact": { "name": "Qorven", "url": "https://qorven.ai" }
  },
  "servers": [{ "url": "/v1", "description": "API v1" }],
  "paths": {
    "/health": { "get": { "summary": "Health check", "responses": { "200": { "description": "OK" } } } },
    "/metrics": { "get": { "summary": "Prometheus metrics", "responses": { "200": { "description": "Metrics" } } } },
    "/chat/completions": { "post": { "summary": "Chat with agent", "tags": ["Chat"], "requestBody": { "content": { "application/json": { "schema": { "$ref": "#/components/schemas/ChatRequest" } } } }, "responses": { "200": { "description": "Chat response" } } } },
    "/agents": { "get": { "summary": "List agents", "tags": ["Agents"], "responses": { "200": { "description": "Agent list" } } }, "post": { "summary": "Create agent", "tags": ["Agents"], "responses": { "201": { "description": "Created" } } } },
    "/agents/{id}": { "get": { "summary": "Get agent", "tags": ["Agents"] }, "put": { "summary": "Update agent", "tags": ["Agents"] }, "delete": { "summary": "Delete agent", "tags": ["Agents"] } },
    "/sessions": { "get": { "summary": "List sessions", "tags": ["Sessions"] }, "post": { "summary": "Create session", "tags": ["Sessions"] } },
    "/memory/company": { "get": { "summary": "Get company memories", "tags": ["Memory"] }, "post": { "summary": "Save company memory", "tags": ["Memory"] } },
    "/workflows": { "get": { "summary": "List workflows", "tags": ["Workflows"] }, "post": { "summary": "Create workflow", "tags": ["Workflows"] } },
    "/workflows/{id}/run": { "post": { "summary": "Run workflow", "tags": ["Workflows"] } },
    "/outbound/pending": { "get": { "summary": "List pending outbound approvals", "tags": ["Approvals"] } },
    "/outbound/{id}/approve": { "post": { "summary": "Approve outbound action", "tags": ["Approvals"] } },
    "/outbound/{id}/reject": { "post": { "summary": "Reject outbound action", "tags": ["Approvals"] } },
    "/research/start": { "post": { "summary": "Start research job", "tags": ["Research"] } },
    "/research/{id}": { "get": { "summary": "Get research status", "tags": ["Research"] } },
    "/supervisor/status": { "get": { "summary": "Supervisor status", "tags": ["Supervisor"] } },
    "/council": { "post": { "summary": "Run LLM council", "tags": ["Council"] } },
    "/tools/builtin": { "get": { "summary": "List builtin tools", "tags": ["Tools"] } },
    "/tools/metrics": { "get": { "summary": "Tool execution metrics", "tags": ["Tools"] } },
    "/billing/costs": { "get": { "summary": "Billing costs", "tags": ["Billing"] } },
    "/audit": { "get": { "summary": "Audit log", "tags": ["Audit"] } },
    "/mail/identities": { "get": { "summary": "List mail identities", "tags": ["Mail"] } },
    "/calendar/events": { "get": { "summary": "List events", "tags": ["Calendar"] }, "post": { "summary": "Create event", "tags": ["Calendar"] } }
  },
  "components": {
    "schemas": {
      "ChatRequest": { "type": "object", "properties": { "agent_id": { "type": "string" }, "session_id": { "type": "string" }, "message": { "type": "string" }, "stream": { "type": "boolean" }, "depth": { "type": "string", "enum": ["quick", "balanced", "deep", "max"] } }, "required": ["agent_id", "message"] }
    },
    "securitySchemes": { "bearerAuth": { "type": "http", "scheme": "bearer" } }
  },
  "security": [{ "bearerAuth": [] }]
}`
