// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package providers

// CFOModelMeta is an enabled model joined to its per-1M USD pricing, for the CFO planner.
// Named CFOModelMeta to avoid collision with the existing ModelMeta catalog type.
type CFOModelMeta struct {
	ID          string  `json:"id"`
	InputPer1M  float64 `json:"input_per_m"`
	OutputPer1M float64 `json:"output_per_m"`
}

// joinEnabledMeta is the pure join (testable without DB): enabled ids × rates.
// rates maps model id → [input_per_1m, output_per_1m]. Unknown ids get zero rates;
// the caller decides how to treat pricing-missing.
func joinEnabledMeta(enabled map[string]bool, rates map[string][2]float64) []CFOModelMeta {
	out := []CFOModelMeta{}
	for id := range enabled {
		r := rates[id]
		out = append(out, CFOModelMeta{ID: id, InputPer1M: r[0], OutputPer1M: r[1]})
	}
	return out
}
