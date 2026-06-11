package providers

import "testing"

func TestModelMetaPricingJoin(t *testing.T) {
	enabled := map[string]bool{"claude-sonnet-4-6": true}
	rates := map[string][2]float64{"claude-sonnet-4-6": {3.0, 15.0}}
	out := joinEnabledMeta(enabled, rates)
	if len(out) != 1 { t.Fatalf("want 1 got %d", len(out)) }
	if out[0].ID != "claude-sonnet-4-6" || out[0].InputPer1M != 3.0 || out[0].OutputPer1M != 15.0 {
		t.Errorf("bad meta: %+v", out[0])
	}
}

func TestModelMetaUnknownPrice(t *testing.T) {
	// an enabled model with no rate entry → zero rates (caller flags missing)
	out := joinEnabledMeta(map[string]bool{"mystery": true}, map[string][2]float64{})
	if len(out) != 1 || out[0].InputPer1M != 0 || out[0].OutputPer1M != 0 {
		t.Errorf("unknown-priced model should have zero rates: %+v", out)
	}
}
