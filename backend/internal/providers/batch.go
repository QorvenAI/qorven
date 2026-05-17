// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// BatchRequest represents a single request in a batch.
type BatchRequest struct {
	CustomID string       `json:"custom_id"`
	Method   string       `json:"method"`
	URL      string       `json:"url"`
	Body     ChatRequest  `json:"body"`
}

// BatchJob tracks a submitted batch.
type BatchJob struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"` // validating, in_progress, completed, failed, expired, cancelled
	InputCount  int       `json:"request_counts_total,omitempty"`
	OutputFile  string    `json:"output_file_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// BatchResult is one result from a completed batch.
type BatchResult struct {
	CustomID string       `json:"custom_id"`
	Response ChatResponse `json:"response"`
	Error    string       `json:"error,omitempty"`
}

// BatchClient submits batch requests to OpenAI-compatible APIs.
// Batch API gives 50% cost reduction with 24-hour turnaround.
// Use for: research tasks, bulk analysis, non-urgent processing.
type BatchClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewBatchClient(baseURL, apiKey string) *BatchClient {
	return &BatchClient{baseURL: baseURL, apiKey: apiKey, client: &http.Client{Timeout: 30 * time.Second}}
}

// Submit creates a batch job from multiple chat requests.
// Returns the batch job ID for polling.
func (b *BatchClient) Submit(ctx context.Context, requests []BatchRequest) (*BatchJob, error) {
	// Step 1: Create JSONL file
	var jsonl bytes.Buffer
	for _, req := range requests {
		line := map[string]any{
			"custom_id": req.CustomID,
			"method":    "POST",
			"url":       "/v1/chat/completions",
			"body": map[string]any{
				"model":      req.Body.Model,
				"messages":   req.Body.Messages,
				"max_tokens": req.Body.Options["max_tokens"],
			},
		}
		b, _ := json.Marshal(line)
		jsonl.Write(b)
		jsonl.WriteByte('\n')
	}

	// Step 2: Upload file
	fileID, err := b.uploadFile(ctx, jsonl.Bytes())
	if err != nil {
		return nil, fmt.Errorf("batch file upload: %w", err)
	}

	// Step 3: Create batch
	body, _ := json.Marshal(map[string]any{
		"input_file_id":    fileID,
		"endpoint":         "/v1/chat/completions",
		"completion_window": "24h",
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", b.baseURL+"/v1/batches", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+b.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var job BatchJob
	json.Unmarshal(data, &job)
	slog.Info("batch.submitted", "id", job.ID, "requests", len(requests), "status", job.Status)
	return &job, nil
}

// Status checks the status of a batch job.
func (b *BatchClient) Status(ctx context.Context, batchID string) (*BatchJob, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", b.baseURL+"/v1/batches/"+batchID, nil)
	req.Header.Set("Authorization", "Bearer "+b.apiKey)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var job BatchJob
	json.Unmarshal(data, &job)
	return &job, nil
}

// Results retrieves completed batch results.
func (b *BatchClient) Results(ctx context.Context, outputFileID string) ([]BatchResult, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", b.baseURL+"/v1/files/"+outputFileID+"/content", nil)
	req.Header.Set("Authorization", "Bearer "+b.apiKey)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	results := []BatchResult{}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var r struct {
			CustomID string `json:"custom_id"`
			Response struct {
				Body struct {
					Choices []struct {
						Message struct {
							Content string `json:"content"`
						} `json:"message"`
					} `json:"choices"`
				} `json:"body"`
			} `json:"response"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.Unmarshal(line, &r)
		br := BatchResult{CustomID: r.CustomID}
		if r.Error != nil {
			br.Error = r.Error.Message
		} else if len(r.Response.Body.Choices) > 0 {
			br.Response = ChatResponse{Content: r.Response.Body.Choices[0].Message.Content}
		}
		results = append(results, br)
	}
	return results, nil
}

func (b *BatchClient) uploadFile(ctx context.Context, jsonl []byte) (string, error) {
	// Multipart upload
	var buf bytes.Buffer
	boundary := "----BatchBoundary"
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Disposition: form-data; name=\"purpose\"\r\n\r\nbatch\r\n")
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Disposition: form-data; name=\"file\"; filename=\"batch.jsonl\"\r\n")
	buf.WriteString("Content-Type: application/jsonl\r\n\r\n")
	buf.Write(jsonl)
	buf.WriteString("\r\n--" + boundary + "--\r\n")

	req, _ := http.NewRequestWithContext(ctx, "POST", b.baseURL+"/v1/files", &buf)
	req.Header.Set("Authorization", "Bearer "+b.apiKey)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err := b.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var file struct {
		ID string `json:"id"`
	}
	json.Unmarshal(data, &file)
	return file.ID, nil
}

// ShouldBatch returns true if a task is suitable for batch processing.
// Batch is good for: research, analysis, bulk content, non-urgent work.
func ShouldBatch(taskType string, urgent bool) bool {
	if urgent {
		return false
	}
	switch taskType {
	case "research", "analysis", "bulk_content", "report", "summarize", "translate":
		return true
	}
	return false
}
