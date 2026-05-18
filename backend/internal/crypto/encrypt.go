// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	encPrefix    = "aes-gcm:"
	apiKeyPrefix = "qorven_"
)

// Encrypt encrypts data using AES-256-GCM.
func Encrypt(plaintext []byte, keyHex string) ([]byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("key must be 64 hex chars (32 bytes)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts AES-256-GCM data.
func Decrypt(ciphertext []byte, keyHex string) ([]byte, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("key must be 64 hex chars (32 bytes)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, ciphertext[:ns], ciphertext[ns:], nil)
}

// EncryptString encrypts plaintext, returning "aes-gcm:" + base64(nonce+ciphertext).
// If key is empty, returns plaintext unchanged.
func EncryptString(plaintext, key string) (string, error) {
	if key == "" || plaintext == "" {
		return plaintext, nil
	}
	keyBytes, err := DeriveKey(key)
	if err != nil {
		return "", err
	}
	ct, err := Encrypt([]byte(plaintext), hex.EncodeToString(keyBytes))
	if err != nil {
		return "", err
	}
	return encPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptString decrypts ciphertext produced by EncryptString.
// Values without "aes-gcm:" prefix are returned as-is (backward compat).
func DecryptString(ciphertext, key string) (string, error) {
	if key == "" || ciphertext == "" || !IsEncrypted(ciphertext) {
		return ciphertext, nil
	}
	keyBytes, err := DeriveKey(key)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, encPrefix))
	if err != nil {
		return ciphertext, nil
	}
	pt, err := Decrypt(data, hex.EncodeToString(keyBytes))
	if err != nil {
		return "", errors.New("decrypt failed: invalid key or corrupted data")
	}
	return string(pt), nil
}

// IsEncrypted returns true if the value has the "aes-gcm:" prefix.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, encPrefix)
}

// DeriveKey converts input to a 32-byte AES key.
// Accepts: hex (64 chars), base64 (44 chars), or raw 32 bytes.
func DeriveKey(input string) ([]byte, error) {
	if len(input) == 64 {
		if b, err := hex.DecodeString(input); err == nil {
			return b, nil
		}
	}
	if len(input) == 44 && strings.HasSuffix(input, "=") {
		if b, err := base64.StdEncoding.DecodeString(input); err == nil && len(b) == 32 {
			return b, nil
		}
	}
	if len(input) == 32 {
		return []byte(input), nil
	}
	return nil, errors.New("key must be 32 bytes (hex 64 chars, base64 44 chars, or raw)")
}

// GenerateAPIKey creates a new API key with format "qorven_<32hex>".
// Returns raw key (show once), SHA-256 hash, and 8-char display prefix.
func GenerateAPIKey() (raw, hash, displayPrefix string, err error) {
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	raw = apiKeyPrefix + hex.EncodeToString(b)
	hash = HashAPIKey(raw)
	displayPrefix = hex.EncodeToString(b[:4])
	return raw, hash, displayPrefix, nil
}

// HashAPIKey returns the SHA-256 hex digest of a raw API key for use as a
// lookup token. This is NOT used for password hashing — passwords go through
// bcrypt (see auth package). SHA-256 is appropriate here because API keys are
// already high-entropy random secrets, so pre-image resistance is sufficient.
func HashAPIKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
