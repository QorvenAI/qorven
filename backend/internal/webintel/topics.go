// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package webintel

// TopicDiscovery provides curated search for specific topics.

// TopicConfig defines search queries and trusted sources for a topic.
type TopicConfig struct {
	Queries []string
	Sources []string // trusted domains
}

// DefaultTopics returns curated topic configurations.
var DefaultTopics = map[string]TopicConfig{
	"tech": {
		Queries: []string{"technology news", "latest tech", "AI developments", "science innovation"},
		Sources: []string{"techcrunch.com", "wired.com", "theverge.com", "arstechnica.com"},
	},
	"finance": {
		Queries: []string{"finance news", "economy", "stock market", "investing trends"},
		Sources: []string{"bloomberg.com", "cnbc.com", "marketwatch.com", "ft.com"},
	},
	"science": {
		Queries: []string{"science news", "research breakthroughs", "nature journal", "scientific discoveries"},
		Sources: []string{"nature.com", "science.org", "newscientist.com", "scientificamerican.com"},
	},
	"ai": {
		Queries: []string{"artificial intelligence news", "machine learning", "LLM developments", "AI research"},
		Sources: []string{"arxiv.org", "huggingface.co", "openai.com", "anthropic.com"},
	},
	"security": {
		Queries: []string{"cybersecurity news", "data breach", "security vulnerability", "infosec"},
		Sources: []string{"krebsonsecurity.com", "bleepingcomputer.com", "thehackernews.com"},
	},
	"startup": {
		Queries: []string{"startup news", "venture capital", "funding rounds", "startup launches"},
		Sources: []string{"techcrunch.com", "crunchbase.com", "producthunt.com"},
	},
}

// QueriesForTopic returns search queries for a topic, with fallback to generic.
func QueriesForTopic(topic string) []string {
	if cfg, ok := DefaultTopics[topic]; ok {
		return cfg.Queries
	}
	return []string{topic + " news", topic + " latest", topic + " trends"}
}

// SourcesForTopic returns trusted domains for a topic.
func SourcesForTopic(topic string) []string {
	if cfg, ok := DefaultTopics[topic]; ok {
		return cfg.Sources
	}
	return nil
}
