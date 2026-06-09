package budgets

import "testing"

func TestDepartmentDecision(t *testing.T) {
	const thresh = int64(25_000_000) // $25
	cases := []struct {
		name   string
		policy string
		plan   int64
		fits   bool
		want   string
	}{
		{"auto fits → apply", PolicyAuto, 10_000_000, true, "apply"},
		{"auto does not fit → propose", PolicyAuto, 10_000_000, false, "propose"},
		{"user_approval always propose (even fits)", PolicyUserApproval, 1, true, "propose"},
		{"both fits + under threshold → apply", PolicyBoth, 20_000_000, true, "apply"},
		{"both fits but over threshold → propose", PolicyBoth, 30_000_000, true, "propose"},
		{"both under threshold but does not fit → propose", PolicyBoth, 20_000_000, false, "propose"},
		{"unknown policy treated as auto", "weird", 1, true, "apply"},
	}
	for _, c := range cases {
		got := DepartmentDecision(c.policy, thresh, c.plan, c.fits)
		if got != c.want {
			t.Errorf("%s: DepartmentDecision(%q,%d,%d,%v)=%q want %q", c.name, c.policy, thresh, c.plan, c.fits, got, c.want)
		}
	}
}

func TestDefaultPolicyForDepartment(t *testing.T) {
	both := []string{"Engineering", "IT", "code", "Technology", "Dev Team", "Software"}
	for _, n := range both {
		if DefaultPolicyForDepartment(n) != PolicyBoth {
			t.Errorf("department %q should default to 'both'", n)
		}
	}
	auto := []string{"Marketing", "Sales", "Finance", "Support", ""}
	for _, n := range auto {
		if DefaultPolicyForDepartment(n) != PolicyAuto {
			t.Errorf("department %q should default to 'auto_within_budget'", n)
		}
	}
}
