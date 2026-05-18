// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package reach

import "time"

type ChannelStatus string

const (
	StatusReady      ChannelStatus = "ready"
	StatusNeedsSetup ChannelStatus = "needs_setup"
	StatusError      ChannelStatus = "error"
)

type Channel interface {
	Name() string
	Check() ChannelStatus
	Search(query string, limit int) ([]ReadResult, error)
	Read(id string) (*ReadResult, error)
}

type ReadResult struct {
	Platform    string         `json:"platform"`
	Title       string         `json:"title"`
	Content     string         `json:"content"`
	URL         string         `json:"url"`
	Author      string         `json:"author"`
	PublishedAt time.Time      `json:"published_at"`
	Engagement  map[string]int `json:"engagement,omitempty"`
}

type Config struct {
	Cookies  map[string]map[string]string `json:"cookies,omitempty"`
	APIKeys  map[string]string            `json:"api_keys,omitempty"`
	ProxyURL string                       `json:"proxy_url,omitempty"`
}

type DoctorReport struct {
	Channels []ChannelReport `json:"channels"`
}

type ChannelReport struct {
	Name   string        `json:"name"`
	Status ChannelStatus `json:"status"`
	Error  string        `json:"error,omitempty"`
}
