// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package reach

import "fmt"

type Registry struct {
	channels map[string]Channel
}

func NewRegistry() *Registry { return &Registry{channels: make(map[string]Channel)} }
func (r *Registry) Register(ch Channel) { r.channels[ch.Name()] = ch }
func (r *Registry) Get(name string) (Channel, bool) { ch, ok := r.channels[name]; return ch, ok }
func (r *Registry) List() []string { var out []string; for k := range r.channels { out = append(out, k) }; return out }

func DefaultRegistry(cfg Config) *Registry {
	r := NewRegistry()
	r.Register(&WebChannel{})
	r.Register(&RedditChannel{})
	r.Register(&YouTubeChannel{})
	r.Register(&GitHubChannel{Token: cfg.APIKeys["github"]})
	r.Register(&HNChannel{})
	r.Register(&V2EXChannel{})
	r.Register(&WeiboChannel{})
	r.Register(&RSSChannel{})
	r.Register(&ExaChannel{APIKey: cfg.APIKeys["exa"]})
	r.Register(&LinkedInChannel{})
	r.Register(&BilibiliChannel{})
	// Cookie-auth channels
	if cookies, ok := cfg.Cookies["twitter"]; ok {
		r.Register(&TwitterChannel{AuthToken: cookies["auth_token"], CT0: cookies["ct0"]})
	}
	if cookies, ok := cfg.Cookies["xiaohongshu"]; ok {
		r.Register(&XiaoHongShuChannel{Cookie: cookies["cookie"]})
	}
	if cookies, ok := cfg.Cookies["xueqiu"]; ok {
		r.Register(&XueqiuChannel{Cookie: cookies["cookie"]})
	}
	r.Register(&DouyinChannel{})
	r.Register(&WeChatChannel{})
	r.Register(&PodcastChannel{})
	return r
}

func (r *Registry) Doctor() DoctorReport {
	var report DoctorReport
	for name, ch := range r.channels {
		status := ch.Check()
		cr := ChannelReport{Name: name, Status: status}
		if status == StatusError { cr.Error = fmt.Sprintf("%s is not available", name) }
		report.Channels = append(report.Channels, cr)
	}
	return report
}

func (r *Registry) SearchAll(query string, limit int) map[string][]ReadResult {
	results := make(map[string][]ReadResult)
	for name, ch := range r.channels {
		if ch.Check() != StatusReady { continue }
		items, err := ch.Search(query, limit)
		if err != nil || len(items) == 0 { continue }
		results[name] = items
	}
	return results
}
