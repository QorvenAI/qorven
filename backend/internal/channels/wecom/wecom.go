// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package wecom

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

// WeChat Work (企业微信 / WeCom) channel — 180M+ Chinese enterprise users.
// Auth: CorpID + AgentSecret → access_token (2 hours); passed as URL query param.
// Inbound: HTTPS webhook with AES-256-CBC encrypted XML payloads + SHA1 signature.
// Outbound: REST API with JSON, access_token as query param (NOT Authorization header).
// Reference: https://developer.work.weixin.qq.com

const (
	wecomAPI      = "https://qyapi.weixin.qq.com/cgi-bin"
	maxMessageLen = 4096 // WeChat Work text/markdown character limit
)

type Config struct {
	AgentID      string `json:"agent_id"`
	CorpID       string `json:"corp_id"`        // 企业ID
	AgentSecret  string `json:"agent_secret"`   // 应用Secret
	WecomAgentID int    `json:"wecom_agent_id"` // 应用AgentId (integer, required on all sends)
	Token        string `json:"token"`          // webhook verification token
	EncodingKey  string `json:"encoding_key"`   // AES encoding key (43 chars base64, no padding)
}

type WeComChannel struct {
	cfg         Config
	handler     channels.InboundHandler
	running     bool
	mu          sync.Mutex
	tokenMu     sync.Mutex // separate mutex; token fetch must not block IsRunning()
	client      *http.Client
	debouncer   *channels.Debouncer
	accessToken string
	tokenExp    time.Time
	aesKey      []byte   // decoded AES key (32 bytes)
	dedup       sync.Map // MsgId → time.Time
}

func New(cfg Config, handler channels.InboundHandler) (*WeComChannel, error) {
	var aesKey []byte
	if cfg.EncodingKey != "" {
		// WeCom encoding key is 43 chars base64 without padding; add "=" before decoding
		decoded, err := base64.StdEncoding.DecodeString(cfg.EncodingKey + "=")
		if err != nil {
			return nil, fmt.Errorf("wecom: invalid encoding key: %w", err)
		}
		aesKey = decoded
	}
	return &WeComChannel{
		cfg:    cfg,
		handler: handler,
		aesKey: aesKey,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (w *WeComChannel) Name() string    { return "wecom" }
func (w *WeComChannel) Type() string    { return "wecom" }
func (w *WeComChannel) AgentID() string { return w.cfg.AgentID }
func (w *WeComChannel) IsRunning() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.running }

func (w *WeComChannel) Start(ctx context.Context) error {
	w.debouncer = channels.NewDebouncer(600*time.Millisecond, func(msg channels.InboundMessage) {
		if w.handler != nil {
			w.handler(ctx, msg)
		}
	})
	w.mu.Lock()
	w.running = true
	w.mu.Unlock()
	slog.Info("wecom.started", "corp", w.cfg.CorpID, "agent", w.cfg.AgentID)
	return nil
}

func (w *WeComChannel) Stop(_ context.Context) error {
	if w.debouncer != nil {
		w.debouncer.FlushAll()
	}
	w.mu.Lock()
	w.running = false
	w.mu.Unlock()
	return nil
}

// --- Access Token ---

func (w *WeComChannel) getToken() (string, error) {
	w.tokenMu.Lock()
	defer w.tokenMu.Unlock()
	if w.accessToken != "" && time.Now().Before(w.tokenExp) {
		return w.accessToken, nil
	}

	url := fmt.Sprintf("%s/gettoken?corpid=%s&corpsecret=%s", wecomAPI, w.cfg.CorpID, w.cfg.AgentSecret)
	resp, err := w.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("wecom token: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.ErrCode != 0 {
		return "", fmt.Errorf("wecom token: %d %s", result.ErrCode, result.ErrMsg)
	}
	w.accessToken = result.AccessToken
	// Refresh 60 seconds before actual expiry.
	w.tokenExp = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)
	return w.accessToken, nil
}

// --- Signature Verification ---

// verifySignature verifies the WeChat Work SHA1 signature.
// Inputs: token, timestamp, nonce, plus a 4th value (echostr for GET, encrypt for POST).
// All four are sorted alphabetically, concatenated, then SHA1-hashed.
func (w *WeComChannel) verifySignature(msgSig, timestamp, nonce, fourth string) bool {
	if w.cfg.Token == "" {
		return true
	}
	strs := []string{w.cfg.Token, timestamp, nonce, fourth}
	sort.Strings(strs)
	hash := sha1.Sum([]byte(strings.Join(strs, "")))
	return fmt.Sprintf("%x", hash) == msgSig
}

// --- Webhook Handler ---

func (w *WeComChannel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	if r.Method == "GET" {
		// One-time URL verification challenge
		msgSig := q.Get("msg_signature")
		timestamp := q.Get("timestamp")
		nonce := q.Get("nonce")
		echostr := q.Get("echostr")

		if !w.verifySignature(msgSig, timestamp, nonce, echostr) {
			http.Error(rw, "verification failed", http.StatusForbidden)
			return
		}
		decrypted := w.decrypt(echostr)
		rw.Write([]byte(decrypted))
		return
	}

	// POST: verify signature before reading the encrypted payload
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))

	var encMsg struct {
		XMLName    xml.Name `xml:"xml"`
		ToUserName string   `xml:"ToUserName"`
		Encrypt    string   `xml:"Encrypt"`
		AgentID    string   `xml:"AgentID"`
	}
	if err := xml.Unmarshal(body, &encMsg); err != nil || encMsg.Encrypt == "" {
		// WeChat Work requires "success" body even on errors for retries to stop
		rw.Write([]byte("success"))
		return
	}

	// Verify POST signature: 4th param is the Encrypt value from the XML body
	msgSig := q.Get("msg_signature")
	timestamp := q.Get("timestamp")
	nonce := q.Get("nonce")
	if !w.verifySignature(msgSig, timestamp, nonce, encMsg.Encrypt) {
		slog.Warn("wecom.webhook.signature_invalid")
		http.Error(rw, "invalid signature", http.StatusUnauthorized)
		return
	}

	decrypted := w.decrypt(encMsg.Encrypt)
	if decrypted == "" {
		rw.Write([]byte("success"))
		return
	}

	var msg wecomMessage
	xml.Unmarshal([]byte(decrypted), &msg)

	// Dedup by MsgId — WeChat Work retries 3 times on non-success
	if msg.MsgId != 0 {
		key := fmt.Sprintf("%d", msg.MsgId)
		if _, loaded := w.dedup.LoadOrStore(key, time.Now()); loaded {
			rw.Write([]byte("success"))
			return
		}
		go func() { time.Sleep(2 * time.Hour); w.dedup.Delete(key) }()
	}

	w.handleMessage(r.Context(), &msg)

	// WeChat Work requires the string "success" in the response body — empty 200 causes retries
	rw.Write([]byte("success"))
}

func (w *WeComChannel) handleMessage(ctx context.Context, msg *wecomMessage) {
	if msg.MsgType == "" || msg.FromUserName == "" {
		return
	}

	var text string
	meta := map[string]string{
		"chat_id":    msg.FromUserName,
		"message_id": fmt.Sprintf("%d", msg.MsgId),
		"msg_type":   msg.MsgType,
	}

	switch msg.MsgType {
	case "text":
		text = strings.TrimSpace(msg.Content)
	case "image":
		meta["has_media"] = "true"
		meta["media_type"] = "image"
		text = "[Image attachment]"
	case "voice":
		meta["has_media"] = "true"
		meta["media_type"] = "voice"
		text = "[Voice attachment]"
	case "video":
		meta["has_media"] = "true"
		meta["media_type"] = "video"
		text = "[Video attachment]"
	case "file":
		meta["has_media"] = "true"
		meta["media_type"] = "file"
		text = fmt.Sprintf("[File: %s]", msg.Title)
		if text == "[File: ]" {
			text = "[File attachment]"
		}
	case "location":
		text = fmt.Sprintf("[Location: %s (%.6f, %.6f)]", msg.Label, msg.LocationX, msg.LocationY)
	case "link":
		text = fmt.Sprintf("[Link: %s — %s]", msg.Title, msg.Url)
	case "event":
		slog.Info("wecom.event", "event", msg.Event, "key", msg.EventKey, "from", msg.FromUserName)
		return
	default:
		text = fmt.Sprintf("[%s message]", msg.MsgType)
	}

	if text == "" {
		return
	}

	slog.Info("wecom.inbound", "from", msg.FromUserName, "type", msg.MsgType)
	w.debouncer.Push(channels.InboundMessage{
		ChannelName: "wecom",
		ChannelType: "wecom",
		AgentID:     w.cfg.AgentID,
		SenderID:    msg.FromUserName,
		ChatID:      msg.FromUserName,
		Content:     text,
		Metadata:    meta,
	})
}

// --- Send ---

func (w *WeComChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	toUser := msg.RecipientID
	if toUser == "" {
		toUser = msg.ChatID
	}
	if toUser == "" && msg.Metadata != nil {
		toUser = msg.Metadata["chat_id"]
	}
	if toUser == "" {
		return fmt.Errorf("wecom: no recipient")
	}

	for _, chunk := range splitMessage(content, maxMessageLen) {
		if err := w.sendText(toUser, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (w *WeComChannel) sendText(toUser, content string) error {
	token, err := w.getToken()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"touser":  toUser,
		"msgtype": "text",
		"agentid": w.cfg.WecomAgentID,
		"text":    map[string]string{"content": content},
	})
	return w.apiPost("/message/send", token, payload)
}

// SendMarkdown sends a markdown-formatted message.
func (w *WeComChannel) SendMarkdown(toUser, content string) error {
	token, err := w.getToken()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"touser":   toUser,
		"msgtype":  "markdown",
		"agentid":  w.cfg.WecomAgentID,
		"markdown": map[string]string{"content": content},
	})
	return w.apiPost("/message/send", token, payload)
}

// SendNews sends a news/article card.
func (w *WeComChannel) SendNews(toUser, title, description, url, picURL string) error {
	token, err := w.getToken()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"touser":  toUser,
		"msgtype": "news",
		"agentid": w.cfg.WecomAgentID,
		"news": map[string]any{
			"articles": []map[string]string{{
				"title": title, "description": description, "url": url, "picurl": picURL,
			}},
		},
	})
	return w.apiPost("/message/send", token, payload)
}

// SendTemplateCard sends an interactive template card.
func (w *WeComChannel) SendTemplateCard(toUser string, card map[string]any) error {
	token, err := w.getToken()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"touser":        toUser,
		"msgtype":       "template_card",
		"agentid":       w.cfg.WecomAgentID,
		"template_card": card,
	})
	return w.apiPost("/message/send", token, payload)
}

// SendImage sends an image by media_id (upload via /media/upload first).
func (w *WeComChannel) SendImage(toUser, mediaID string) error {
	token, err := w.getToken()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"touser":  toUser,
		"msgtype": "image",
		"agentid": w.cfg.WecomAgentID,
		"image":   map[string]string{"media_id": mediaID},
	})
	return w.apiPost("/message/send", token, payload)
}

// access_token is passed as URL query param, NOT Authorization header — WeChat Work spec
func (w *WeComChannel) apiPost(path, token string, payload []byte) error {
	url := fmt.Sprintf("%s%s?access_token=%s", wecomAPI, path, token)
	resp, err := w.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("wecom api: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.ErrCode != 0 {
		return fmt.Errorf("wecom api: %d %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

// --- AES-256-CBC Decryption ---

func (w *WeComChannel) decrypt(encrypted string) string {
	if len(w.aesKey) == 0 {
		return encrypted
	}
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return ""
	}
	block, err := aes.NewCipher(w.aesKey)
	if err != nil {
		return ""
	}
	if len(ciphertext) < aes.BlockSize || len(ciphertext)%aes.BlockSize != 0 {
		return ""
	}
	iv := w.aesKey[:aes.BlockSize]
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)
	// Remove PKCS7 padding
	pad := int(ciphertext[len(ciphertext)-1])
	if pad > aes.BlockSize || pad == 0 || pad > len(ciphertext) {
		return ""
	}
	ciphertext = ciphertext[:len(ciphertext)-pad]
	// Strip 16-byte random prefix + 4-byte big-endian message length
	if len(ciphertext) < 20 {
		return ""
	}
	msgLen := binary.BigEndian.Uint32(ciphertext[16:20])
	if int(msgLen) > len(ciphertext)-20 {
		return ""
	}
	return string(ciphertext[20 : 20+msgLen])
}

// --- Types ---

type wecomMessage struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
	MsgId        int64    `xml:"MsgId"`
	PicUrl       string   `xml:"PicUrl"`
	MediaId      string   `xml:"MediaId"`
	Title        string   `xml:"Title"`
	Url          string   `xml:"Url"`
	Label        string   `xml:"Label"`
	LocationX    float64  `xml:"Location_X"`
	LocationY    float64  `xml:"Location_Y"`
	Event        string   `xml:"Event"`
	EventKey     string   `xml:"EventKey"`
}

// splitMessage splits text at line boundaries before the limit.
func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}
	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}
		cut := maxLen
		if idx := strings.LastIndex(text[:maxLen], "\n"); idx > maxLen/2 {
			cut = idx + 1
		}
		chunks = append(chunks, text[:cut])
		text = text[cut:]
	}
	return chunks
}
