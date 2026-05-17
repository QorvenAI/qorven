// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

// Exported versions of the common argument-coercion helpers so tools
// living in other packages (internal/qor/browser/primitive_tools.go,
// etc.) can parse map[string]any args the same way the built-in
// tools do. Kept as thin wrappers — no behavior change.

// ToInt coerces a map[string]any value (which arrives as float64
// from JSON decoding) to an int. Returns ok=false when the value
// is missing or the type can't be cast. Mirrors toInt.
func ToInt(v any) (int, bool) { return toInt(v) }

// ToFloat coerces a map[string]any value to a float64 with the same
// semantics as ToInt. Mirrors toFloat.
func ToFloat(v any) (float64, bool) { return toFloat(v) }
