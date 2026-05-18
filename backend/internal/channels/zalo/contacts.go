// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package zalo

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// contacts.go — Fetch friends and groups from Zalo.

type FriendInfo struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar"`
	Phone       string `json:"phoneNumber,omitempty"`
}

type GroupInfo struct {
	GroupID     string `json:"groupId"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar"`
	MemberCount int    `json:"memberCount"`
}

// FetchFriends retrieves the user's friend list.
func FetchFriends(ctx context.Context, sess *Session) ([]FriendInfo, error) {
	url := getServiceURL(sess, "profile") + "/api/social/friend/getfriends"
	body, _ := json.Marshal(map[string]any{"params": map[string]any{"count": 500, "offset": 0}})

	resp, err := doPost(ctx, sess, url, body)
	if err != nil { return nil, fmt.Errorf("zalo.contacts: friends: %w", err) }

	encrypted, _ := resp["data"].(string)
	if encrypted == "" { return nil, fmt.Errorf("zalo.contacts: no data in response") }

	decrypted, err := DecryptCBC(SecretKey(sess.SecretKey).Bytes(), encrypted)
	if err != nil { return nil, fmt.Errorf("zalo.contacts: decrypt: %w", err) }

	var friends []FriendInfo
	json.Unmarshal(decrypted, &friends)
	slog.Debug("zalo.contacts: fetched friends", "count", len(friends))
	return friends, nil
}

// FetchGroups retrieves the user's group list.
func FetchGroups(ctx context.Context, sess *Session) ([]GroupInfo, error) {
	url := getServiceURL(sess, "group") + "/api/group/getJoinedGroups"
	body, _ := json.Marshal(map[string]any{"params": map[string]any{}})

	resp, err := doPost(ctx, sess, url, body)
	if err != nil { return nil, fmt.Errorf("zalo.contacts: groups: %w", err) }

	encrypted, _ := resp["data"].(string)
	if encrypted == "" { return nil, fmt.Errorf("zalo.contacts: no data") }

	decrypted, err := DecryptCBC(SecretKey(sess.SecretKey).Bytes(), encrypted)
	if err != nil { return nil, fmt.Errorf("zalo.contacts: decrypt: %w", err) }

	var result struct {
		Groups []GroupInfo `json:"gridInfoMap"`
	}
	json.Unmarshal(decrypted, &result)
	slog.Debug("zalo.contacts: fetched groups", "count", len(result.Groups))
	return result.Groups, nil
}

func getProfileServiceURL(sess *Session) string {
	if sess.LoginInfo != nil && len(sess.LoginInfo.ServiceMap.Profile) > 0 {
		return "https://" + sess.LoginInfo.ServiceMap.Profile[0]
	}
	return ZaloBaseURL
}
