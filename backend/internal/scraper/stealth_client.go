// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scraper

import (
	"context"
	stdtls "crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	tls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// StealthClient creates an HTTP client that spoofs a real browser's TLS fingerprint.
// Uses uTLS to mimic Chrome/Firefox/Safari TLS ClientHello, bypassing JA3/JA4 detection.

type TLSProfile int

const (
	ProfileChrome TLSProfile = iota
	ProfileFirefox
	ProfileSafari
	ProfileEdge
	ProfileRandom
)

var profileIDs = []tls.ClientHelloID{
	tls.HelloChrome_Auto,
	tls.HelloFirefox_Auto,
	tls.HelloSafari_Auto,
	tls.HelloEdge_Auto,
}

func NewStealthClient(profile TLSProfile, timeout time.Duration) *http.Client {
	if timeout == 0 { timeout = 30 * time.Second }

	var helloID tls.ClientHelloID
	switch profile {
	case ProfileChrome:  helloID = tls.HelloChrome_Auto
	case ProfileFirefox: helloID = tls.HelloFirefox_Auto
	case ProfileSafari:  helloID = tls.HelloSafari_Auto
	case ProfileEdge:    helloID = tls.HelloEdge_Auto
	case ProfileRandom:  helloID = profileIDs[rand.Intn(len(profileIDs))]
	default:             helloID = tls.HelloChrome_Auto
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}

	// HTTP/2 transport with uTLS — handles modern sites that require H2
	h2Transport := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *stdtls.Config) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(addr)
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil { return nil, err }
			tlsConn := tls.UClient(conn, &tls.Config{ServerName: host}, helloID)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				conn.Close()
				return nil, err
			}
			return tlsConn, nil
		},
	}

	return &http.Client{
		Transport: h2Transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 { return fmt.Errorf("too many redirects") }
			return nil
		},
	}
}

// StealthFetch fetches a URL with spoofed TLS fingerprint + browser headers.
func StealthFetch(ctx context.Context, rawURL string) (string, int, error) {
	client := NewStealthClient(ProfileRandom, 30*time.Second)

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil { return "", 0, err }

	profile := GenerateProfile()
	req.Header.Set("User-Agent", profile.UserAgent)
	for k, v := range profile.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil { return "", 0, err }
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil { return "", resp.StatusCode, err }

	return string(body), resp.StatusCode, nil
}

// StealthFetchMarkdown fetches and converts HTML to clean text.
func StealthFetchMarkdown(ctx context.Context, rawURL string) (string, error) {
	html, status, err := StealthFetch(ctx, rawURL)
	if err != nil { return "", err }
	if status >= 400 { return "", fmt.Errorf("HTTP %d", status) }
	text := htmlToText(html)
	if text == "" && len(html) > 0 {
		if len(html) > 10000 { html = html[:10000] }
		return html, nil
	}
	return text, nil
}

func htmlToText(html string) string {
	for _, tag := range []string{"script", "style", "noscript"} {
		for {
			start := strings.Index(strings.ToLower(html), "<"+tag)
			if start < 0 { break }
			end := strings.Index(strings.ToLower(html[start:]), "</"+tag+">")
			if end < 0 { break }
			html = html[:start] + html[start+end+len("</"+tag+">"):]
		}
	}
	var result strings.Builder
	inTag := false
	for _, c := range html {
		if c == '<' { inTag = true; continue }
		if c == '>' { inTag = false; result.WriteRune(' '); continue }
		if !inTag { result.WriteRune(c) }
	}
	text := result.String()
	lines := strings.Split(text, "\n")
	clean := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" { clean = append(clean, line) }
	}
	return strings.Join(clean, "\n")
}
