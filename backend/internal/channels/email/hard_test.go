// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package email

import "testing"

func TestHard_Email_Config(t *testing.T) {
	cfg := Config{AgentID: "a1", SMTPHost: "smtp.gmail.com", SMTPPort: 465, IMAPHost: "imap.gmail.com", IMAPPort: 993}
	if cfg.SMTPHost != "smtp.gmail.com" { t.Error("smtp") }
	if cfg.IMAPHost != "imap.gmail.com" { t.Error("imap") }
	if cfg.SMTPPort != 465 { t.Error("smtp port") }
}
