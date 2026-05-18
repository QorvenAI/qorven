// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scraper

import (
	"fmt"
	"math/rand"
	"runtime"
	"strings"
)

// Fingerprint generates consistent browser fingerprints that match the User-Agent.
// Headers must be CONSISTENT — a Chrome UA with Firefox Accept headers is a red flag.

// BrowserProfile represents a complete browser identity.
type BrowserProfile struct {
	Name       string
	Version    int
	OS         string
	UserAgent  string
	Headers    map[string]string
}

var chromeVersions = []int{120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131}
var firefoxVersions = []int{120, 121, 122, 123, 124, 125, 126, 127, 128, 129, 130, 131, 132, 133}

// GenerateProfile creates a consistent browser profile where all headers match.
func GenerateProfile() BrowserProfile {
	osName := detectOS()
	browser := pickBrowser()
	version := pickVersion(browser)

	ua := buildUserAgent(browser, version, osName)
	headers := buildHeaders(browser, version, osName)

	return BrowserProfile{
		Name: browser, Version: version, OS: osName,
		UserAgent: ua, Headers: headers,
	}
}

func detectOS() string {
	switch runtime.GOOS {
	case "darwin": return "macOS"
	case "windows": return "Windows"
	default: return "Linux"
	}
}

func pickBrowser() string {
	browsers := []string{"chrome", "chrome", "chrome", "firefox", "edge"} // weighted toward chrome
	return browsers[rand.Intn(len(browsers))]
}

func pickVersion(browser string) int {
	switch browser {
	case "chrome", "edge":
		return chromeVersions[rand.Intn(len(chromeVersions))]
	case "firefox":
		return firefoxVersions[rand.Intn(len(firefoxVersions))]
	}
	return 131
}

func buildUserAgent(browser string, version int, os string) string {
	var osPart string
	switch os {
	case "Windows": osPart = "Windows NT 10.0; Win64; x64"
	case "macOS": osPart = "Macintosh; Intel Mac OS X 10_15_7"
	default: osPart = "X11; Linux x86_64"
	}

	switch browser {
	case "chrome":
		return fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.0.0 Safari/537.36", osPart, version)
	case "firefox":
		return fmt.Sprintf("Mozilla/5.0 (%s; rv:%d.0) Gecko/20100101 Firefox/%d.0", osPart, version, version)
	case "edge":
		return fmt.Sprintf("Mozilla/5.0 (%s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.0.0 Safari/537.36 Edg/%d.0.0.0", osPart, version, version)
	}
	return userAgents[0]
}

func buildHeaders(browser string, version int, os string) map[string]string {
	h := map[string]string{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		"Accept-Language":           "en-US,en;q=0.9",
		"Accept-Encoding":           "gzip, deflate, br",
		"DNT":                       "1",
		"Connection":                "keep-alive",
		"Upgrade-Insecure-Requests": "1",
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
	}

	// Browser-specific headers
	switch browser {
	case "chrome", "edge":
		h["sec-ch-ua"] = fmt.Sprintf(`"Chromium";v="%d", "Not_A Brand";v="8"`, version)
		h["sec-ch-ua-mobile"] = "?0"
		switch os {
		case "Windows": h["sec-ch-ua-platform"] = `"Windows"`
		case "macOS": h["sec-ch-ua-platform"] = `"macOS"`
		default: h["sec-ch-ua-platform"] = `"Linux"`
		}
	case "firefox":
		// Firefox doesn't send sec-ch-ua headers
		delete(h, "Sec-Fetch-User")
		h["Accept"] = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"
	}

	return h
}

// IsProxyError checks if an error is proxy-related (from scrapling-go).
func IsProxyError(err error) bool {
	if err == nil { return false }
	msg := strings.ToLower(err.Error())
	indicators := []string{
		"net::err_proxy", "net::err_tunnel", "connection refused",
		"connection reset", "connection timed out", "failed to connect",
		"could not resolve proxy", "proxy authentication required",
		"socks", "407",
	}
	for _, ind := range indicators {
		if strings.Contains(msg, ind) { return true }
	}
	return false
}

// BlockedResources lists resource types to block for faster scraping.
var BlockedResources = map[string]bool{
	"image": true, "media": true, "font": true, "stylesheet": true,
	"ping": true, "beacon": true, "websocket": true,
}

// BlockedDomains lists ad/tracking domains to block.
var BlockedDomains = map[string]bool{
	"google-analytics.com": true, "googletagmanager.com": true,
	"facebook.net": true, "doubleclick.net": true,
	"googlesyndication.com": true, "adservice.google.com": true,
	"analytics.google.com": true, "hotjar.com": true,
	"mixpanel.com": true, "segment.io": true,
	"amplitude.com": true, "intercom.io": true,
	"crisp.chat": true, "tawk.to": true,
}

// ShouldBlockDomain checks if a URL belongs to a blocked domain.
func ShouldBlockDomain(urlStr string) bool {
	for domain := range BlockedDomains {
		if strings.Contains(urlStr, domain) { return true }
	}
	return false
}
