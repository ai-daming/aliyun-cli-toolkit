package oss

import (
	"encoding/json"
	"reflect"
	"testing"
)

func decodePolicy(t *testing.T, encoded string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("decode policy: %v", err)
	}
	return decoded
}

func onlyStatement(t *testing.T, policy map[string]any) map[string]any {
	t.Helper()
	statements, ok := policy["Statement"].([]any)
	if !ok || len(statements) != 1 {
		t.Fatalf("Statement = %#v", policy["Statement"])
	}
	statement, ok := statements[0].(map[string]any)
	if !ok {
		t.Fatalf("statement = %#v", statements[0])
	}
	return statement
}

func TestBuildListPolicyIsBucketAndExactPrefixOnly(t *testing.T) {
	encoded, err := buildListPolicy("component-bucket", "媒体/2026-")
	if err != nil {
		t.Fatal(err)
	}
	statement := onlyStatement(t, decodePolicy(t, encoded))
	if !reflect.DeepEqual(statement["Action"], []any{"oss:ListObjects"}) {
		t.Fatalf("Action = %#v", statement["Action"])
	}
	if !reflect.DeepEqual(statement["Resource"], []any{"acs:oss:*:*:component-bucket"}) {
		t.Fatalf("Resource = %#v", statement["Resource"])
	}
	wantCondition := map[string]any{"StringEquals": map[string]any{"oss:Prefix": "媒体/2026-"}}
	if !reflect.DeepEqual(statement["Condition"], wantCondition) {
		t.Fatalf("Condition = %#v", statement["Condition"])
	}
	if _, exists := statement["NotAction"]; exists {
		t.Fatal("policy must not contain NotAction")
	}
}

func TestBuildObjectPolicyIsExactKeyOnly(t *testing.T) {
	encoded, err := buildObjectPolicy("component-bucket", "oss:DeleteObject", "媒体/photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	statement := onlyStatement(t, decodePolicy(t, encoded))
	if !reflect.DeepEqual(statement["Action"], []any{"oss:DeleteObject"}) {
		t.Fatalf("Action = %#v", statement["Action"])
	}
	if !reflect.DeepEqual(statement["Resource"], []any{"acs:oss:*:*:component-bucket/媒体/photo.jpg"}) {
		t.Fatalf("Resource = %#v", statement["Resource"])
	}
	if _, exists := statement["Condition"]; exists {
		t.Fatal("object policy must not contain a condition")
	}
}

func TestPolicyJSONEscapesInputsInsteadOfInterpolating(t *testing.T) {
	encoded, err := buildObjectPolicy("bucket", "oss:GetObject", "quote\"/space name")
	if err != nil {
		t.Fatal(err)
	}
	statement := onlyStatement(t, decodePolicy(t, encoded))
	want := []any{"acs:oss:*:*:bucket/quote\"/space name"}
	if !reflect.DeepEqual(statement["Resource"], want) {
		t.Fatalf("Resource = %#v", statement["Resource"])
	}
}

func TestPolicyBuildersRejectInputsThatCouldBroadenPolicy(t *testing.T) {
	tests := []struct {
		name string
		call func() (string, error)
	}{
		{name: "bucket wildcard", call: func() (string, error) { return buildListPolicy("bucket*", "media/") }},
		{name: "key wildcard", call: func() (string, error) { return buildObjectPolicy("bucket", "oss:DeleteObject", "media/*") }},
		{name: "key traversal", call: func() (string, error) { return buildObjectPolicy("bucket", "oss:DeleteObject", "media/../secret") }},
		{name: "prefix control", call: func() (string, error) { return buildListPolicy("bucket", "media/line\nbreak") }},
		{name: "unsupported action", call: func() (string, error) { return buildObjectPolicy("bucket", "oss:*", "media/file") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.call(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
