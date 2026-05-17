// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

import "strings"

// schema.go — Tool JSON Schema normalization for different LLM providers.
// Each provider has quirks: Gemini rejects $ref, OpenAI wants strict mode,
// Anthropic needs refs resolved. This normalizes schemas per-provider.

const maxSchemaDepth = 64

// SchemaProfile controls which normalizations apply to a provider's tool schemas.
type SchemaProfile struct {
	ResolveRefs       bool
	FlattenUnions     bool
	InjectObjectType  bool
	ConvertConst      bool
	StripNullType     bool
	RemoveTypeOnUnion bool
	StripKeys         []string
	StrictToolMode    bool
}

var (
	geminiStripKeys  = []string{"$ref", "$defs", "definitions", "additionalProperties", "patternProperties", "$schema", "$id", "examples", "default", "minLength", "maxLength", "minimum", "maximum", "multipleOf", "pattern", "format", "minItems", "maxItems", "uniqueItems", "minProperties", "maxProperties"}
	xaiStripKeys     = []string{"minLength", "maxLength", "minItems", "maxItems", "minContains", "maxContains"}
	refOnlyStripKeys = []string{"$ref", "$defs", "definitions"}
)

// NormalizeSchema applies provider-specific normalization to a tool's JSON Schema.
func NormalizeSchema(providerName string, schema map[string]any) map[string]any {
	return normalizeWithProfile(profileForProvider(providerName), schema)
}

func normalizeWithProfile(profile SchemaProfile, schema map[string]any) map[string]any {
	if schema == nil { return nil }
	result := copySchema(schema)
	if profile.ResolveRefs { defs := collectDefs(result); result = resolveRefs(result, defs, nil, 0); defs2 := collectDefs(result); if len(defs2) > 0 { result = resolveRefs(result, defs2, nil, 0) } }
	if profile.StripNullType { result = stripNullVariants(result, 0) }
	if profile.FlattenUnions { result = flattenUnions(result, 0) }
	if profile.ConvertConst { result = convertConst(result, 0) }
	if profile.InjectObjectType { result = injectObjectType(result, 0) }
	if profile.RemoveTypeOnUnion { result = removeTypeOnUnion(result, 0) }
	if len(profile.StripKeys) > 0 { result = stripKeys(result, profile.StripKeys, 0) }
	if profile.StrictToolMode { result = applyStrictMode(result, 0) }
	return result
}

func profileForProvider(name string) SchemaProfile {
	switch {
	case name == "anthropic":
		return SchemaProfile{ResolveRefs: true, StripKeys: refOnlyStripKeys}
	case strings.Contains(strings.ToLower(name), "gemini"):
		return SchemaProfile{ResolveRefs: true, FlattenUnions: true, ConvertConst: true, StripNullType: true, RemoveTypeOnUnion: true, StripKeys: geminiStripKeys}
	case name == "xai" || strings.HasPrefix(name, "xai-"):
		return SchemaProfile{ResolveRefs: true, FlattenUnions: true, InjectObjectType: true, StripKeys: xaiStripKeys}
	case name == "openai" || name == "codex" || strings.Contains(strings.ToLower(name), "chatgpt"):
		return SchemaProfile{ResolveRefs: true, FlattenUnions: true, InjectObjectType: true, StrictToolMode: true, StripKeys: refOnlyStripKeys}
	default:
		return SchemaProfile{ResolveRefs: true, FlattenUnions: true, InjectObjectType: true, StripKeys: refOnlyStripKeys}
	}
}

// CleanToolSchemas normalizes tool schemas for a specific provider (batch).
func CleanToolSchemas(providerName string, tools []ToolDefinition) []ToolDefinition {
	if len(tools) == 0 { return tools }
	profile := profileForProvider(providerName)
	cleaned := make([]ToolDefinition, len(tools))
	for i, t := range tools {
		useStrict := profile.StrictToolMode && !IsMultiActionSchema(t.Function.Parameters)
		var strictPtr *bool
		if useStrict { tr := true; strictPtr = &tr }
		toolProfile := profile
		toolProfile.StrictToolMode = useStrict
		cleaned[i] = ToolDefinition{
			Type: t.Type,
			Function: ToolFunctionSchema{
				Name: t.Function.Name, Description: t.Function.Description,
				Parameters: normalizeWithProfile(toolProfile, t.Function.Parameters),
				Strict:     strictPtr,
			},
		}
	}
	return cleaned
}

// CleanSchemaForProvider normalizes a single tool's parameters.
func CleanSchemaForProvider(providerName string, params map[string]any) map[string]any {
	return NormalizeSchema(providerName, params)
}

// --- $ref resolution ---

func collectDefs(schema map[string]any) map[string]any {
	defs := make(map[string]any)
	for _, key := range []string{"$defs", "definitions"} {
		if block, ok := schema[key].(map[string]any); ok {
			for name, def := range block { defs[name] = def }
		}
	}
	return defs
}

func resolveRefs(schema map[string]any, defs map[string]any, visited map[string]bool, depth int) map[string]any {
	if schema == nil || depth > maxSchemaDepth { return schema }
	if ref, ok := schema["$ref"].(string); ok {
		if visited[ref] { return map[string]any{"type": "object", "description": "circular reference"} }
		name := refName(ref)
		if resolved, ok := defs[name]; ok {
			if m, ok := resolved.(map[string]any); ok {
				next := copyVisited(visited); next[ref] = true
				out := resolveRefs(copySchema(m), defs, next, depth+1)
				copyMeta(schema, out)
				return out
			}
		}
		out := make(map[string]any); copyMeta(schema, out); return out
	}
	return walkSchema(schema, func(child map[string]any) map[string]any { return resolveRefs(child, defs, visited, depth+1) })
}

// --- Null variant stripping ---

func stripNullVariants(schema map[string]any, depth int) map[string]any {
	if schema == nil || depth > maxSchemaDepth { return schema }
	for _, key := range []string{"anyOf", "oneOf"} {
		variants, ok := schema[key].([]any)
		if !ok || len(variants) == 0 { continue }
		nonNull := make([]any, 0, len(variants))
		for _, v := range variants {
			if m, ok := v.(map[string]any); ok && isNullSchema(m) { continue }
			nonNull = append(nonNull, v)
		}
		if len(nonNull) == len(variants) { continue }
		if len(nonNull) == 1 {
			if m, ok := nonNull[0].(map[string]any); ok {
				merged := copySchema(m); copyMeta(schema, merged)
				return stripNullVariants(merged, depth+1)
			}
		}
		schema[key] = nonNull
	}
	if typeArr, ok := schema["type"].([]any); ok {
		filtered := make([]any, 0, len(typeArr))
		for _, t := range typeArr { if s, ok := t.(string); ok && s == "null" { continue }; filtered = append(filtered, t) }
		if len(filtered) == 1 { schema["type"] = filtered[0] } else if len(filtered) != len(typeArr) { schema["type"] = filtered }
	}
	return walkSchema(schema, func(child map[string]any) map[string]any { return stripNullVariants(child, depth+1) })
}

// --- Union flattening ---

func flattenUnions(schema map[string]any, depth int) map[string]any {
	if schema == nil || depth > maxSchemaDepth { return schema }
	for _, key := range []string{"anyOf", "oneOf"} {
		variants, ok := schema[key].([]any)
		if !ok { continue }
		if merged, ok := mergeObjectVariants(variants); ok {
			for k, v := range merged { schema[k] = v }
			delete(schema, key)
			return flattenUnions(schema, depth+1)
		}
		if flat, ok := flattenLiterals(variants); ok {
			for k, v := range flat { schema[k] = v }
			delete(schema, key)
			return schema
		}
	}
	return walkSchema(schema, func(child map[string]any) map[string]any { return flattenUnions(child, depth+1) })
}

func flattenLiterals(variants []any) (map[string]any, bool) {
	var enums []any
	var typ string
	for _, v := range variants {
		m, ok := v.(map[string]any)
		if !ok { return nil, false }
		if c, ok := m["const"]; ok { enums = append(enums, c); if t := inferType(c); t != "" { typ = t }; continue }
		if e, ok := m["enum"].([]any); ok { enums = append(enums, e...); continue }
		return nil, false
	}
	if len(enums) == 0 { return nil, false }
	result := map[string]any{"enum": enums}
	if typ != "" { result["type"] = typ }
	return result, true
}

func mergeObjectVariants(variants []any) (map[string]any, bool) {
	allProps := make(map[string]any)
	allRequired := make(map[string]bool)
	for _, v := range variants {
		m, ok := v.(map[string]any)
		if !ok { return nil, false }
		t, _ := m["type"].(string)
		if t != "object" && t != "" { return nil, false }
		if props, ok := m["properties"].(map[string]any); ok {
			for k, v := range props { allProps[k] = v }
		}
		if req, ok := m["required"].([]any); ok {
			for _, r := range req { if s, ok := r.(string); ok { allRequired[s] = true } }
		}
	}
	if len(allProps) == 0 { return nil, false }
	reqArr := make([]any, 0, len(allRequired))
	for k := range allRequired { reqArr = append(reqArr, k) }
	return map[string]any{"type": "object", "properties": allProps, "required": reqArr}, true
}

// --- Other transforms ---

func convertConst(schema map[string]any, depth int) map[string]any {
	if schema == nil || depth > maxSchemaDepth { return schema }
	if c, ok := schema["const"]; ok {
		schema["enum"] = []any{c}; delete(schema, "const")
		if _, hasType := schema["type"]; !hasType { if t := inferType(c); t != "" { schema["type"] = t } }
	}
	return walkSchema(schema, func(child map[string]any) map[string]any { return convertConst(child, depth+1) })
}

func injectObjectType(schema map[string]any, depth int) map[string]any {
	if schema == nil || depth > maxSchemaDepth { return schema }
	if _, hasType := schema["type"]; !hasType {
		if _, hasProps := schema["properties"]; hasProps { schema["type"] = "object" }
	}
	return walkSchema(schema, func(child map[string]any) map[string]any { return injectObjectType(child, depth+1) })
}

func removeTypeOnUnion(schema map[string]any, depth int) map[string]any {
	if schema == nil || depth > maxSchemaDepth { return schema }
	for _, key := range []string{"anyOf", "oneOf"} {
		if _, ok := schema[key]; ok { delete(schema, "type"); break }
	}
	return walkSchema(schema, func(child map[string]any) map[string]any { return removeTypeOnUnion(child, depth+1) })
}

func stripKeys(schema map[string]any, keys []string, depth int) map[string]any {
	if schema == nil || depth > maxSchemaDepth { return schema }
	for _, k := range keys { delete(schema, k) }
	return walkSchema(schema, func(child map[string]any) map[string]any { return stripKeys(child, keys, depth+1) })
}

// --- Strict mode ---

func applyStrictMode(schema map[string]any, depth int) map[string]any {
	if schema == nil || depth > maxSchemaDepth { return schema }
	typ, _ := schema["type"].(string)
	props, hasProps := schema["properties"].(map[string]any)
	if typ != "object" || !hasProps { return schema }

	reqSet := make(map[string]bool)
	if reqArr, ok := schema["required"].([]any); ok { for _, r := range reqArr { if s, ok := r.(string); ok { reqSet[s] = true } } }

	allRequired := make([]any, 0, len(props))
	for name, prop := range props {
		allRequired = append(allRequired, name)
		pm, ok := prop.(map[string]any)
		if !ok { continue }
		pm = applyStrictMode(pm, depth+1)
		if items, ok := pm["items"].(map[string]any); ok { pm["items"] = applyStrictMode(items, depth+1) }
		props[name] = pm
		if !reqSet[name] { makeNullable(pm) }
	}
	schema["properties"] = props
	schema["required"] = allRequired
	schema["additionalProperties"] = false
	return schema
}

// IsMultiActionSchema detects tools with action enum (2+ values) — exempt from strict mode.
func IsMultiActionSchema(schema map[string]any) bool {
	props, ok := schema["properties"].(map[string]any)
	if !ok { return false }
	ap, ok := props["action"].(map[string]any)
	if !ok { return false }
	switch e := ap["enum"].(type) {
	case []string: return len(e) >= 2
	case []any: return len(e) >= 2
	}
	return false
}

func makeNullable(schema map[string]any) {
	for _, key := range []string{"anyOf", "oneOf"} {
		if variants, ok := schema[key].([]any); ok {
			for _, v := range variants { if m, ok := v.(map[string]any); ok && isNullSchema(m) { return } }
			schema[key] = append(variants, map[string]any{"type": "null"}); return
		}
	}
	switch t := schema["type"].(type) {
	case string:
		if t != "null" { schema["type"] = []any{t, "null"} }
	case []any:
		for _, v := range t { if s, ok := v.(string); ok && s == "null" { return } }
		schema["type"] = append(t, "null")
	default:
		schema["type"] = []any{"object", "null"}
	}
}

// --- Helpers ---

func walkSchema(schema map[string]any, fn func(map[string]any) map[string]any) map[string]any {
	if props, ok := schema["properties"].(map[string]any); ok {
		cleaned := make(map[string]any, len(props))
		for k, v := range props { if m, ok := v.(map[string]any); ok { cleaned[k] = fn(m) } else { cleaned[k] = v } }
		schema["properties"] = cleaned
	}
	if items, ok := schema["items"].(map[string]any); ok { schema["items"] = fn(items) }
	if ap, ok := schema["additionalProperties"].(map[string]any); ok { schema["additionalProperties"] = fn(ap) }
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if arr, ok := schema[key].([]any); ok { for i, item := range arr { if m, ok := item.(map[string]any); ok { arr[i] = fn(m) } } }
	}
	if notSchema, ok := schema["not"].(map[string]any); ok { schema["not"] = fn(notSchema) }
	return schema
}

func copySchema(schema map[string]any) map[string]any {
	if schema == nil { return nil }
	result := make(map[string]any, len(schema))
	for k, v := range schema {
		switch val := v.(type) {
		case map[string]any: result[k] = copySchema(val)
		case []any:
			cp := make([]any, len(val))
			for i, item := range val { if m, ok := item.(map[string]any); ok { cp[i] = copySchema(m) } else { cp[i] = item } }
			result[k] = cp
		default: result[k] = v
		}
	}
	return result
}

func copyVisited(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m)+1)
	for k, v := range m { out[k] = v }
	return out
}

func copyMeta(src, dst map[string]any) {
	for _, key := range []string{"description", "title"} { if v, ok := src[key]; ok { if _, exists := dst[key]; !exists { dst[key] = v } } }
}

func refName(ref string) string {
	for _, prefix := range []string{"#/$defs/", "#/definitions/"} { if after, ok := strings.CutPrefix(ref, prefix); ok { return after } }
	return ""
}

func isNullSchema(schema map[string]any) bool {
	if t, ok := schema["type"].(string); ok && t == "null" { return true }
	if c, ok := schema["const"]; ok && c == nil { return true }
	if e, ok := schema["enum"].([]any); ok && len(e) == 1 && e[0] == nil { return true }
	return false
}

func inferType(val any) string {
	switch val.(type) {
	case string: return "string"
	case float64, float32, int, int64: return "number"
	case bool: return "boolean"
	default: return ""
	}
}
