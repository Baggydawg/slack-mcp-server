package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
)

// Image processing constants
const (
	MaxImageSize           = 5 * 1024 * 1024  // 5MB - Claude API limit
	MaxImagesPerCall       = 10               // Maximum images per tool call
	ImageDownloadTimeout   = 30 * time.Second // Timeout for downloading images
	MaxConcurrentDownloads = 3                // Concurrent download limit
)

// SlackFileDownloader interface allows mocking the Slack file download functionality
type SlackFileDownloader interface {
	GetFileContext(ctx context.Context, downloadURL string, writer io.Writer) error
}

// ImageInfo holds metadata about an image extracted from a Slack message
type ImageInfo struct {
	FileID   string // Slack file ID
	Name     string // Original filename
	MimeType string // MIME type (e.g., image/png)
	Size     int    // File size in bytes
	URL      string // URLPrivate for download
	MsgTS    string // Message timestamp for context
}

// Allowed image hosts for SSRF protection
// Only Slack-hosted URLs are allowed to prevent server-side request forgery
var allowedImageHosts = map[string]bool{
	"files.slack.com":        true,
	"slack-edge.com":         true,
	"avatars.slack-edge.com": true,
}

// Supported image MIME types that Claude can process
var supportedImageMimeTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// isAllowedImageHost checks if the URL is from an allowed Slack domain
// This prevents SSRF attacks by rejecting external URLs
func isAllowedImageHost(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	host := parsed.Hostname()

	// Check exact match
	if allowedImageHosts[host] {
		return true
	}

	// Check if it's a subdomain of allowed hosts
	for allowed := range allowedImageHosts {
		if strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}

	return false
}

// isImageMimeType checks if the given MIME type is a supported image format
func isImageMimeType(mimeType string) bool {
	// Normalize MIME type by taking only the type/subtype part
	// (removing any parameters like charset)
	mimeType = strings.TrimSpace(mimeType)
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	mimeType = strings.ToLower(mimeType)

	return supportedImageMimeTypes[mimeType]
}

// ExtractImagesFromMessage extracts image file information from a Slack message
// It filters for supported image MIME types and returns metadata for each image
func ExtractImagesFromMessage(msg slack.Message) []ImageInfo {
	var images []ImageInfo

	for _, file := range msg.Files {
		if !isImageMimeType(file.Mimetype) {
			continue
		}

		// Use URLPrivate which requires authentication for download
		downloadURL := file.URLPrivate
		if downloadURL == "" {
			// Fall back to URLPrivateDownload if URLPrivate is empty
			downloadURL = file.URLPrivateDownload
		}

		if downloadURL == "" {
			// Skip files without a download URL
			continue
		}

		images = append(images, ImageInfo{
			FileID:   file.ID,
			Name:     file.Name,
			MimeType: file.Mimetype,
			Size:     file.Size,
			URL:      downloadURL,
			MsgTS:    msg.Timestamp,
		})
	}

	return images
}

// ExtractImagesFromAttachments extracts image URLs from Slack message attachments
// Only Slack-hosted URLs are included to prevent SSRF vulnerabilities
func ExtractImagesFromAttachments(attachments []slack.Attachment, msgTS string) []ImageInfo {
	var images []ImageInfo

	for _, attachment := range attachments {
		// Check ImageURL first (full-size image)
		if attachment.ImageURL != "" && isAllowedImageHost(attachment.ImageURL) {
			mimeType := guessMimeTypeFromURL(attachment.ImageURL)
			if isImageMimeType(mimeType) {
				images = append(images, ImageInfo{
					FileID:   "", // Attachments don't have file IDs
					Name:     extractFilenameFromURL(attachment.ImageURL),
					MimeType: mimeType,
					Size:     0, // Size unknown for attachment URLs
					URL:      attachment.ImageURL,
					MsgTS:    msgTS,
				})
			}
		}

		// Check ThumbURL as fallback (thumbnail image)
		if attachment.ThumbURL != "" && isAllowedImageHost(attachment.ThumbURL) {
			// Skip if we already added the full-size image from the same attachment
			if attachment.ImageURL != "" && isAllowedImageHost(attachment.ImageURL) {
				continue
			}

			mimeType := guessMimeTypeFromURL(attachment.ThumbURL)
			if isImageMimeType(mimeType) {
				images = append(images, ImageInfo{
					FileID:   "",
					Name:     extractFilenameFromURL(attachment.ThumbURL),
					MimeType: mimeType,
					Size:     0,
					URL:      attachment.ThumbURL,
					MsgTS:    msgTS,
				})
			}
		}
	}

	return images
}

// guessMimeTypeFromURL attempts to determine MIME type from URL file extension
func guessMimeTypeFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	path := strings.ToLower(parsed.Path)

	switch {
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(path, ".gif"):
		return "image/gif"
	case strings.HasSuffix(path, ".webp"):
		return "image/webp"
	default:
		// Default to PNG for unknown extensions (common for Slack screenshots)
		return "image/png"
	}
}

// extractFilenameFromURL extracts the filename from a URL path
func extractFilenameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "image"
	}

	path := parsed.Path
	if idx := strings.LastIndex(path, "/"); idx != -1 {
		filename := path[idx+1:]
		if filename != "" {
			return filename
		}
	}

	return "image"
}

// DownloadImage downloads an image from the given URL using the Slack API
// Returns the raw bytes or an error if download fails or size exceeds limit
func DownloadImage(ctx context.Context, slackClient SlackFileDownloader, url string) ([]byte, error) {
	// Create a context with timeout
	downloadCtx, cancel := context.WithTimeout(ctx, ImageDownloadTimeout)
	defer cancel()

	// Use a bytes.Buffer as the writer
	var buf bytes.Buffer

	// Download the file
	err := slackClient.GetFileContext(downloadCtx, url, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}

	data := buf.Bytes()

	// Check size doesn't exceed MaxImageSize
	if len(data) > MaxImageSize {
		return nil, fmt.Errorf("image size %d bytes exceeds maximum allowed size of %d bytes", len(data), MaxImageSize)
	}

	// CRITICAL: Validate that we actually got image data, not HTML
	// This prevents crashes when Slack returns a login page instead of the image
	if !isValidImageData(data) {
		// Check if it's HTML (indicates auth failure)
		if isHTMLContent(data) {
			return nil, fmt.Errorf("authentication failed: received HTML login page instead of image (browser tokens may not support file downloads)")
		}
		return nil, fmt.Errorf("downloaded data is not a valid image format")
	}

	return data, nil
}

// isValidImageData checks if the data starts with valid image magic bytes
func isValidImageData(data []byte) bool {
	if len(data) < 8 {
		return false
	}

	// PNG: starts with 0x89 0x50 0x4E 0x47 (‰PNG)
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return true
	}

	// JPEG: starts with 0xFF 0xD8 0xFF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return true
	}

	// GIF: starts with "GIF87a" or "GIF89a"
	if len(data) >= 6 && string(data[0:3]) == "GIF" {
		return true
	}

	// WebP: starts with "RIFF" and contains "WEBP" at offset 8
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return true
	}

	return false
}

// isHTMLContent checks if the data appears to be HTML
func isHTMLContent(data []byte) bool {
	if len(data) < 15 {
		return false
	}

	// Check beginning for HTML markers
	prefix := strings.ToLower(string(data[:min(500, len(data))]))
	if strings.Contains(prefix, "<!doctype") ||
		strings.Contains(prefix, "<html") ||
		strings.Contains(prefix, "<head") ||
		strings.Contains(prefix, "<body") {
		return true
	}

	// Check end for HTML closing tags
	if len(data) > 20 {
		suffix := strings.ToLower(string(data[len(data)-20:]))
		if strings.Contains(suffix, "</html>") || strings.Contains(suffix, "</body>") {
			return true
		}
	}

	return false
}

// imageDownloadResult holds the result of a single image download
type imageDownloadResult struct {
	FileID string
	Data   []byte
	Error  error
}

// DownloadImagesWithConcurrencyLimit downloads multiple images with a semaphore limit
// Returns map of fileID -> bytes, and slice of warning messages for failures
func DownloadImagesWithConcurrencyLimit(ctx context.Context, slackClient SlackFileDownloader, images []ImageInfo) (map[string][]byte, []string) {
	// Limit to MaxImagesPerCall
	if len(images) > MaxImagesPerCall {
		images = images[:MaxImagesPerCall]
	}

	if len(images) == 0 {
		return make(map[string][]byte), nil
	}

	// Create semaphore for concurrency control
	semaphore := make(chan struct{}, MaxConcurrentDownloads)
	resultChan := make(chan imageDownloadResult, len(images))

	var wg sync.WaitGroup

	for _, img := range images {
		wg.Add(1)
		go func(image ImageInfo) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Generate a unique key (use FileID if available, otherwise URL hash)
			key := image.FileID
			if key == "" {
				key = image.URL
			}

			// Skip images that are known to be over the size limit
			if image.Size > MaxImageSize {
				resultChan <- imageDownloadResult{
					FileID: key,
					Error:  fmt.Errorf("image '%s' size %d bytes exceeds limit", image.Name, image.Size),
				}
				return
			}

			data, err := DownloadImage(ctx, slackClient, image.URL)
			resultChan <- imageDownloadResult{
				FileID: key,
				Data:   data,
				Error:  err,
			}
		}(img)
	}

	// Close result channel after all downloads complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	imageData := make(map[string][]byte)
	var warnings []string

	for result := range resultChan {
		if result.Error != nil {
			warnings = append(warnings, fmt.Sprintf("Skipped image: %v", result.Error))
		} else {
			imageData[result.FileID] = result.Data
		}
	}

	return imageData, warnings
}

// ImagesToMCPContent converts downloaded images to MCP ImageContent
func ImagesToMCPContent(images []ImageInfo, imageData map[string][]byte) []mcp.Content {
	var content []mcp.Content

	for _, img := range images {
		// Get the key (FileID or URL)
		key := img.FileID
		if key == "" {
			key = img.URL
		}

		data, ok := imageData[key]
		if !ok {
			// Image was not downloaded (skipped or failed)
			continue
		}

		// Base64 encode the image data
		encoded := base64.StdEncoding.EncodeToString(data)

		// Create MCP ImageContent
		imageContent := mcp.NewImageContent(encoded, img.MimeType)
		content = append(content, imageContent)
	}

	return content
}
