package app

import (
	"encoding/json"

	"github.com/dangernoodle-io/shesha/mcpx"
)

// jsonUnmarshalText unmarshals the JSON text of an mcpx.CallToolResult's
// single text-content block into v, mirroring unmarshalResult for the V2
// (shesha-typed) handler tests.
func jsonUnmarshalText(res *mcpx.CallToolResult, v any) error {
	return json.Unmarshal([]byte(mcpx.ResultText(res)), v)
}
