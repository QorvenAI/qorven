// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build spike

// Confirmed whatsmeow API for version v0.0.0-20260611094716-089932318bc2:
//
//   sqlstore.New(ctx context.Context, dialect, address string, log waLog.Logger) (*Container, error)
//     — dialect must be "pgx" (the registered driver name from jackc/pgx/v5/stdlib).
//     — dbutil.ParseDialect accepts "pgx" as equivalent to "postgres".
//     — IMPORTANT: sqlstore.New calls Upgrade() internally; do NOT call it again.
//     — Caller must blank-import _ "github.com/jackc/pgx/v5/stdlib" to register driver.
//
//   container.Upgrade(ctx context.Context) error
//     — Creates/migrates the whatsmeow_* tables (auto-called by New).
//
//   container.GetFirstDevice(ctx context.Context) (*store.Device, error)
//     — Returns the first stored device, or creates a blank one if none exists.
//
//   container.Close() error
//
//   whatsmeow.NewClient(deviceStore *store.Device, log waLog.Logger) *Client
//
//   cli.Connect() error                              (no ctx)
//   cli.ConnectContext(ctx context.Context) error   (ctx variant)
//   cli.Disconnect()
//   cli.IsConnected() bool
//   cli.AddEventHandler(handler EventHandler) uint32  (EventHandler = func(evt any))
//   cli.GetQRChannel(ctx context.Context) (<-chan QRChannelItem, error)
//   cli.SendMessage(ctx context.Context, to types.JID, message *waE2E.Message, extra ...SendRequestExtra) (SendResponse, error)
//
//   Proto import path: go.mau.fi/whatsmeow/proto/waE2E
//   Text message:      &waE2E.Message{Conversation: proto.String("hello")}
//
//   JID helpers (package go.mau.fi/whatsmeow/types):
//     types.ParseJID(jid string) (JID, error)
//     types.NewJID(user, server string) JID
//     types.DefaultUserServer = "s.whatsapp.net"
//     jid.ToNonAD() JID   — strips device/agent, returns plain user@server JID
//
//   events.Message fields (evt.Info is types.MessageInfo, embeds MessageSource):
//     evt.Info.ID                         (types.MessageID = string)
//     evt.Info.Sender                     (types.JID)
//     evt.Info.Chat                       (types.JID)
//     evt.Info.IsFromMe                   (bool)   — from embedded MessageSource
//     evt.Info.PushName                   (string)
//     evt.Message                         (*waE2E.Message)
//     evt.Message.GetConversation()       string   — plain-text body
//     evt.Message.GetExtendedTextMessage().GetText()  string — rich-text body

package whatsapp

import (
	"context"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver with database/sql
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestWhatsmeowSpike(t *testing.T) {
	dsn := os.Getenv("QORVEN_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set QORVEN_POSTGRES_DSN to run the spike")
	}

	ctx := context.Background()

	// sqlstore.New opens the DB and calls Upgrade() automatically.
	// dialect = "pgx" matches the driver registered by jackc/pgx/v5/stdlib.
	container, err := sqlstore.New(ctx, "pgx", dsn, waLog.Noop)
	if err != nil {
		t.Fatalf("sqlstore.New: %v", err)
	}
	defer func() {
		if err := container.Close(); err != nil {
			t.Logf("container.Close: %v", err)
		}
	}()

	// Upgrade was already called by sqlstore.New. Calling it again is safe (idempotent).
	// We call it explicitly here to confirm the method works as documented.
	if err := container.Upgrade(ctx); err != nil {
		t.Fatalf("container.Upgrade (idempotent re-run): %v", err)
	}
	t.Log("container.Upgrade: OK — whatsmeow tables created/verified")

	// GetFirstDevice returns the first persisted device, or inserts and returns
	// a blank device store if the table is empty.
	dev, err := container.GetFirstDevice(ctx)
	if err != nil {
		t.Fatalf("container.GetFirstDevice: %v", err)
	}
	if dev == nil {
		t.Fatal("expected a device (blank or otherwise), got nil")
	}
	t.Logf("whatsmeow store OK; device JID = %v (nil = unpaired, expected on first run)", dev.ID)
}
