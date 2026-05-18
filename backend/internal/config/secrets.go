// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package config

import "encoding/json"

const secretMask = "***"

// MaskedCopy returns a deep copy of the config with all secret fields masked.
// Used to avoid exposing secrets to WebSocket clients.
func (c *Config) MaskedCopy() *Config {
	data, err := json.Marshal(c)
	if err != nil {
		return defaults()
	}
	cp := defaults()
	if err := json.Unmarshal(data, cp); err != nil {
		return defaults()
	}

	maskNonEmpty(&cp.Auth.Token)
	maskNonEmpty(&cp.Auth.EncryptionKey)
	maskNonEmpty(&cp.Database.DSN)
	for i := range cp.Providers {
		maskNonEmpty(&cp.Providers[i].APIKey)
	}
	return cp
}

// StripSecrets zeros out all secret fields in the config.
// Used before saving to disk to ensure secrets never persist.
func (c *Config) StripSecrets() {
	c.Auth.Token = ""
	c.Auth.EncryptionKey = ""
	c.Database.DSN = ""
	for i := range c.Providers {
		c.Providers[i].APIKey = ""
	}
}

// StripMaskedSecrets strips only fields that still contain the mask value "***".
// Real values (user-entered via UI) are preserved.
func (c *Config) StripMaskedSecrets() {
	stripIfMasked(&c.Auth.Token)
	stripIfMasked(&c.Auth.EncryptionKey)
	stripIfMasked(&c.Database.DSN)
	for i := range c.Providers {
		stripIfMasked(&c.Providers[i].APIKey)
	}
}

func maskNonEmpty(s *string) {
	if *s != "" {
		*s = secretMask
	}
}

func stripIfMasked(s *string) {
	if *s == secretMask {
		*s = ""
	}
}
