package budgets

import "testing"

func TestProjectFeasibility(t *testing.T) {
	// budget $1000, spent $200, committed $300, daily burn $10/day, 10 days left.
	// projected_burn = 100; available = 1000-200-300-100 = 400.
	const m = int64(1_000_000)
	f := ProjectFeasibility(1000*m, 200*m, 300*m, 10*m, 10, 350*m)
	if f.ProjectedBurnUUSD != 100*m {
		t.Errorf("projected burn: want %d got %d", 100*m, f.ProjectedBurnUUSD)
	}
	if f.AvailableUUSD != 400*m {
		t.Errorf("available: want %d got %d", 400*m, f.AvailableUUSD)
	}
	if !f.Fits {
		t.Errorf("plan $350 should fit in $400 available")
	}

	// A $500 plan does NOT fit in $400 available.
	f2 := ProjectFeasibility(1000*m, 200*m, 300*m, 10*m, 10, 500*m)
	if f2.Fits {
		t.Errorf("plan $500 should NOT fit in $400 available")
	}

	// Over-committed: available goes negative, nothing fits.
	f3 := ProjectFeasibility(100*m, 80*m, 50*m, 0, 0, 1)
	if f3.AvailableUUSD != -30*m {
		t.Errorf("available: want %d got %d", -30*m, f3.AvailableUUSD)
	}
	if f3.Fits {
		t.Errorf("nothing fits when available is negative")
	}
}
