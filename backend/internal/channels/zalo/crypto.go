// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package zalo

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// crypto.go — Zalo protocol encryption (AES-CBC with zero IV, AES-GCM with 16-byte nonce).

var (
	errInvalidBlockSize    = errors.New("zalo: invalid block size")
	errInvalidPKCS7Data    = errors.New("zalo: invalid PKCS#7 data")
	errInvalidPKCS7Padding = errors.New("zalo: invalid PKCS#7 padding")
)

// EncryptCBC encrypts data with AES-CBC using a zero IV (Zalo protocol quirk).
// encHex=true returns hex-encoded ciphertext, false returns base64.
func EncryptCBC(key []byte, data string, encHex bool) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil { return "", fmt.Errorf("zalo.crypto: %w", err) }

	plain, err := pkcs7Pad([]byte(data), aes.BlockSize)
	if err != nil { return "", fmt.Errorf("zalo.crypto: %w", err) }

	iv := make([]byte, aes.BlockSize)
	ct := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, plain)

	if encHex { return hex.EncodeToString(ct), nil }
	return base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptCBC decrypts base64-encoded AES-CBC ciphertext with zero IV.
func DecryptCBC(key []byte, data string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(data)
	if err != nil { return nil, fmt.Errorf("zalo.crypto: base64: %w", err) }

	block, err := aes.NewCipher(key)
	if err != nil { return nil, fmt.Errorf("zalo.crypto: cipher: %w", err) }

	iv := make([]byte, aes.BlockSize)
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)

	plain, err = pkcs7Unpad(plain, aes.BlockSize)
	if err != nil { return nil, fmt.Errorf("zalo.crypto: unpad: %w", err) }
	return plain, nil
}

// DecryptGCM decrypts with AES-GCM using a 16-byte nonce (Zalo non-standard).
func DecryptGCM(key, iv, aad, ct []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil { return nil, fmt.Errorf("zalo.crypto: cipher: %w", err) }

	gcm, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil { return nil, fmt.Errorf("zalo.crypto: gcm: %w", err) }

	plain, err := gcm.Open(nil, iv, ct, aad)
	if err != nil { return nil, fmt.Errorf("zalo.crypto: decrypt: %w", err) }
	return plain, nil
}

func pkcs7Pad(data []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 { return nil, errInvalidBlockSize }
	padLen := blockSize - (len(data) % blockSize)
	if padLen == 0 { padLen = blockSize }
	return append(data, bytes.Repeat([]byte{byte(padLen)}, padLen)...), nil
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 { return nil, errInvalidBlockSize }
	if len(data) == 0 || len(data)%blockSize != 0 { return nil, errInvalidPKCS7Data }
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) { return nil, errInvalidPKCS7Padding }
	if !bytes.Equal(bytes.Repeat([]byte{byte(padLen)}, padLen), data[len(data)-padLen:]) { return nil, errInvalidPKCS7Padding }
	return data[:len(data)-padLen], nil
}
