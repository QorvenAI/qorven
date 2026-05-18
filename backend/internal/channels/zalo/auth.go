// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package zalo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// auth.go — Zalo authentication: QR login, credential login, session management.

const (
	DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	DefaultLanguage  = "vi"
	MaxRedirects     = 5
	ZaloBaseURL      = "https://chat.zalo.me"
)

// Session holds the authenticated state for a Zalo connection.
type Session struct {
	UID       string
	IMEI      string
	UserAgent string
	Language  string
	SecretKey string // base64-encoded zpw_enk

	LoginInfo *LoginInfo
	Settings  *Settings
	CookieJar http.CookieJar
	Client    *http.Client
}

// NewSession creates a fresh unauthenticated session.
func NewSession() *Session {
	jar, _ := cookiejar.New(nil)
	return &Session{
		UserAgent: DefaultUserAgent,
		Language:  DefaultLanguage,
		CookieJar: jar,
		Client: &http.Client{
			Jar:     jar,
			Timeout: 60 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= MaxRedirects { return fmt.Errorf("zalo: too many redirects") }
				return nil
			},
		},
	}
}

// LoginWithCredentials authenticates using saved credentials (IMEI + secret key).
func LoginWithCredentials(ctx context.Context, sess *Session, cfg ZaloConfig) error {
	sess.IMEI = cfg.IMEI
	sess.SecretKey = cfg.SecretKey
	if cfg.UserAgent != "" { sess.UserAgent = cfg.UserAgent }

	loginInfo, err := fetchLoginInfo(ctx, sess)
	if err != nil { return fmt.Errorf("zalo.auth: login info: %w", err) }
	sess.LoginInfo = loginInfo
	sess.UID = loginInfo.UID

	serverInfo, err := fetchServerInfo(ctx, sess)
	if err != nil { return fmt.Errorf("zalo.auth: server info: %w", err) }
	if serverInfo.Settings != nil { sess.Settings = serverInfo.Settings }

	slog.Info("zalo.auth: logged in", "uid", sess.UID)
	return nil
}

// LoginQR performs QR code login. Calls qrCallback with the QR PNG image bytes.
func LoginQR(ctx context.Context, sess *Session, qrCallback func(qrPNG []byte)) (*ZaloConfig, error) {
	// Load login page to get cookies
	ver, err := loadLoginPage(ctx, sess)
	if err != nil { return nil, fmt.Errorf("zalo.auth: load page: %w", err) }

	// Generate QR code
	qrData, qrPNG, err := qrGenerateCode(ctx, sess, ver)
	if err != nil { return nil, fmt.Errorf("zalo.auth: generate QR: %w", err) }

	if qrCallback != nil { qrCallback(qrPNG) }

	// Wait for scan
	if err := qrWaitingScan(ctx, sess, ver, qrData.Code); err != nil {
		return nil, fmt.Errorf("zalo.auth: waiting scan: %w", err)
	}

	// Wait for confirm
	if err := qrWaitingConfirm(ctx, sess, ver, qrData.Code); err != nil {
		return nil, fmt.Errorf("zalo.auth: waiting confirm: %w", err)
	}

	// Get login info
	loginInfo, err := fetchLoginInfo(ctx, sess)
	if err != nil { return nil, fmt.Errorf("zalo.auth: login info: %w", err) }
	sess.LoginInfo = loginInfo
	sess.UID = loginInfo.UID

	slog.Info("zalo.auth: QR login success", "uid", sess.UID)
	return &ZaloConfig{
		SecretKey: loginInfo.ZPWEnk,
		IMEI:      sess.IMEI,
	}, nil
}

func fetchLoginInfo(ctx context.Context, sess *Session) (*LoginInfo, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", ZaloBaseURL+"/api/login/getLoginInfo", nil)
	setDefaultHeaders(req, sess)
	resp, err := sess.Client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	var result APIResponse[json.RawMessage]
	if err := readJSON(resp, &result); err != nil { return nil, err }
	if result.ErrorCode != 0 { return nil, fmt.Errorf("zalo: getLoginInfo error %d: %s", result.ErrorCode, result.ErrorMessage) }

	// Decrypt with secret key
	decrypted, err := DecryptCBC(SecretKey(sess.SecretKey).Bytes(), string(result.Data))
	if err != nil { return nil, fmt.Errorf("zalo: decrypt login info: %w", err) }

	var info LoginInfo
	if err := json.Unmarshal(decrypted, &info); err != nil { return nil, err }
	return &info, nil
}

func fetchServerInfo(ctx context.Context, sess *Session) (*ServerInfo, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", ZaloBaseURL+"/api/login/getServerInfo", nil)
	setDefaultHeaders(req, sess)
	resp, err := sess.Client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	var result APIResponse[ServerInfo]
	if err := readJSON(resp, &result); err != nil { return nil, err }
	return &result.Data, nil
}

func loadLoginPage(ctx context.Context, sess *Session) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", ZaloBaseURL, nil)
	setDefaultHeaders(req, sess)
	resp, err := sess.Client.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	// Extract version from page
	for _, line := range strings.Split(string(body), "\n") {
		if strings.Contains(line, "data-version") {
			start := strings.Index(line, `"`) + 1
			end := strings.LastIndex(line, `"`)
			if start > 0 && end > start { return line[start:end], nil }
		}
	}
	return "1", nil
}

func qrGenerateCode(ctx context.Context, sess *Session, ver string) (*QRData, []byte, error) {
	form := url.Values{"ver": {ver}}
	req, _ := http.NewRequestWithContext(ctx, "POST", ZaloBaseURL+"/api/login/qr/generate", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setDefaultHeaders(req, sess)
	resp, err := sess.Client.Do(req)
	if err != nil { return nil, nil, err }
	defer resp.Body.Close()

	var result APIResponse[QRData]
	if err := readJSON(resp, &result); err != nil { return nil, nil, err }
	if result.ErrorCode != 0 { return nil, nil, fmt.Errorf("zalo: QR generate error %d", result.ErrorCode) }

	// Decode base64 PNG from data URI
	var png []byte
	if idx := strings.Index(result.Data.Image, ","); idx >= 0 {
		png = []byte(result.Data.Image[idx+1:])
	}
	return &result.Data, png, nil
}

func qrWaitingScan(ctx context.Context, sess *Session, ver, code string) error {
	for {
		select {
		case <-ctx.Done(): return ctx.Err()
		default:
		}
		form := url.Values{"ver": {ver}, "code": {code}}
		req, _ := http.NewRequestWithContext(ctx, "POST", ZaloBaseURL+"/api/login/qr/waiting-scan", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		setDefaultHeaders(req, sess)
		resp, err := sess.Client.Do(req)
		if err != nil { return err }
		var result APIResponse[QRScannedData]
		readJSON(resp, &result)
		resp.Body.Close()
		if result.ErrorCode == 0 {
			slog.Info("zalo.auth: QR scanned", "name", result.Data.DisplayName)
			return nil
		}
		time.Sleep(2 * time.Second)
	}
}

func qrWaitingConfirm(ctx context.Context, sess *Session, ver, code string) error {
	for {
		select {
		case <-ctx.Done(): return ctx.Err()
		default:
		}
		form := url.Values{"ver": {ver}, "code": {code}}
		req, _ := http.NewRequestWithContext(ctx, "POST", ZaloBaseURL+"/api/login/qr/waiting-confirm", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		setDefaultHeaders(req, sess)
		resp, err := sess.Client.Do(req)
		if err != nil { return err }
		var result APIResponse[json.RawMessage]
		readJSON(resp, &result)
		resp.Body.Close()
		if result.ErrorCode == 0 { return nil }
		time.Sleep(2 * time.Second)
	}
}

func setDefaultHeaders(req *http.Request, sess *Session) {
	req.Header.Set("User-Agent", sess.UserAgent)
	req.Header.Set("Accept-Language", sess.Language)
	req.Header.Set("Referer", ZaloBaseURL)
}

func readJSON(resp *http.Response, target any) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil { return err }
	return json.Unmarshal(body, target)
}
