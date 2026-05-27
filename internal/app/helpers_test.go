package app

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithRecover_HandlesPanic(t *testing.T) {
	handler := withRecover(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		panic("boom")
	})
	res, err := handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.IsError)
	textContent, ok := mcp.AsTextContent(res.Content[0])
	require.True(t, ok)
	assert.Contains(t, textContent.Text, "internal error")
	assert.Contains(t, textContent.Text, "boom")
}

func TestWithRecover_PassThrough(t *testing.T) {
	expected := mcp.NewToolResultText("hello")
	handler := withRecover(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return expected, nil
	})
	res, err := handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.Equal(t, expected, res)
}

func TestJSONResult_MarshalError(t *testing.T) {
	// chan fields cannot be marshaled to JSON
	type unmarshalable struct {
		Ch chan int
	}
	res, err := jsonResult(unmarshalable{Ch: make(chan int)})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, res.IsError)
}
