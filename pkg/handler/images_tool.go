package handler

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

type ImagesHandler struct {
	apiProvider *provider.ApiProvider
	logger      *zap.Logger
}

func NewImagesHandler(apiProvider *provider.ApiProvider, logger *zap.Logger) *ImagesHandler {
	return &ImagesHandler{
		apiProvider: apiProvider,
		logger:      logger,
	}
}

// GetImageHandler fetches a single image by its Slack file ID
// This allows retrieval of images that were not included inline due to size limits
func (ih *ImagesHandler) GetImageHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ih.logger.Debug("GetImageHandler called", zap.Any("params", request.Params))

	// Parse file_id parameter (required)
	fileID := request.GetString("file_id", "")
	if fileID == "" {
		return mcp.NewToolResultError("file_id parameter is required"), nil
	}

	// Check if file downloads are supported with current auth method
	if !ih.apiProvider.CanDownloadFiles() {
		return mcp.NewToolResultError("Image downloads not supported with browser tokens (xoxc/xoxd). Use OAuth tokens (xoxp/xoxb) instead."), nil
	}

	// Get file metadata from Slack
	file, _, _, err := ih.apiProvider.Slack().GetFileInfoContext(ctx, fileID, 0, 0)
	if err != nil {
		ih.logger.Error("Failed to get file info", zap.String("file_id", fileID), zap.Error(err))
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get file info: %v", err)), nil
	}

	// Validate file is an image
	if !isImageMimeType(file.Mimetype) {
		return mcp.NewToolResultError(fmt.Sprintf("File '%s' is not an image (type: %s). Only image files (PNG, JPEG, GIF, WebP) can be retrieved.", file.Name, file.Mimetype)), nil
	}

	// Get the download URL
	downloadURL := file.URLPrivate
	if downloadURL == "" {
		downloadURL = file.URLPrivateDownload
	}
	if downloadURL == "" {
		return mcp.NewToolResultError(fmt.Sprintf("File '%s' does not have a download URL available", file.Name)), nil
	}

	// Validate URL is from an allowed host (SSRF protection)
	if !isAllowedImageHost(downloadURL) {
		return mcp.NewToolResultError(fmt.Sprintf("File URL is not from an allowed Slack domain")), nil
	}

	// Check file size before downloading
	if file.Size > MaxImageSize {
		return mcp.NewToolResultError(fmt.Sprintf("File '%s' is too large (%d bytes). Maximum allowed size is %d bytes (5MB).", file.Name, file.Size, MaxImageSize)), nil
	}

	// Download the image
	imageData, err := DownloadImage(ctx, ih.apiProvider.Slack(), downloadURL)
	if err != nil {
		ih.logger.Error("Failed to download image", zap.String("file_id", fileID), zap.Error(err))
		return mcp.NewToolResultError(fmt.Sprintf("Failed to download image: %v", err)), nil
	}

	// Compress if needed to fit within response size limit
	mimeType := file.Mimetype
	compResult, _ := CompressImageIfNeeded(imageData, mimeType, MaxInlineImageBudget)
	if compResult.WasConverted {
		ih.logger.Debug("Image compressed",
			zap.String("file_id", fileID),
			zap.Int("original_size", compResult.OriginalSize),
			zap.Int("final_size", compResult.FinalSize),
			zap.String("original_type", mimeType),
			zap.String("final_type", compResult.MimeType),
		)
	}
	imageData = compResult.Data
	mimeType = compResult.MimeType

	// Create multi-content result with text metadata and image
	textContent := mcp.NewTextContent(fmt.Sprintf("File: %s\nSize: %d bytes\nType: %s", file.Name, len(imageData), mimeType))
	imageContent := mcp.NewImageContent(base64.StdEncoding.EncodeToString(imageData), mimeType)

	return &mcp.CallToolResult{
		Content: []mcp.Content{textContent, imageContent},
	}, nil
}
