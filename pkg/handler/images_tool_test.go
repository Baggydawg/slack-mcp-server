package handler

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

func TestUnitGetImageHandler_MissingFileID(t *testing.T) {
	// Test that missing file_id parameter returns an error

	// Create a no-op logger for testing
	logger := zap.NewNop()

	// Create a mock ImagesHandler with nil provider (we won't reach provider calls)
	ih := &ImagesHandler{
		apiProvider: nil,
		logger:      logger,
	}

	// Create request without file_id parameter
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{}

	ctx := context.Background()
	result, err := ih.GetImageHandler(ctx, request)

	// Should not return a Go error (MCP errors are returned in result)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	// Should return an error result
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// Check that IsError is true
	if !result.IsError {
		t.Error("expected IsError to be true")
	}

	// Check that content contains error message about file_id
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	expectedMsg := "file_id parameter is required"
	if textContent.Text != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, textContent.Text)
	}
}

func TestUnitGetImageHandler_EmptyFileID(t *testing.T) {
	// Test that empty file_id parameter returns an error

	logger := zap.NewNop()
	ih := &ImagesHandler{
		apiProvider: nil,
		logger:      logger,
	}

	// Create request with empty file_id
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"file_id": "",
	}

	ctx := context.Background()
	result, err := ih.GetImageHandler(ctx, request)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if !result.IsError {
		t.Error("expected IsError to be true")
	}

	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	expectedMsg := "file_id parameter is required"
	if textContent.Text != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, textContent.Text)
	}
}
