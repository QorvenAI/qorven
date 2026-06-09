// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"os"
	"time"

	"github.com/qorvenai/qorven/internal/presence"
	"github.com/qorvenai/qorven/internal/reachuser"
)

// reachDeliverer implements reachuser.Deliverer using the gateway's existing
// notification, realtime, channel, presence and SMTP infrastructure.
type reachDeliverer struct {
	gw       *Gateway
	presence *presence.Store
}

func (d *reachDeliverer) IsOnline(ctx context.Context, userID string) bool {
	if d.presence == nil {
		return false
	}
	p, err := d.presence.Get(ctx, userID)
	if err != nil || p == nil {
		return false
	}
	return p.IsOnline
}

func (d *reachDeliverer) Deliver(ctx context.Context, e reachuser.Escalation, rung int, channel string) error {
	switch channel {
	case reachuser.ChannelInApp:
		return d.deliverInApp(ctx, e)
	case reachuser.ChannelIM:
		return d.deliverIM(ctx, e)
	case reachuser.ChannelEmail:
		return d.deliverEmail(ctx, e)
	default:
		return fmt.Errorf("unknown channel %q", channel)
	}
}

func (d *reachDeliverer) deliverInApp(ctx context.Context, e reachuser.Escalation) error {
	if d.gw.notifStore != nil {
		d.gw.notifStore.Create(ctx, "", "", "", e.Kind, e.Title, e.Body, e.Kind, e.RefID)
	}
	if d.gw.rtHub != nil {
		d.gw.rtHub.BroadcastNotification(e.Title, e.Body)
	}
	return nil
}

func (d *reachDeliverer) deliverIM(ctx context.Context, e reachuser.Escalation) error {
	if d.presence == nil || d.gw.chanMgr == nil {
		return fmt.Errorf("im delivery unavailable")
	}
	channel, chatID, err := d.presence.LastChannelAndChatID(ctx, e.UserID)
	if err != nil || chatID == "" || channel == "web" {
		return fmt.Errorf("no IM reachability for user")
	}
	msg := e.Title
	if e.Body != "" {
		msg += "\n\n" + e.Body
	}
	return d.gw.chanMgr.SendToChannel(ctx, channel, chatID, msg)
}

func (d *reachDeliverer) deliverEmail(ctx context.Context, e reachuser.Escalation) error {
	host, port, user, pass, from := d.gw.smtpSettings()
	if host == "" {
		return fmt.Errorf("smtp not configured")
	}
	var to string
	if d.gw.db != nil {
		d.gw.db.Pool.QueryRow(ctx, `SELECT COALESCE(email,'') FROM users WHERE id=$1`, e.UserID).Scan(&to)
	}
	if to == "" {
		return fmt.Errorf("no email for user")
	}
	body := fmt.Sprintf("Subject: %s\r\nFrom: %s\r\nTo: %s\r\n\r\n%s\r\n", e.Title, from, to, e.Body)
	addr := fmt.Sprintf("%s:%d", host, port)
	auth := smtp.PlainAuth("", user, pass, host)
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(body))
}

// smtpSettings returns SMTP host/port/user/pass/from from config or env.
func (gw *Gateway) smtpSettings() (host string, port int, user, pass, from string) {
	host = gw.cfg.Email.SMTPHost
	if host == "" {
		host = os.Getenv("SMTP_HOST")
	}
	port = gw.cfg.Email.SMTPPort
	if port == 0 {
		port = 465
	}
	user = gw.cfg.Email.SMTPUser
	if user == "" {
		user = os.Getenv("SMTP_USER")
	}
	pass = gw.cfg.Email.SMTPPass
	if pass == "" {
		pass = os.Getenv("SMTP_PASS")
	}
	from = gw.cfg.Email.From
	if from == "" {
		from = os.Getenv("SMTP_FROM")
	}
	return
}

// ReachUser opens an escalation to reach the human. Non-blocking: rung 1 is
// delivered synchronously, the climb is handled by the ticker.
func (gw *Gateway) ReachUser(ctx context.Context, userID, kind, refID, title, body, urgency string) (string, error) {
	if gw.reach == nil {
		return "", fmt.Errorf("reach-user engine not available")
	}
	return gw.reach.Open(ctx, reachuser.Escalation{
		TenantID: defaultTenant, UserID: userID, Kind: kind, RefID: refID,
		Title: title, Body: body, Urgency: urgency,
	})
}

// startReachUserTicker scans for due escalations every 30s and advances them.
func (gw *Gateway) startReachUserTicker() {
	if gw.reach == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				if err := gw.reach.Tick(ctx); err != nil {
					slog.Warn("reachuser.tick.failed", "err", err)
				}
			}()
		}
	}()
}
