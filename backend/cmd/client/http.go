// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HTTPClient wraps net/http.Client with auth, retries, and error handling.
type HTTPClient struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	Verbose    bool
}

// NewHTTPClient creates a client for the given gateway URL.
func NewHTTPClient(baseURL, token string, insecure bool) *HTTPClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &HTTPClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// Get performs an HTTP GET.
func (c *HTTPClient) Get(path string) (json.RawMessage, error) {
	return c.do("GET", path, nil)
}

// Post performs an HTTP POST with a JSON body.
func (c *HTTPClient) Post(path string, body any) (json.RawMessage, error) {
	return c.do("POST", path, body)
}

// Put performs an HTTP PUT with a JSON body.
func (c *HTTPClient) Put(path string, body any) (json.RawMessage, error) {
	return c.do("PUT", path, body)
}

// Delete performs an HTTP DELETE.
func (c *HTTPClient) Delete(path string) (json.RawMessage, error) {
	return c.do("DELETE", path, nil)
}

// DeleteWithBody performs an HTTP DELETE with a JSON body.
func (c *HTTPClient) DeleteWithBody(path string, body any) (json.RawMessage, error) {
	return c.do("DELETE", path, body)
}

// UploadFile performs a multipart POST to upload a file.
func (c *HTTPClient) UploadFile(path, filePath, agentID string) (json.RawMessage, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if agentID != "" {
		w.WriteField("agent_id", agentID)
	}
	part, err := w.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(part, f); err != nil {
		return nil, err
	}
	w.Close()

	req, err := http.NewRequest("POST", c.BaseURL+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// GetRaw returns the raw *http.Response (for streaming, file downloads).
func (c *HTTPClient) GetRaw(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return c.HTTPClient.Do(req)
}

// PostRaw returns the raw *http.Response (for streaming SSE).
func (c *HTTPClient) PostRaw(path string, body any) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequest("POST", c.BaseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return c.HTTPClient.Do(req)
}

// HealthCheck performs GET /health.
func (c *HTTPClient) HealthCheck() error {
	req, err := http.NewRequest("GET", c.BaseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach gateway: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *HTTPClient) do(method, path string, body any) (json.RawMessage, error) {
	url := c.BaseURL + path

	var bodyData []byte
	if body != nil {
		var err error
		bodyData, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
	}

	// Retry on 429 / 5xx (up to 3 attempts with exponential backoff)
	var resp *http.Response
	var err error
	for attempt := range 3 {
		var reqBody io.Reader
		if bodyData != nil {
			reqBody = bytes.NewReader(bodyData)
		}
		req, reqErr := http.NewRequest(method, url, reqBody)
		if reqErr != nil {
			return nil, reqErr
		}
		if bodyData != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}

		resp, err = c.HTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		if resp.StatusCode != 429 && resp.StatusCode < 500 {
			break
		}
		resp.Body.Close()
		if attempt < 2 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Handle error responses
	if resp.StatusCode >= 400 {
		// Try to parse structured error
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respData, &errResp) == nil && errResp.Error != "" {
			return nil, &APIError{
				StatusCode: resp.StatusCode,
				Code:       http.StatusText(resp.StatusCode),
				Message:    errResp.Error,
			}
		}
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Code:       http.StatusText(resp.StatusCode),
			Message:    string(respData),
		}
	}

	return respData, nil
}

// PostJSON sends a POST request with JSON body and returns the response as a map.
func PostJSON(url string, body map[string]string) (map[string]string, error) {
	data, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("login failed: %s", result["error"])
	}
	return result, nil
}
