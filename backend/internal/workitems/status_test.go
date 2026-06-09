package workitems

import "testing"

func TestCanTransition_LegalMoves(t *testing.T) {
	legal := [][2]string{
		{StatusOpen, StatusAssigned},
		{StatusAssigned, StatusInProgress},
		{StatusInProgress, StatusBlocked},
		{StatusBlocked, StatusInProgress},
		{StatusInProgress, StatusInReview},
		{StatusInReview, StatusDone},
		{StatusOpen, StatusCancelled},
		{StatusAssigned, StatusCancelled},
		{StatusInProgress, StatusCancelled},
	}
	for _, m := range legal {
		if !CanTransition(m[0], m[1]) {
			t.Errorf("expected %s→%s to be legal", m[0], m[1])
		}
	}
}

func TestCanTransition_IllegalMoves(t *testing.T) {
	illegal := [][2]string{
		{StatusDone, StatusInProgress},
		{StatusCancelled, StatusOpen},
		{StatusOpen, StatusDone},
		{StatusOpen, StatusInReview},
		{StatusDone, StatusCancelled},
		{"bogus", StatusOpen},
		{StatusOpen, "bogus"},
	}
	for _, m := range illegal {
		if CanTransition(m[0], m[1]) {
			t.Errorf("expected %s→%s to be ILLEGAL", m[0], m[1])
		}
	}
}
