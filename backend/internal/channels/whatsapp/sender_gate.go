package whatsapp

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"
)

type otpVerifyResult int

const (
	otpVerifyApproved  otpVerifyResult = iota
	otpVerifyWrong
	otpVerifyLockedOut
	otpVerifyNotFound
)

const (
	maxOTPAttempts  = 3
	otpLockDuration = 5 * time.Minute
)

type pendingEntry struct {
	senderJID   string
	displayName string
	origMessage string
	otp         string
	attempts    int
	lockedUntil time.Time
}

// senderGate manages in-memory OTP challenges for unknown senders.
type senderGate struct {
	mu      sync.Mutex
	pending map[string]*pendingEntry // keyed by senderJID
}

func newSenderGate() *senderGate {
	return &senderGate{pending: make(map[string]*pendingEntry)}
}

// challenge creates or retrieves an OTP for senderJID.
// Returns the OTP string if newly created, or "" if already pending.
func (g *senderGate) challenge(senderJID, displayName, origMessage string) string {
	g.mu.Lock()
	defer g.mu.Unlock()

	if e, exists := g.pending[senderJID]; exists {
		if e.lockedUntil.IsZero() || time.Now().After(e.lockedUntil) {
			e.origMessage = origMessage
		}
		return ""
	}

	otp := generateOTP()
	g.pending[senderJID] = &pendingEntry{
		senderJID:   senderJID,
		displayName: displayName,
		origMessage: origMessage,
		otp:         otp,
	}
	return otp
}

// verify checks if the submitted code matches. Returns the result.
func (g *senderGate) verify(senderJID, submittedCode string) otpVerifyResult {
	g.mu.Lock()
	defer g.mu.Unlock()

	e, exists := g.pending[senderJID]
	if !exists {
		return otpVerifyNotFound
	}

	if !e.lockedUntil.IsZero() && time.Now().Before(e.lockedUntil) {
		return otpVerifyLockedOut
	}

	if submittedCode == e.otp {
		delete(g.pending, senderJID)
		return otpVerifyApproved
	}

	e.attempts++
	if e.attempts >= maxOTPAttempts {
		e.lockedUntil = time.Now().Add(otpLockDuration)
		return otpVerifyLockedOut
	}
	return otpVerifyWrong
}

// popOrigMessage returns and removes the original message for a pending sender.
func (g *senderGate) popOrigMessage(senderJID string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if e, ok := g.pending[senderJID]; ok {
		msg := e.origMessage
		delete(g.pending, senderJID)
		return msg
	}
	return ""
}

// isPending reports whether senderJID has a pending OTP.
func (g *senderGate) isPending(senderJID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.pending[senderJID]
	return ok
}

// generateOTP returns a cryptographically random 6-digit string.
func generateOTP() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	return fmt.Sprintf("%06d", n.Int64())
}
