package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateSchema_ProducesValidJSON(t *testing.T) {
	b, err := GenerateSchema()
	if err != nil {
		t.Fatalf("GenerateSchema: %v", err)
	}
	var raw any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
}

func TestGenerateSchema_ContainsExpectedFields(t *testing.T) {
	b, err := GenerateSchema()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	wants := []string{
		// YAML tag names used in the schema
		"\"version\"",
		"\"roots\"",
		"\"sessions\"",
		"\"templates\"",
		"\"defaults\"",
		"\"on_create\"", // hooks should be reflected
		SchemaID,
	}
	for _, w := range wants {
		if !strings.Contains(s, w) {
			t.Errorf("schema missing expected fragment %q", w)
		}
	}
}
