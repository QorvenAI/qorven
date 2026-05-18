// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package zalo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// send.go — Send text, images, and files via Zalo protocol.

// ThreadType distinguishes DM vs group conversations.
type ThreadType int

const (
	ThreadUser  ThreadType = 0
	ThreadGroup ThreadType = 1
)

// SendText sends a text message to a thread.
func SendText(ctx context.Context, sess *Session, threadID string, threadType ThreadType, text string) (string, error) {
	payload := map[string]any{
		"toid":    threadID,
		"message": text,
		"ttl":     0,
	}

	encrypted, err := encryptPayload(sess, payload)
	if err != nil { return "", fmt.Errorf("zalo.send: encrypt: %w", err) }

	service := "chat"
	if threadType == ThreadGroup { service = "group" }
	url := getServiceURL(sess, service) + "/api/message/sms"

	body, _ := json.Marshal(map[string]any{"params": encrypted})
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil { return "", fmt.Errorf("zalo.send: request: %w", err) }
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var result APIResponse[map[string]any]
	json.Unmarshal(respBody, &result)

	if result.ErrorCode != 0 {
		return "", fmt.Errorf("zalo.send: error %d: %s", result.ErrorCode, result.ErrorMessage)
	}

	msgID, _ := result.Data["msgId"].(string)
	slog.Debug("zalo.send: sent", "thread", threadID, "msgId", msgID)
	return msgID, nil
}

// SendTyping sends a typing indicator to a thread.
func SendTyping(ctx context.Context, sess *Session, threadID string, threadType ThreadType) error {
	payload := map[string]any{"toid": threadID, "type": "typing"}
	encrypted, err := encryptPayload(sess, payload)
	if err != nil { return err }

	service := "chat"
	if threadType == ThreadGroup { service = "group" }
	url := getServiceURL(sess, service) + "/api/message/typing"

	body, _ := json.Marshal(map[string]any{"params": encrypted})
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil { return err }
	resp.Body.Close()
	return nil
}

func getServiceURL(sess *Session, service string) string {
	if sess.LoginInfo == nil { return ZaloBaseURL }
	switch service {
	case "chat":
		if len(sess.LoginInfo.ServiceMap.Chat) > 0 { return "https://" + sess.LoginInfo.ServiceMap.Chat[0] }
	case "group":
		if len(sess.LoginInfo.ServiceMap.Group) > 0 { return "https://" + sess.LoginInfo.ServiceMap.Group[0] }
	case "file":
		if len(sess.LoginInfo.ServiceMap.File) > 0 { return "https://" + sess.LoginInfo.ServiceMap.File[0] }
	}
	return ZaloBaseURL
}

func encryptPayload(sess *Session, payload map[string]any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil { return "", err }
	return EncryptCBC(SecretKey(sess.SecretKey).Bytes(), string(data), false)
}
