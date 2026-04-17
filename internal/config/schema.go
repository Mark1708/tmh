package config

import (
	"bytes"
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// SchemaID is the canonical identifier embedded in generated schema files
// and referenced from `tmh init` boilerplate so editors can fetch it.
const SchemaID = "https://raw.githubusercontent.com/mark1708/tmh/main/schemas/tmh.schema.json"

// GenerateSchema reflects the Config type into a JSON Schema document.
// Callers receive pretty-printed JSON ready to write to disk.
func GenerateSchema() ([]byte, error) {
	r := &jsonschema.Reflector{
		// The on-disk config is YAML; tell the reflector to derive
		// property names from `yaml:` struct tags instead of json.
		FieldNameTag:              "yaml",
		AllowAdditionalProperties: false,
		DoNotReference:            false,
		ExpandedStruct:            true,
	}
	schema := r.Reflect(&Config{})
	schema.ID = jsonschema.ID(SchemaID)
	schema.Title = "tmh configuration"
	schema.Description = "Schema for tmh's config.yml — see https://github.com/mark1708/tmh for the full reference."

	buf := bytes.Buffer{}
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(schema); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
