// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package auth

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// User represents a Qorven user account.
type User struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name,omitempty"`
	Email       string    `json:"email,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	Role        string    `json:"role"` // admin, user, viewer
	IsActive    bool      `json:"is_active"`
	LastLoginAt time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// AuthService handles user authentication.
type AuthService struct {
	pool      *pgxpool.Pool
	jwtSecret []byte
}

// NewAuthService creates the auth service, loading or generating the JWT secret.
func NewAuthService(pool *pgxpool.Pool) *AuthService {
	secret := loadOrCreateSecret()
	return &AuthService{pool: pool, jwtSecret: secret}
}

// SetupRequired returns true if no users exist (first run).
// Also returns true if the users table doesn't exist yet (migrations
// haven't run), so the setup wizard always shows on a blank database.
func (s *AuthService) SetupRequired(ctx context.Context) bool {
	var count int
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count)
	if err != nil {
		// Table missing or other DB error — treat as setup required.
		return true
	}
	return count == 0
}

// CreateUser creates a new user with bcrypt-hashed password.
func (s *AuthService) CreateUser(ctx context.Context, username, password, email, role, tenantID string) (*User, error) {
	return s.CreateUserWithDisplay(ctx, username, password, email, "", role, tenantID)
}

// CreateUserWithDisplay creates a new user and stores an optional display name.
func (s *AuthService) CreateUserWithDisplay(ctx context.Context, username, password, email, displayName, role, tenantID string) (*User, error) {
	if len(password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return nil, err
	}
	id := uuid.New().String()
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}
	_, err = s.pool.Exec(ctx,
		`INSERT INTO users (id, tenant_id, username, email, display_name, password_hash, role)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, tenantID, strings.ToLower(username), email, displayName, string(hash), role)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &User{ID: id, TenantID: tenantID, Username: username, DisplayName: displayName, Email: email, Role: role, IsActive: true, CreatedAt: time.Now()}, nil
}

// Login verifies credentials and returns a JWT token.
func (s *AuthService) Login(ctx context.Context, username, password string) (string, *User, error) {
	var user User
	var passwordHash string
	var failedLogins int
	var lockedUntil *time.Time
	var email *string

	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, username, email, password_hash, role, is_active, failed_logins, locked_until
		 FROM users WHERE username = $1`, strings.ToLower(username),
	).Scan(&user.ID, &user.TenantID, &user.Username, &email, &passwordHash, &user.Role, &user.IsActive, &failedLogins, &lockedUntil)
	if err != nil {
		return "", nil, fmt.Errorf("invalid credentials")
	}
	if email != nil {
		user.Email = *email
	}

	if !user.IsActive {
		return "", nil, fmt.Errorf("account disabled")
	}
	if lockedUntil != nil && time.Now().Before(*lockedUntil) {
		return "", nil, fmt.Errorf("account locked until %s", lockedUntil.Format(time.RFC3339))
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		// Increment failed logins
		newFailed := failedLogins + 1
		if newFailed >= 5 {
			lockUntil := time.Now().Add(15 * time.Minute)
			s.pool.Exec(ctx, "UPDATE users SET failed_logins = $1, locked_until = $2 WHERE id = $3", newFailed, lockUntil, user.ID)
		} else {
			s.pool.Exec(ctx, "UPDATE users SET failed_logins = $1 WHERE id = $2", newFailed, user.ID)
		}
		return "", nil, fmt.Errorf("invalid credentials")
	}

	// Reset failed logins on success
	s.pool.Exec(ctx, "UPDATE users SET failed_logins = 0, locked_until = NULL, last_login_at = now() WHERE id = $1", user.ID)

	return s.IssueToken(&user), &user, nil
}

// JWTLifetime is the short-lived access-token window. 24h is long enough
// for a workday without re-login but short enough that a leaked token
// can't be used indefinitely — the refresh-token flow issues a new JWT
// when the client hits 401, preserving the single-sign-in UX.
const JWTLifetime = 24 * time.Hour

// RefreshTokenLifetime controls how long the persisted refresh token
// stays valid. Beyond this the user must log in with their password
// again. Matches the Phase 3 spec (7 days).
const RefreshTokenLifetime = 7 * 24 * time.Hour

// IssueToken generates a new 24h JWT for an already-authenticated user.
func (s *AuthService) IssueToken(user *User) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.ID, "username": user.Username, "role": user.Role, "tenant": user.TenantID,
		"iat": time.Now().Unix(), "exp": time.Now().Add(JWTLifetime).Unix(),
	})
	str, _ := token.SignedString(s.jwtSecret)
	return str
}

// IssueRefreshToken creates a persisted refresh token. Returned string
// is opaque; the caller sends it to the frontend which stores it in an
// httpOnly cookie and exchanges it at /auth/refresh.
func (s *AuthService) IssueRefreshToken(ctx context.Context, userID, userAgent, ipAddress string) (string, error) {
	tok, err := randomToken(32)
	if err != nil { return "", err }
	_, err = s.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token, expires_at, user_agent, ip_address, last_used_at)
		 VALUES ($1, $2, $3, $4, $5, now())`,
		userID, tok, time.Now().Add(RefreshTokenLifetime), userAgent, ipAddress)
	if err != nil { return "", err }
	return tok, nil
}

// RevokeSessionByID revokes a refresh token owned by the given user.
func (s *AuthService) RevokeSessionByID(ctx context.Context, userID, tokenID string) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		tokenID, userID)
	if err != nil { return err }
	if ct.RowsAffected() == 0 { return fmt.Errorf("session not found") }
	return nil
}

// ValidateRefreshToken returns (userID, error). Fails for missing,
// expired, or revoked tokens. Used by the /auth/refresh handler to
// mint a fresh JWT without re-prompting the user for a password.
func (s *AuthService) ValidateRefreshToken(ctx context.Context, token string) (*User, error) {
	var userID string
	var expiresAt time.Time
	var revokedAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, expires_at, revoked_at FROM refresh_tokens WHERE token = $1`, token,
	).Scan(&userID, &expiresAt, &revokedAt)
	if err != nil { return nil, fmt.Errorf("invalid refresh token") }
	if revokedAt != nil { return nil, fmt.Errorf("refresh token revoked") }
	if time.Now().After(expiresAt) { return nil, fmt.Errorf("refresh token expired") }

	var user User
	var emailR *string
	err = s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, username, email, role, is_active FROM users WHERE id = $1`, userID,
	).Scan(&user.ID, &user.TenantID, &user.Username, &emailR, &user.Role, &user.IsActive)
	if emailR != nil { user.Email = *emailR }
	if err != nil || !user.IsActive { return nil, fmt.Errorf("user not found or inactive") }
	s.pool.Exec(ctx, `UPDATE refresh_tokens SET last_used_at = now() WHERE token = $1`, token)
	return &user, nil
}

// RevokeRefreshToken marks a refresh token revoked. Used on logout so
// a stolen cookie can't keep minting access tokens.
func (s *AuthService) RevokeRefreshToken(ctx context.Context, token string) {
	s.pool.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE token = $1 AND revoked_at IS NULL`, token)
}

// ─── Magic-link password reset ──────────────────────────────────────────

// CreateMagicLink generates a one-time token for password reset, stores
// it with a 15-minute expiry, and returns (token, user). Delivery is the
// caller's responsibility (email / telegram / log). Looks up the user
// by either username or email so the forgot-password form can accept
// either identifier.
func (s *AuthService) CreateMagicLink(ctx context.Context, identifier, delivery string) (string, *User, error) {
	var user User
	var emailML *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, username, email, role, is_active
		 FROM users WHERE username = $1 OR email = $1
		 LIMIT 1`, strings.ToLower(identifier),
	).Scan(&user.ID, &user.TenantID, &user.Username, &emailML, &user.Role, &user.IsActive)
	if emailML != nil { user.Email = *emailML }
	if err != nil { return "", nil, fmt.Errorf("user not found") }
	if !user.IsActive { return "", nil, fmt.Errorf("account disabled") }

	tok, err := randomToken(32)
	if err != nil { return "", nil, err }
	if delivery == "" { delivery = "email" }

	_, err = s.pool.Exec(ctx,
		`INSERT INTO magic_links (user_id, token, delivery, expires_at) VALUES ($1, $2, $3, $4)`,
		user.ID, tok, delivery, time.Now().Add(15*time.Minute))
	if err != nil { return "", nil, err }
	return tok, &user, nil
}

// ConsumeMagicLink resolves a magic-link token to its user. The token
// is marked used atomically so it can't be reused even in a race.
func (s *AuthService) ConsumeMagicLink(ctx context.Context, token string) (*User, error) {
	var userID string
	err := s.pool.QueryRow(ctx,
		`UPDATE magic_links
		   SET used = true
		 WHERE token = $1 AND used = false AND expires_at > now()
		 RETURNING user_id`, token,
	).Scan(&userID)
	if err != nil { return nil, fmt.Errorf("invalid or expired token") }

	var user User
	var emailCML *string
	err = s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, username, email, role, is_active FROM users WHERE id = $1`, userID,
	).Scan(&user.ID, &user.TenantID, &user.Username, &emailCML, &user.Role, &user.IsActive)
	if emailCML != nil { user.Email = *emailCML }
	if err != nil || !user.IsActive { return nil, fmt.Errorf("user not found or inactive") }
	return &user, nil
}

// ResetPassword sets a new password hash for the given user. Used by the
// /auth/reset-password endpoint after ConsumeMagicLink succeeds. Also
// clears failed_logins / locked_until so a user recovering a forgotten
// password isn't blocked by a prior lockout.
//
// Uses bcrypt to match CreateUser + Login. The argon2id helpers in
// argon2id.go are a pending migration — once Login is switched to the
// IsLegacyHash + VerifyPassword fallback path, swap this to
// HashPassword(newPassword) in the same commit.
func (s *AuthService) ResetPassword(ctx context.Context, userID, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil { return err }
	_, err = s.pool.Exec(ctx,
		`UPDATE users SET password_hash = $1, failed_logins = 0, locked_until = NULL, updated_at = now() WHERE id = $2`,
		string(hash), userID)
	return err
}

// randomToken returns a URL-safe random string of n bytes' entropy.
func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := cryptorand.Read(b); err != nil { return "", err }
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateOTP generates a 6-digit one-time code for password reset, stores it
// in magic_links (reusing the same table), and returns (otp, resetToken, user).
// The resetToken is a long random string returned only after VerifyOTP succeeds —
// it is NOT returned here. The OTP itself is what gets delivered to the user.
func (s *AuthService) CreateOTP(ctx context.Context, identifier string) (otp string, user *User, err error) {
	var u User
	var emailOTP *string
	err = s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, username, email, role, is_active
		 FROM users WHERE username = $1 OR email = $1 LIMIT 1`,
		strings.ToLower(identifier),
	).Scan(&u.ID, &u.TenantID, &u.Username, &emailOTP, &u.Role, &u.IsActive)
	if emailOTP != nil { u.Email = *emailOTP }
	if err != nil { return "", nil, fmt.Errorf("user not found") }
	if !u.IsActive { return "", nil, fmt.Errorf("account disabled") }

	// 6-digit numeric OTP
	b := make([]byte, 4)
	cryptorand.Read(b)
	n := int(b[0])<<24 | int(b[1])<<16 | int(b[2])<<8 | int(b[3])
	if n < 0 { n = -n }
	code := fmt.Sprintf("%06d", n%1000000)

	// Invalidate any previous unused OTPs for this user
	s.pool.Exec(ctx, `UPDATE magic_links SET used = true WHERE user_id = $1 AND used = false`, u.ID)

	_, err = s.pool.Exec(ctx,
		`INSERT INTO magic_links (user_id, token, delivery, expires_at) VALUES ($1, $2, 'otp', now() + interval '15 minutes')`,
		u.ID, code)
	if err != nil { return "", nil, err }

	return code, &u, nil
}

// VerifyOTP checks a 6-digit OTP and, if valid, marks it used and returns a
// short-lived reset token (valid 5 minutes) that the client passes to ResetPassword.
func (s *AuthService) VerifyOTP(ctx context.Context, identifier, otp string) (resetToken string, err error) {
	var userID string
	err = s.pool.QueryRow(ctx,
		`UPDATE magic_links SET used = true
		 WHERE token = $1 AND used = false AND expires_at > now()
		   AND user_id = (SELECT id FROM users WHERE username = $2 OR email = $2 LIMIT 1)
		 RETURNING user_id`,
		otp, strings.ToLower(identifier),
	).Scan(&userID)
	if err != nil { return "", fmt.Errorf("invalid or expired code") }

	tok, err := randomToken(24)
	if err != nil { return "", err }
	_, err = s.pool.Exec(ctx,
		`INSERT INTO magic_links (user_id, token, delivery, expires_at) VALUES ($1, $2, 'reset', now() + interval '5 minutes')`,
		userID, tok)
	if err != nil { return "", err }
	return tok, nil
}

func (s *AuthService) ValidateToken(tokenStr string) (*User, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}

	return &User{
		ID:       claims["sub"].(string),
		Username: claims["username"].(string),
		Role:     claims["role"].(string),
		TenantID: claims["tenant"].(string),
	}, nil
}

// CreateAPIKey generates a new API key for a user.
func (s *AuthService) CreateAPIKey(ctx context.Context, userID, name string) (string, error) {
	// Generate key: qk_ prefix + 32 random bytes
	raw := make([]byte, 32)
	cryptorand.Read(raw)
	key := "qk_" + hex.EncodeToString(raw)

	// Store hash
	hash := sha256.Sum256([]byte(key))
	hashStr := hex.EncodeToString(hash[:])

	_, err := s.pool.Exec(ctx,
		`INSERT INTO api_keys (id, user_id, name, key_hash) VALUES ($1, $2, $3, $4)`,
		uuid.New().String(), userID, name, hashStr)
	if err != nil {
		return "", err
	}

	return key, nil // Return plaintext key only once
}

// ChangePassword verifies the current password then updates to the new one.
func (s *AuthService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	var oldHash string
	err := s.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&oldHash)
	if err != nil {
		return fmt.Errorf("user not found")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(oldHash), []byte(currentPassword)); err != nil {
		return fmt.Errorf("current password is incorrect")
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, string(newHash), userID)
	return err
}

// ValidateAPIKey checks if an API key is valid.
func (s *AuthService) ValidateAPIKey(ctx context.Context, key string) (*User, error) {
	hash := sha256.Sum256([]byte(key))
	hashStr := hex.EncodeToString(hash[:])

	var userID string
	err := s.pool.QueryRow(ctx,
		`SELECT user_id FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL`, hashStr,
	).Scan(&userID)
	if err != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	var user User
	var emailAK *string
	err = s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, username, email, role FROM users WHERE id = $1 AND is_active = true`, userID,
	).Scan(&user.ID, &user.TenantID, &user.Username, &emailAK, &user.Role)
	if emailAK != nil { user.Email = *emailAK }
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}

	// Update last used
	s.pool.Exec(ctx, "UPDATE api_keys SET last_used_at = now(), usage_count = usage_count + 1 WHERE key_hash = $1", hashStr)

	return &user, nil
}

// loadOrCreateSecret loads JWT secret from disk or generates a new one.
func loadOrCreateSecret() []byte {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".qorven", "jwt_secret")

	data, err := os.ReadFile(path)
	if err == nil && len(data) >= 32 {
		return data
	}

	// Generate new secret
	secret := make([]byte, 64)
	cryptorand.Read(secret)

	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, secret, 0600)
	slog.Info("auth: generated new JWT secret")

	return secret
}
