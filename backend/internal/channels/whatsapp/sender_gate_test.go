package whatsapp

import (
	"testing"
)

func TestGenerateOTP_SixDigits(t *testing.T) {
	otp := generateOTP()
	if len(otp) != 6 {
		t.Errorf("expected 6-digit OTP, got %q (len %d)", otp, len(otp))
	}
	for _, ch := range otp {
		if ch < '0' || ch > '9' {
			t.Errorf("OTP contains non-digit: %q", otp)
		}
	}
}

func TestOTPChallenge_CorrectCode_Passes(t *testing.T) {
	gate := newSenderGate()
	otp := gate.challenge("91XX@s.whatsapp.net", "Jay", "hello world")
	result := gate.verify("91XX@s.whatsapp.net", otp)
	if result != otpVerifyApproved {
		t.Errorf("expected approved, got %v", result)
	}
}

func TestOTPChallenge_WrongCode_ThreeAttempts_LocksOut(t *testing.T) {
	gate := newSenderGate()
	gate.challenge("91XX@s.whatsapp.net", "Jay", "hello")
	gate.verify("91XX@s.whatsapp.net", "000000")
	gate.verify("91XX@s.whatsapp.net", "000000")
	result := gate.verify("91XX@s.whatsapp.net", "000000")
	if result != otpVerifyLockedOut {
		t.Errorf("expected locked out after 3 failures, got %v", result)
	}
}

func TestOTPChallenge_AlreadyPending_ReturnsPending(t *testing.T) {
	gate := newSenderGate()
	gate.challenge("91XX@s.whatsapp.net", "Jay", "msg1")
	otp2 := gate.challenge("91XX@s.whatsapp.net", "Jay", "msg2")
	if otp2 != "" {
		t.Errorf("second challenge should return empty (already pending), got %q", otp2)
	}
}
