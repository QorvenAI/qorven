// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package scraper

import (
	"crypto/tls"
	"math/rand"
	"net/http/cookiejar"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Stealth adds anti-detection capabilities to the scraper.

var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:133.0) Gecko/20100101 Firefox/133.0",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
}

// ProxyRotator cycles through a list of proxy URLs.
type ProxyRotator struct {
	mu      sync.Mutex
	proxies []*url.URL
	index   int
}

// NewProxyRotator creates a rotator from proxy URL strings.
func NewProxyRotator(proxyURLs []string) *ProxyRotator {
	proxies := []*url.URL{}
	for _, p := range proxyURLs {
		if u, err := url.Parse(p); err == nil {
			proxies = append(proxies, u)
		}
	}
	return &ProxyRotator{proxies: proxies}
}

// Next returns the next proxy in rotation.
func (pr *ProxyRotator) Next() *url.URL {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if len(pr.proxies) == 0 {
		return nil
	}
	proxy := pr.proxies[pr.index%len(pr.proxies)]
	pr.index++
	return proxy
}

// SetProxies configures proxy rotation on the scraper.
func (s *Scraper) SetProxies(proxyURLs []string) {
	s.proxyRotator = NewProxyRotator(proxyURLs)
	s.client.Transport = &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return s.proxyRotator.Next(), nil
		},
	}
}

// RotateUserAgent picks a random user agent for each request.
func (s *Scraper) RotateUserAgent() {
	s.userAgent = userAgents[rand.Intn(len(userAgents))]
}

// SetStealth enables all anti-detection features.
func (s *Scraper) SetStealth() {
	s.stealth = true
	// Generate a consistent browser profile
	profile := GenerateProfile()
	s.userAgent = profile.UserAgent
	for k, v := range profile.Headers {
		s.headers[k] = v
	}
	s.EnableCookieJar()
	s.RandomizeTLS()
}

// RandomDelay adds a random delay between requests to avoid detection.
func RandomDelay(min, max time.Duration) {
	delay := min + time.Duration(rand.Int63n(int64(max-min)))
	time.Sleep(delay)
}

// CookieJar enables persistent cookies across requests.
func (s *Scraper) EnableCookieJar() {
	jar, _ := cookiejar.New(nil)
	s.client.Jar = jar
}

// RandomizeTLS randomizes TLS fingerprint to avoid detection.
func (s *Scraper) RandomizeTLS() {
	s.client.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			CipherSuites: shuffleCiphers([]uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_AES_256_GCM_SHA384,
			}),
		},
		Proxy: func(req *http.Request) (*url.URL, error) {
			if s.proxyRotator != nil { return s.proxyRotator.Next(), nil }
			return nil, nil
		},
	}
}

func shuffleCiphers(ciphers []uint16) []uint16 {
	rand.Shuffle(len(ciphers), func(i, j int) { ciphers[i], ciphers[j] = ciphers[j], ciphers[i] })
	return ciphers
}
