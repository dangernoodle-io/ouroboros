package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func withRecover(handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		defer func() {
			if r := recover(); r != nil {
				result = mcp.NewToolResultError(fmt.Sprintf("internal error: %v", r))
				err = nil
			}
		}()
		return handler(ctx, req)
	}
}

func jsonResult(v interface{}) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// parseStringSlice extracts a string array from tool arguments by key.
func parseStringSlice(args map[string]interface{}, key string) []string {
	if rawArray, ok := args[key].([]interface{}); ok {
		result := make([]string, 0, len(rawArray))
		for _, v := range rawArray {
			if s, ok := v.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

// parseInt64Slice extracts a number array from tool arguments by key.
func parseInt64Slice(args map[string]interface{}, key string) []int64 {
	if rawArray, ok := args[key].([]interface{}); ok {
		result := make([]int64, 0, len(rawArray))
		for _, v := range rawArray {
			if f, ok := v.(float64); ok {
				result = append(result, int64(f))
			}
		}
		return result
	}
	return nil
}

// parseEntriesArray extracts an array of entry objects from tool arguments by key.
func parseEntriesArray(args map[string]interface{}, key string) []map[string]interface{} {
	if rawArray, ok := args[key].([]interface{}); ok {
		result := make([]map[string]interface{}, 0, len(rawArray))
		for _, v := range rawArray {
			if m, ok := v.(map[string]interface{}); ok {
				result = append(result, m)
			}
		}
		return result
	}
	return nil
}
