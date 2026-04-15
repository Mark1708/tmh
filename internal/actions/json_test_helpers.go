package actions

import "encoding/json"

// jsonMarshalImpl indirected so test helpers can invoke marshalling without
// pulling encoding/json into every test file individually.
func jsonMarshalImpl(v any) ([]byte, error) { return json.Marshal(v) }
