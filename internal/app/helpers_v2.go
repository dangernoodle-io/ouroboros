package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dangernoodle-io/mcpkit/mcpx"

	"dangernoodle.io/ouroboros/internal/backlog"
)

// jsonResultV2 is jsonResult's mcpx-typed counterpart: marshal v to a single
// text-content block, or an ErrorResult carrying the marshal error verbatim.
// Out is always nil (see the STEP-0 probe / OU-1 build spec).
func jsonResultV2(v any) (*mcpx.CallToolResult, any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcpx.ErrorResult(err.Error()), nil, nil
	}
	return mcpx.TextResult(string(data)), nil, nil
}

// itemLinesTextV2 renders itemLinesResult's compact one-line-per-item text
// (or "no items"), as a plain string for the caller to wrap in
// mcpx.TextResult.
func itemLinesTextV2(items []backlog.Item) string {
	if len(items) == 0 {
		return "no items"
	}

	lines := make([]string, 0, len(items))
	for _, item := range items {
		componentStr := ""
		if item.Component != "" {
			componentStr = fmt.Sprintf("(%s) ", item.Component)
		}
		lines = append(lines, fmt.Sprintf("%s %s [%s] %s%s", item.ID, item.Priority, item.Status, componentStr, item.Title))
	}
	return strings.Join(lines, "\n")
}
