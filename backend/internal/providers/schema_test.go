// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"testing"
)

// schema_test.go — Tests for tool JSON Schema normalization.

func TestNormalizeSchema_NilInput(t *testing.T) {
	result := NormalizeSchema("openai", nil)
	if result != nil { t.Errorf("expected nil, got %v", result) }
}

func TestNormalizeSchema_EmptySchema(t *testing.T) {
	result := NormalizeSchema("openai", map[string]any{})
	if result == nil { t.Fatal("expected non-nil") }
}

func TestNormalizeSchema_ResolveRef(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"$ref": "#/$defs/Name"},
		},
		"$defs": map[string]any{
			"Name": map[string]any{"type": "string", "description": "A name"},
		},
	}
	result := NormalizeSchema("anthropic", schema)
	props := result["properties"].(map[string]any)
	nameProp := props["name"].(map[string]any)
	if nameProp["type"] != "string" { t.Errorf("ref not resolved: %v", nameProp) }
	if nameProp["description"] != "A name" { t.Errorf("description lost: %v", nameProp) }
}

func TestNormalizeSchema_CircularRef(t *testing.T) {
	schema := map[string]any{
		"$ref": "#/$defs/Node",
		"$defs": map[string]any{
			"Node": map[string]any{"$ref": "#/$defs/Node"},
		},
	}
	result := NormalizeSchema("openai", schema)
	if result == nil { t.Fatal("should handle circular ref") }
}

func TestNormalizeSchema_StripNullVariant(t *testing.T) {
	schema := map[string]any{
		"anyOf": []any{
			map[string]any{"type": "string"},
			map[string]any{"type": "null"},
		},
	}
	result := NormalizeSchema("gemini", schema)
	if result["type"] != "string" { t.Errorf("null not stripped: %v", result) }
}

func TestNormalizeSchema_StripNullTypeArray(t *testing.T) {
	schema := map[string]any{"type": []any{"string", "null"}}
	result := NormalizeSchema("gemini", schema)
	if result["type"] != "string" { t.Errorf("null type not stripped: %v", result) }
}

func TestNormalizeSchema_FlattenUnionLiterals(t *testing.T) {
	schema := map[string]any{
		"anyOf": []any{
			map[string]any{"const": "a"},
			map[string]any{"const": "b"},
			map[string]any{"const": "c"},
		},
	}
	result := NormalizeSchema("openai", schema)
	enums, ok := result["enum"].([]any)
	if !ok { t.Fatalf("expected enum, got %v", result) }
	if len(enums) != 3 { t.Errorf("expected 3 enums, got %d", len(enums)) }
}

func TestNormalizeSchema_FlattenUnionObjects(t *testing.T) {
	schema := map[string]any{
		"anyOf": []any{
			map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}, "required": []any{"a"}},
			map[string]any{"type": "object", "properties": map[string]any{"b": map[string]any{"type": "integer"}}},
		},
	}
	result := NormalizeSchema("openai", schema)
	props, ok := result["properties"].(map[string]any)
	if !ok { t.Fatalf("expected merged properties, got %v", result) }
	if _, ok := props["a"]; !ok { t.Error("missing property a") }
	if _, ok := props["b"]; !ok { t.Error("missing property b") }
}

func TestNormalizeSchema_ConvertConst(t *testing.T) {
	schema := map[string]any{"const": "hello"}
	result := NormalizeSchema("gemini", schema)
	enums, ok := result["enum"].([]any)
	if !ok { t.Fatalf("const not converted to enum: %v", result) }
	if len(enums) != 1 || enums[0] != "hello" { t.Errorf("wrong enum: %v", enums) }
}

func TestNormalizeSchema_InjectObjectType(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{"name": map[string]any{"type": "string"}},
	}
	result := NormalizeSchema("openai", schema)
	if result["type"] != "object" { t.Errorf("type not injected: %v", result) }
}

func TestNormalizeSchema_GeminiStripKeys(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 100}},
		"additionalProperties": false,
		"$schema": "http://json-schema.org/draft-07/schema#",
	}
	result := NormalizeSchema("gemini", schema)
	if _, ok := result["additionalProperties"]; ok { t.Error("additionalProperties not stripped for Gemini") }
	if _, ok := result["$schema"]; ok { t.Error("$schema not stripped for Gemini") }
	props := result["properties"].(map[string]any)
	nameProp := props["name"].(map[string]any)
	if _, ok := nameProp["minLength"]; ok { t.Error("minLength not stripped for Gemini") }
}

func TestNormalizeSchema_OpenAIStrictMode(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  map[string]any{"type": "string"},
			"email": map[string]any{"type": "string"},
		},
		"required": []any{"name"},
	}
	result := NormalizeSchema("openai", schema)
	// All properties should be required in strict mode
	req, ok := result["required"].([]any)
	if !ok { t.Fatalf("expected required array, got %v", result["required"]) }
	if len(req) != 2 { t.Errorf("expected 2 required, got %d", len(req)) }
	// additionalProperties should be false
	if result["additionalProperties"] != false { t.Error("additionalProperties not set to false") }
	// Optional property (email) should be nullable
	props := result["properties"].(map[string]any)
	emailProp := props["email"].(map[string]any)
	emailType, ok := emailProp["type"].([]any)
	if !ok { t.Fatalf("email type not array: %v", emailProp["type"]) }
	hasNull := false
	for _, t2 := range emailType { if t2 == "null" { hasNull = true } }
	if !hasNull { t.Error("optional email not made nullable") }
}

func TestNormalizeSchema_AnthropicProfile(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"$ref": "#/$defs/Query"},
		},
		"$defs": map[string]any{
			"Query": map[string]any{"type": "string"},
		},
	}
	result := NormalizeSchema("anthropic", schema)
	// Refs should be resolved
	props := result["properties"].(map[string]any)
	queryProp := props["query"].(map[string]any)
	if queryProp["type"] != "string" { t.Errorf("ref not resolved for anthropic: %v", queryProp) }
	// $defs should be stripped
	if _, ok := result["$defs"]; ok { t.Error("$defs not stripped for anthropic") }
}

func TestNormalizeSchema_NestedProperties(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"address": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
			},
		},
	}
	result := NormalizeSchema("openai", schema)
	props := result["properties"].(map[string]any)
	addr := props["address"].(map[string]any)
	if addr["additionalProperties"] != false { t.Error("nested object not strict") }
}

func TestNormalizeSchema_ArrayItems(t *testing.T) {
	schema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"$ref": "#/$defs/Name"},
			},
		},
		"$defs": map[string]any{
			"Name": map[string]any{"type": "string"},
		},
	}
	result := NormalizeSchema("openai", schema)
	items := result["items"].(map[string]any)
	props := items["properties"].(map[string]any)
	nameProp := props["name"].(map[string]any)
	if nameProp["type"] != "string" { t.Errorf("ref in array items not resolved: %v", nameProp) }
}

func TestCleanToolSchemas_Empty(t *testing.T) {
	result := CleanToolSchemas("openai", nil)
	if result != nil { t.Errorf("expected nil, got %v", result) }
}

func TestCleanToolSchemas_MultiAction_ExemptFromStrict(t *testing.T) {
	tools := []ToolDefinition{{
		Type: "function",
		Function: ToolFunctionSchema{
			Name: "multi_tool",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "enum": []any{"read", "write", "delete"}},
					"path":   map[string]any{"type": "string"},
				},
				"required": []any{"action"},
			},
		},
	}}
	result := CleanToolSchemas("openai", tools)
	// Multi-action tools should NOT get strict mode
	if result[0].Function.Strict != nil { t.Error("multi-action tool should not have strict") }
}

func TestCleanToolSchemas_SingleAction_GetsStrict(t *testing.T) {
	tools := []ToolDefinition{{
		Type: "function",
		Function: ToolFunctionSchema{
			Name: "simple_tool",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
				"required": []any{"query"},
			},
		},
	}}
	result := CleanToolSchemas("openai", tools)
	if result[0].Function.Strict == nil || !*result[0].Function.Strict {
		t.Error("single-action tool should get strict mode for OpenAI")
	}
}

func TestIsMultiActionSchema_True(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"action": map[string]any{"enum": []any{"a", "b"}},
		},
	}
	if !IsMultiActionSchema(schema) { t.Error("should detect multi-action") }
}

func TestIsMultiActionSchema_False_SingleAction(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"action": map[string]any{"enum": []any{"only_one"}},
		},
	}
	if IsMultiActionSchema(schema) { t.Error("single action should not be multi") }
}

func TestIsMultiActionSchema_False_NoAction(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"query": map[string]any{"type": "string"},
		},
	}
	if IsMultiActionSchema(schema) { t.Error("no action property should not be multi") }
}

func TestCopySchema_DeepCopy(t *testing.T) {
	original := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	}
	copied := copySchema(original)
	// Modify copy
	props := copied["properties"].(map[string]any)
	nameProp := props["name"].(map[string]any)
	nameProp["type"] = "integer"
	// Original should be unchanged
	origProps := original["properties"].(map[string]any)
	origName := origProps["name"].(map[string]any)
	if origName["type"] != "string" { t.Error("deep copy failed — original modified") }
}

func TestMakeNullable_String(t *testing.T) {
	schema := map[string]any{"type": "string"}
	makeNullable(schema)
	types, ok := schema["type"].([]any)
	if !ok { t.Fatalf("expected array type, got %T", schema["type"]) }
	if len(types) != 2 { t.Errorf("expected 2 types, got %d", len(types)) }
}

func TestMakeNullable_AlreadyNull(t *testing.T) {
	schema := map[string]any{"type": "null"}
	makeNullable(schema)
	if schema["type"] != "null" { t.Error("should not modify already-null") }
}

func TestMakeNullable_AnyOf(t *testing.T) {
	schema := map[string]any{
		"anyOf": []any{map[string]any{"type": "string"}},
	}
	makeNullable(schema)
	variants := schema["anyOf"].([]any)
	if len(variants) != 2 { t.Errorf("expected 2 variants, got %d", len(variants)) }
}

func TestInferType(t *testing.T) {
	tests := []struct{ val any; want string }{
		{"hello", "string"},
		{42.0, "number"},
		{true, "boolean"},
		{nil, ""},
	}
	for _, tt := range tests {
		got := inferType(tt.val)
		if got != tt.want { t.Errorf("inferType(%v) = %q, want %q", tt.val, got, tt.want) }
	}
}

func TestIsNullSchema(t *testing.T) {
	tests := []struct{ schema map[string]any; want bool }{
		{map[string]any{"type": "null"}, true},
		{map[string]any{"const": nil}, true},
		{map[string]any{"enum": []any{nil}}, true},
		{map[string]any{"type": "string"}, false},
		{map[string]any{"enum": []any{"a", "b"}}, false},
	}
	for i, tt := range tests {
		got := isNullSchema(tt.schema)
		if got != tt.want { t.Errorf("test %d: isNullSchema = %v, want %v", i, got, tt.want) }
	}
}

func TestProfileForProvider(t *testing.T) {
	tests := []struct{ name string; strict bool; geminiStrip bool }{
		{"openai", true, false},
		{"anthropic", false, false},
		{"gemini", false, true},
		{"gemini-flash", false, true},
		{"xai", false, false},
		{"unknown", false, false},
	}
	for _, tt := range tests {
		p := profileForProvider(tt.name)
		if p.StrictToolMode != tt.strict { t.Errorf("%s: strict=%v, want %v", tt.name, p.StrictToolMode, tt.strict) }
		if tt.geminiStrip && len(p.StripKeys) < 10 { t.Errorf("%s: expected many strip keys for gemini", tt.name) }
	}
}

func TestRemoveTypeOnUnion(t *testing.T) {
	schema := map[string]any{
		"type":  "string",
		"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "integer"}},
	}
	result := removeTypeOnUnion(schema, 0)
	if _, ok := result["type"]; ok { t.Error("type should be removed when anyOf present") }
}

func TestStripKeys_Recursive(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"$schema": "draft-07",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "$schema": "should-be-stripped"},
		},
	}
	result := stripKeys(schema, []string{"$schema"}, 0)
	if _, ok := result["$schema"]; ok { t.Error("$schema not stripped at top level") }
	props := result["properties"].(map[string]any)
	nameProp := props["name"].(map[string]any)
	if _, ok := nameProp["$schema"]; ok { t.Error("$schema not stripped in nested property") }
}
