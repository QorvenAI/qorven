// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package zalo

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

// --- Protocol types ---

// SecretKey is a base64-encoded secret key from Zalo login.
type SecretKey string

func (s SecretKey) Bytes() []byte {
	decoded, _ := base64.StdEncoding.DecodeString(string(s))
	return decoded
}

// LoginInfo from getLoginInfo response.
type LoginInfo struct {
	UID          string          `json:"uid"`
	ZPWEnk       string          `json:"zpw_enk"`
	WebSockets   []string        `json:"zpw_ws"`
	ServiceMap   ServiceMapV3    `json:"zpw_service_map_v3"`
}

type ServiceMapV3 struct {
	Chat    []string `json:"chat"`
	Group   []string `json:"group"`
	File    []string `json:"file"`
	Profile []string `json:"profile"`
}

// ServerInfo from getServerInfo response.
type ServerInfo struct {
	Settings *Settings `json:"settings"`
}

func (s *ServerInfo) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil { return err }
	for _, k := range []string{"settings", "setttings"} {
		if v, ok := raw[k]; ok { return json.Unmarshal(v, &s.Settings) }
	}
	return nil
}

type Settings struct {
	Features  Features          `json:"features"`
	Keepalive KeepaliveSettings `json:"keepalive"`
}

type Features struct {
	Socket SocketSettings `json:"socket"`
}

type SocketSettings struct {
	PingInterval     int                          `json:"ping_interval"`
	Retries          map[string]SocketRetryConfig `json:"retries"`
	CloseAndRetry    []int                        `json:"close_and_retry_codes"`
	RotateErrorCodes []int                        `json:"rotate_error_codes"`
}

type SocketRetryConfig struct {
	Max   *int  `json:"max,omitempty"`
	Times []int `json:"times"`
}

func (r *SocketRetryConfig) UnmarshalJSON(data []byte) error {
	type alias struct {
		Max   *int            `json:"max,omitempty"`
		Times json.RawMessage `json:"times"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil { return err }
	r.Max = a.Max
	if err := json.Unmarshal(a.Times, &r.Times); err != nil {
		var single int
		if err2 := json.Unmarshal(a.Times, &single); err2 != nil { return err }
		r.Times = []int{single}
	}
	return nil
}

type KeepaliveSettings struct {
	AlwaysKeepalive   uint `json:"alway_keepalive"`
	KeepaliveDuration uint `json:"keepalive_duration"`
}

// --- API response types ---

type APIResponse[T any] struct {
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
	Data         T      `json:"data"`
}

type QRData struct {
	Code  string `json:"code"`
	Image string `json:"image"`
}

type QRScannedData struct {
	Avatar      string `json:"avatar"`
	DisplayName string `json:"display_name"`
}

type UserInfo struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

// --- Channel message types ---

type InboundMessage struct {
	MessageID string    `json:"message_id"`
	SenderID  string    `json:"sender_id"`
	ChatID    string    `json:"chat_id"`
	Text      string    `json:"text"`
	ImageURL  string    `json:"image_url,omitempty"`
	FileURL   string    `json:"file_url,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	IsGroup   bool      `json:"is_group"`
}

type Contact struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Avatar      string `json:"avatar"`
	Phone       string `json:"phone,omitempty"`
}

// ZaloConfig holds configuration for the Zalo channel.
type ZaloConfig struct {
	AppID        string `json:"app_id" toml:"app_id"`
	AppSecret    string `json:"app_secret" toml:"app_secret"`       // for webhook signature + token refresh
	RefreshToken string `json:"refresh_token" toml:"refresh_token"` // rotates on every token refresh
	AccessToken  string `json:"access_token" toml:"access_token"`   // short-lived; auto-refreshed
	WebhookURL   string `json:"webhook_url" toml:"webhook_url"`
	AgentID      string `json:"agent_id" toml:"agent_id"`
	// Personal mode (QR login)
	PersonalMode bool   `json:"personal_mode" toml:"personal_mode"`
	SecretKey    string `json:"secret_key" toml:"secret_key"` // AES key for personal mode
	IMEI         string `json:"imei" toml:"imei"`
	UserAgent    string `json:"user_agent" toml:"user_agent"`
}

// zaloEvent is the inbound webhook event payload from Zalo OA.
type zaloEvent struct {
	AppID     string `json:"app_id"`
	EventName string `json:"event_name"`
	Timestamp int64  `json:"timestamp"`
	Sender    struct {
		ID string `json:"id"`
	} `json:"sender"`
	Recipient struct {
		ID string `json:"id"`
	} `json:"recipient"`
	Follower struct {
		ID string `json:"id"`
	} `json:"follower"`
	Message struct {
		MsgID       string           `json:"msg_id"`
		Text        string           `json:"text"`
		ReactIcon   string           `json:"react_icon"`
		Attachments []zaloAttachment `json:"attachments"`
	} `json:"message"`
}

type zaloAttachment struct {
	Type    string `json:"type"`
	Payload struct {
		URL         string `json:"url"`
		Name        string `json:"name"`
		Coordinates struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"coordinates"`
	} `json:"payload"`
}
