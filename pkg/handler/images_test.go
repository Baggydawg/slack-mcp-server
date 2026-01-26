package handler

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"testing"

	"github.com/slack-go/slack"
)

func TestUnitIsAllowedImageHost(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		allowed bool
	}{
		// Allowed Slack-hosted URLs
		{
			name:    "files.slack.com direct",
			url:     "https://files.slack.com/files/123/image.png",
			allowed: true,
		},
		{
			name:    "avatars.slack-edge.com",
			url:     "https://avatars.slack-edge.com/user.jpg",
			allowed: true,
		},
		{
			name:    "subdomain of slack-edge.com",
			url:     "https://a]b.slack-edge.com/img.png",
			allowed: true,
		},
		{
			name:    "slack-edge.com direct",
			url:     "https://slack-edge.com/image.png",
			allowed: true,
		},
		{
			name:    "files.slack.com with path",
			url:     "https://files.slack.com/files-pri/T12345-F67890/screenshot.png",
			allowed: true,
		},
		{
			name:    "files.slack.com with query params",
			url:     "https://files.slack.com/files/123/image.png?t=xoxe-12345",
			allowed: true,
		},

		// Blocked URLs - SSRF protection
		{
			name:    "AWS metadata endpoint",
			url:     "http://169.254.169.254/latest/meta-data",
			allowed: false,
		},
		{
			name:    "localhost",
			url:     "http://localhost/image.png",
			allowed: false,
		},
		{
			name:    "internal domain",
			url:     "http://internal.company.com/image.png",
			allowed: false,
		},
		{
			name:    "evil domain with slack in path",
			url:     "https://evil.com/files.slack.com/fake",
			allowed: false,
		},
		{
			name:    "file protocol",
			url:     "file:///etc/passwd",
			allowed: false,
		},
		{
			name:    "IP address",
			url:     "http://192.168.1.1/image.png",
			allowed: false,
		},
		{
			name:    "127.0.0.1",
			url:     "http://127.0.0.1/image.png",
			allowed: false,
		},
		{
			name:    "external image host",
			url:     "https://imgur.com/image.png",
			allowed: false,
		},
		{
			name:    "slack in subdomain of evil domain",
			url:     "https://files.slack.com.evil.com/image.png",
			allowed: false,
		},
		{
			name:    "empty URL",
			url:     "",
			allowed: false,
		},
		{
			name:    "malformed URL",
			url:     "not-a-url",
			allowed: false,
		},
		{
			name:    "URL with only scheme",
			url:     "https://",
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAllowedImageHost(tt.url)
			if got != tt.allowed {
				t.Errorf("isAllowedImageHost(%q) = %v, want %v", tt.url, got, tt.allowed)
			}
		})
	}
}

func TestUnitIsImageMimeType(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		isImage  bool
	}{
		// Supported image types
		{
			name:     "image/png",
			mimeType: "image/png",
			isImage:  true,
		},
		{
			name:     "image/jpeg",
			mimeType: "image/jpeg",
			isImage:  true,
		},
		{
			name:     "image/gif",
			mimeType: "image/gif",
			isImage:  true,
		},
		{
			name:     "image/webp",
			mimeType: "image/webp",
			isImage:  true,
		},

		// MIME types with parameters
		{
			name:     "image/png with charset",
			mimeType: "image/png; charset=utf-8",
			isImage:  true,
		},
		{
			name:     "image/jpeg with boundary",
			mimeType: "image/jpeg; boundary=something",
			isImage:  true,
		},
		{
			name:     "image/gif with extra spaces",
			mimeType: "  image/gif  ; charset=utf-8  ",
			isImage:  true,
		},

		// Case insensitivity
		{
			name:     "IMAGE/PNG uppercase",
			mimeType: "IMAGE/PNG",
			isImage:  true,
		},
		{
			name:     "Image/Jpeg mixed case",
			mimeType: "Image/Jpeg",
			isImage:  true,
		},

		// Non-image types
		{
			name:     "application/pdf",
			mimeType: "application/pdf",
			isImage:  false,
		},
		{
			name:     "text/plain",
			mimeType: "text/plain",
			isImage:  false,
		},
		{
			name:     "application/json",
			mimeType: "application/json",
			isImage:  false,
		},
		{
			name:     "video/mp4",
			mimeType: "video/mp4",
			isImage:  false,
		},
		{
			name:     "audio/mpeg",
			mimeType: "audio/mpeg",
			isImage:  false,
		},
		{
			name:     "image/svg+xml unsupported",
			mimeType: "image/svg+xml",
			isImage:  false,
		},
		{
			name:     "image/bmp unsupported",
			mimeType: "image/bmp",
			isImage:  false,
		},
		{
			name:     "image/tiff unsupported",
			mimeType: "image/tiff",
			isImage:  false,
		},
		{
			name:     "empty string",
			mimeType: "",
			isImage:  false,
		},
		{
			name:     "whitespace only",
			mimeType: "   ",
			isImage:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isImageMimeType(tt.mimeType)
			if got != tt.isImage {
				t.Errorf("isImageMimeType(%q) = %v, want %v", tt.mimeType, got, tt.isImage)
			}
		})
	}
}

func TestUnitExtractImagesFromMessage(t *testing.T) {
	tests := []struct {
		name           string
		message        slack.Message
		expectedCount  int
		expectedImages []ImageInfo
	}{
		{
			name: "message with PNG file",
			message: slack.Message{
				Msg: slack.Msg{
					Timestamp: "1234567890.123456",
					Files: []slack.File{
						{
							ID:         "F123",
							Name:       "screenshot.png",
							Mimetype:   "image/png",
							Size:       1024,
							URLPrivate: "https://files.slack.com/files/F123/screenshot.png",
						},
					},
				},
			},
			expectedCount: 1,
			expectedImages: []ImageInfo{
				{
					FileID:   "F123",
					Name:     "screenshot.png",
					MimeType: "image/png",
					Size:     1024,
					URL:      "https://files.slack.com/files/F123/screenshot.png",
					MsgTS:    "1234567890.123456",
				},
			},
		},
		{
			name: "message with PDF file (should be filtered out)",
			message: slack.Message{
				Msg: slack.Msg{
					Timestamp: "1234567890.123456",
					Files: []slack.File{
						{
							ID:         "F456",
							Name:       "document.pdf",
							Mimetype:   "application/pdf",
							Size:       2048,
							URLPrivate: "https://files.slack.com/files/F456/document.pdf",
						},
					},
				},
			},
			expectedCount:  0,
			expectedImages: nil,
		},
		{
			name: "message with mixed files (only images extracted)",
			message: slack.Message{
				Msg: slack.Msg{
					Timestamp: "1234567890.123456",
					Files: []slack.File{
						{
							ID:         "F001",
							Name:       "photo.jpeg",
							Mimetype:   "image/jpeg",
							Size:       512,
							URLPrivate: "https://files.slack.com/files/F001/photo.jpeg",
						},
						{
							ID:         "F002",
							Name:       "document.pdf",
							Mimetype:   "application/pdf",
							Size:       1024,
							URLPrivate: "https://files.slack.com/files/F002/document.pdf",
						},
						{
							ID:         "F003",
							Name:       "animation.gif",
							Mimetype:   "image/gif",
							Size:       256,
							URLPrivate: "https://files.slack.com/files/F003/animation.gif",
						},
						{
							ID:         "F004",
							Name:       "data.json",
							Mimetype:   "application/json",
							Size:       128,
							URLPrivate: "https://files.slack.com/files/F004/data.json",
						},
					},
				},
			},
			expectedCount: 2,
			expectedImages: []ImageInfo{
				{
					FileID:   "F001",
					Name:     "photo.jpeg",
					MimeType: "image/jpeg",
					Size:     512,
					URL:      "https://files.slack.com/files/F001/photo.jpeg",
					MsgTS:    "1234567890.123456",
				},
				{
					FileID:   "F003",
					Name:     "animation.gif",
					MimeType: "image/gif",
					Size:     256,
					URL:      "https://files.slack.com/files/F003/animation.gif",
					MsgTS:    "1234567890.123456",
				},
			},
		},
		{
			name: "message with no files",
			message: slack.Message{
				Msg: slack.Msg{
					Timestamp: "1234567890.123456",
					Text:      "Hello world",
				},
			},
			expectedCount:  0,
			expectedImages: nil,
		},
		{
			name: "message with image but no download URL",
			message: slack.Message{
				Msg: slack.Msg{
					Timestamp: "1234567890.123456",
					Files: []slack.File{
						{
							ID:       "F789",
							Name:     "no-url.png",
							Mimetype: "image/png",
							Size:     1024,
							// URLPrivate is empty
						},
					},
				},
			},
			expectedCount:  0,
			expectedImages: nil,
		},
		{
			name: "message with image using URLPrivateDownload fallback",
			message: slack.Message{
				Msg: slack.Msg{
					Timestamp: "1234567890.123456",
					Files: []slack.File{
						{
							ID:                 "F999",
							Name:               "fallback.webp",
							Mimetype:           "image/webp",
							Size:               2048,
							URLPrivate:         "",
							URLPrivateDownload: "https://files.slack.com/files/F999/download/fallback.webp",
						},
					},
				},
			},
			expectedCount: 1,
			expectedImages: []ImageInfo{
				{
					FileID:   "F999",
					Name:     "fallback.webp",
					MimeType: "image/webp",
					Size:     2048,
					URL:      "https://files.slack.com/files/F999/download/fallback.webp",
					MsgTS:    "1234567890.123456",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractImagesFromMessage(tt.message)

			if len(got) != tt.expectedCount {
				t.Errorf("ExtractImagesFromMessage() returned %d images, want %d", len(got), tt.expectedCount)
				return
			}

			for i, expected := range tt.expectedImages {
				if i >= len(got) {
					break
				}
				if got[i].FileID != expected.FileID {
					t.Errorf("Image[%d].FileID = %q, want %q", i, got[i].FileID, expected.FileID)
				}
				if got[i].Name != expected.Name {
					t.Errorf("Image[%d].Name = %q, want %q", i, got[i].Name, expected.Name)
				}
				if got[i].MimeType != expected.MimeType {
					t.Errorf("Image[%d].MimeType = %q, want %q", i, got[i].MimeType, expected.MimeType)
				}
				if got[i].Size != expected.Size {
					t.Errorf("Image[%d].Size = %d, want %d", i, got[i].Size, expected.Size)
				}
				if got[i].URL != expected.URL {
					t.Errorf("Image[%d].URL = %q, want %q", i, got[i].URL, expected.URL)
				}
				if got[i].MsgTS != expected.MsgTS {
					t.Errorf("Image[%d].MsgTS = %q, want %q", i, got[i].MsgTS, expected.MsgTS)
				}
			}
		})
	}
}

func TestUnitExtractImagesFromAttachments(t *testing.T) {
	tests := []struct {
		name           string
		attachments    []slack.Attachment
		msgTS          string
		expectedCount  int
		expectedImages []ImageInfo
	}{
		{
			name: "attachment with Slack-hosted ImageURL",
			attachments: []slack.Attachment{
				{
					ImageURL: "https://files.slack.com/files/123/preview.png",
				},
			},
			msgTS:         "1234567890.123456",
			expectedCount: 1,
			expectedImages: []ImageInfo{
				{
					FileID:   "",
					Name:     "preview.png",
					MimeType: "image/png",
					Size:     0,
					URL:      "https://files.slack.com/files/123/preview.png",
					MsgTS:    "1234567890.123456",
				},
			},
		},
		{
			name: "attachment with external ImageURL (blocked by SSRF protection)",
			attachments: []slack.Attachment{
				{
					ImageURL: "https://external-site.com/image.png",
				},
			},
			msgTS:          "1234567890.123456",
			expectedCount:  0,
			expectedImages: nil,
		},
		{
			name: "attachment with no ImageURL",
			attachments: []slack.Attachment{
				{
					Title: "Some attachment",
					Text:  "With text but no image",
				},
			},
			msgTS:          "1234567890.123456",
			expectedCount:  0,
			expectedImages: nil,
		},
		{
			name: "attachment with Slack-hosted ThumbURL (fallback)",
			attachments: []slack.Attachment{
				{
					ThumbURL: "https://avatars.slack-edge.com/thumb.jpg",
				},
			},
			msgTS:         "1234567890.123456",
			expectedCount: 1,
			expectedImages: []ImageInfo{
				{
					FileID:   "",
					Name:     "thumb.jpg",
					MimeType: "image/jpeg",
					Size:     0,
					URL:      "https://avatars.slack-edge.com/thumb.jpg",
					MsgTS:    "1234567890.123456",
				},
			},
		},
		{
			name: "attachment with both ImageURL and ThumbURL (prefers ImageURL)",
			attachments: []slack.Attachment{
				{
					ImageURL: "https://files.slack.com/files/123/full.png",
					ThumbURL: "https://files.slack.com/files/123/thumb.png",
				},
			},
			msgTS:         "1234567890.123456",
			expectedCount: 1,
			expectedImages: []ImageInfo{
				{
					FileID:   "",
					Name:     "full.png",
					MimeType: "image/png",
					Size:     0,
					URL:      "https://files.slack.com/files/123/full.png",
					MsgTS:    "1234567890.123456",
				},
			},
		},
		{
			name: "attachment with blocked ImageURL but allowed ThumbURL",
			attachments: []slack.Attachment{
				{
					ImageURL: "https://external.com/image.png",
					ThumbURL: "https://files.slack.com/files/123/thumb.png",
				},
			},
			msgTS:         "1234567890.123456",
			expectedCount: 1,
			expectedImages: []ImageInfo{
				{
					FileID:   "",
					Name:     "thumb.png",
					MimeType: "image/png",
					Size:     0,
					URL:      "https://files.slack.com/files/123/thumb.png",
					MsgTS:    "1234567890.123456",
				},
			},
		},
		{
			name: "multiple attachments with mixed URLs",
			attachments: []slack.Attachment{
				{
					ImageURL: "https://files.slack.com/files/1/image1.png",
				},
				{
					ImageURL: "https://evil.com/malicious.png",
				},
				{
					ImageURL: "https://slack-edge.com/image2.gif",
				},
			},
			msgTS:         "1234567890.123456",
			expectedCount: 2,
			expectedImages: []ImageInfo{
				{
					FileID:   "",
					Name:     "image1.png",
					MimeType: "image/png",
					Size:     0,
					URL:      "https://files.slack.com/files/1/image1.png",
					MsgTS:    "1234567890.123456",
				},
				{
					FileID:   "",
					Name:     "image2.gif",
					MimeType: "image/gif",
					Size:     0,
					URL:      "https://slack-edge.com/image2.gif",
					MsgTS:    "1234567890.123456",
				},
			},
		},
		{
			name:           "empty attachments",
			attachments:    []slack.Attachment{},
			msgTS:          "1234567890.123456",
			expectedCount:  0,
			expectedImages: nil,
		},
		{
			name:           "nil attachments",
			attachments:    nil,
			msgTS:          "1234567890.123456",
			expectedCount:  0,
			expectedImages: nil,
		},
		{
			name: "attachment with subdomain of slack-edge.com",
			attachments: []slack.Attachment{
				{
					ImageURL: "https://cdn.slack-edge.com/images/photo.webp",
				},
			},
			msgTS:         "1234567890.123456",
			expectedCount: 1,
			expectedImages: []ImageInfo{
				{
					FileID:   "",
					Name:     "photo.webp",
					MimeType: "image/webp",
					Size:     0,
					URL:      "https://cdn.slack-edge.com/images/photo.webp",
					MsgTS:    "1234567890.123456",
				},
			},
		},
		{
			name: "attachment with SSRF attempt via localhost",
			attachments: []slack.Attachment{
				{
					ImageURL: "http://localhost:8080/admin/secret.png",
				},
			},
			msgTS:          "1234567890.123456",
			expectedCount:  0,
			expectedImages: nil,
		},
		{
			name: "attachment with SSRF attempt via AWS metadata",
			attachments: []slack.Attachment{
				{
					ImageURL: "http://169.254.169.254/latest/meta-data/",
				},
			},
			msgTS:          "1234567890.123456",
			expectedCount:  0,
			expectedImages: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractImagesFromAttachments(tt.attachments, tt.msgTS)

			if len(got) != tt.expectedCount {
				t.Errorf("ExtractImagesFromAttachments() returned %d images, want %d", len(got), tt.expectedCount)
				return
			}

			for i, expected := range tt.expectedImages {
				if i >= len(got) {
					break
				}
				if got[i].FileID != expected.FileID {
					t.Errorf("Image[%d].FileID = %q, want %q", i, got[i].FileID, expected.FileID)
				}
				if got[i].Name != expected.Name {
					t.Errorf("Image[%d].Name = %q, want %q", i, got[i].Name, expected.Name)
				}
				if got[i].MimeType != expected.MimeType {
					t.Errorf("Image[%d].MimeType = %q, want %q", i, got[i].MimeType, expected.MimeType)
				}
				if got[i].URL != expected.URL {
					t.Errorf("Image[%d].URL = %q, want %q", i, got[i].URL, expected.URL)
				}
				if got[i].MsgTS != expected.MsgTS {
					t.Errorf("Image[%d].MsgTS = %q, want %q", i, got[i].MsgTS, expected.MsgTS)
				}
			}
		})
	}
}

func TestUnitGuessMimeTypeFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "PNG extension",
			url:      "https://files.slack.com/files/123/image.png",
			expected: "image/png",
		},
		{
			name:     "JPG extension",
			url:      "https://files.slack.com/files/123/photo.jpg",
			expected: "image/jpeg",
		},
		{
			name:     "JPEG extension",
			url:      "https://files.slack.com/files/123/photo.jpeg",
			expected: "image/jpeg",
		},
		{
			name:     "GIF extension",
			url:      "https://files.slack.com/files/123/animation.gif",
			expected: "image/gif",
		},
		{
			name:     "WEBP extension",
			url:      "https://files.slack.com/files/123/modern.webp",
			expected: "image/webp",
		},
		{
			name:     "uppercase extension",
			url:      "https://files.slack.com/files/123/IMAGE.PNG",
			expected: "image/png",
		},
		{
			name:     "unknown extension defaults to PNG",
			url:      "https://files.slack.com/files/123/file.xyz",
			expected: "image/png",
		},
		{
			name:     "no extension defaults to PNG",
			url:      "https://files.slack.com/files/123/noextension",
			expected: "image/png",
		},
		{
			name:     "URL with query params",
			url:      "https://files.slack.com/files/123/image.jpg?token=abc",
			expected: "image/jpeg",
		},
		{
			name:     "malformed URL still returns default PNG",
			url:      "not-a-valid-url-://",
			expected: "image/png",
		},
		{
			name:     "truly invalid URL returns empty",
			url:      "://",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guessMimeTypeFromURL(tt.url)
			if got != tt.expected {
				t.Errorf("guessMimeTypeFromURL(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestUnitExtractFilenameFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "simple filename",
			url:      "https://files.slack.com/files/123/screenshot.png",
			expected: "screenshot.png",
		},
		{
			name:     "nested path",
			url:      "https://files.slack.com/files/T123/F456/deep/path/image.jpg",
			expected: "image.jpg",
		},
		{
			name:     "URL with query params",
			url:      "https://files.slack.com/files/123/photo.gif?t=xoxe-123",
			expected: "photo.gif",
		},
		{
			name:     "trailing slash defaults to image",
			url:      "https://files.slack.com/files/",
			expected: "image",
		},
		{
			name:     "invalid URL defaults to image",
			url:      "not-a-url",
			expected: "image",
		},
		{
			name:     "empty URL defaults to image",
			url:      "",
			expected: "image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFilenameFromURL(tt.url)
			if got != tt.expected {
				t.Errorf("extractFilenameFromURL(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

// mockSlackFileDownloader is a mock implementation of SlackFileDownloader for testing
type mockSlackFileDownloader struct {
	files map[string][]byte // URL -> data mapping
	err   error             // error to return (if any)
}

func (m *mockSlackFileDownloader) GetFileContext(ctx context.Context, downloadURL string, writer io.Writer) error {
	if m.err != nil {
		return m.err
	}
	if data, ok := m.files[downloadURL]; ok {
		_, err := writer.Write(data)
		return err
	}
	return fmt.Errorf("file not found: %s", downloadURL)
}

func TestUnitDownloadImagesWithBudget(t *testing.T) {
	// Create valid PNG magic bytes for testing
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	// Helper to create fake PNG data of a specific size
	makePNG := func(size int) []byte {
		data := make([]byte, size)
		copy(data, pngMagic)
		return data
	}

	tests := []struct {
		name             string
		images           []ImageInfo
		budget           int
		mockFiles        map[string][]byte
		expectedIncluded int
		expectedSkipped  int
		expectedWarnings int
	}{
		{
			name: "all images fit within budget",
			images: []ImageInfo{
				{FileID: "F001", Name: "img1.png", Size: 1000, URL: "https://files.slack.com/F001"},
				{FileID: "F002", Name: "img2.png", Size: 1000, URL: "https://files.slack.com/F002"},
			},
			budget: 5000,
			mockFiles: map[string][]byte{
				"https://files.slack.com/F001": makePNG(1000),
				"https://files.slack.com/F002": makePNG(1000),
			},
			expectedIncluded: 2,
			expectedSkipped:  0,
			expectedWarnings: 0,
		},
		{
			name: "some images fit - partial inclusion in chronological order",
			images: []ImageInfo{
				{FileID: "F001", Name: "img1.png", Size: 1000, URL: "https://files.slack.com/F001"},
				{FileID: "F002", Name: "img2.png", Size: 1000, URL: "https://files.slack.com/F002"},
				{FileID: "F003", Name: "img3.png", Size: 1000, URL: "https://files.slack.com/F003"},
			},
			budget: 1500, // Only enough for first image + some buffer
			mockFiles: map[string][]byte{
				"https://files.slack.com/F001": makePNG(1000),
				"https://files.slack.com/F002": makePNG(1000),
				"https://files.slack.com/F003": makePNG(1000),
			},
			expectedIncluded: 1, // Only first image fits
			expectedSkipped:  2, // Remaining two skipped
			expectedWarnings: 0,
		},
		{
			name: "first image exceeds budget - all skipped",
			images: []ImageInfo{
				{FileID: "F001", Name: "img1.png", Size: 2000, URL: "https://files.slack.com/F001"},
				{FileID: "F002", Name: "img2.png", Size: 500, URL: "https://files.slack.com/F002"},
			},
			budget: 1000,
			mockFiles: map[string][]byte{
				"https://files.slack.com/F001": makePNG(2000),
				"https://files.slack.com/F002": makePNG(500),
			},
			expectedIncluded: 0, // First image exceeds budget based on known size
			expectedSkipped:  2, // Both skipped (first exceeds budget, second skipped due to budget flag)
			expectedWarnings: 0,
		},
		{
			name:             "empty images list",
			images:           []ImageInfo{},
			budget:           5000,
			mockFiles:        map[string][]byte{},
			expectedIncluded: 0,
			expectedSkipped:  0,
			expectedWarnings: 0,
		},
		{
			name: "image without FileID uses URL as key",
			images: []ImageInfo{
				{FileID: "", Name: "attachment.png", Size: 500, URL: "https://files.slack.com/attachment"},
			},
			budget: 1000,
			mockFiles: map[string][]byte{
				"https://files.slack.com/attachment": makePNG(500),
			},
			expectedIncluded: 1,
			expectedSkipped:  0,
			expectedWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockSlackFileDownloader{files: tt.mockFiles}
			ctx := context.Background()

			imageData, _, skipped, warnings := DownloadImagesWithBudget(ctx, mock, tt.images, tt.budget)

			if len(imageData) != tt.expectedIncluded {
				t.Errorf("expected %d included images, got %d", tt.expectedIncluded, len(imageData))
			}
			if len(skipped) != tt.expectedSkipped {
				t.Errorf("expected %d skipped images, got %d", tt.expectedSkipped, len(skipped))
			}
			if len(warnings) != tt.expectedWarnings {
				t.Errorf("expected %d warnings, got %d", tt.expectedWarnings, len(warnings))
			}
		})
	}
}

// createTestPNG creates a synthetic PNG image for testing
func createTestPNG(width, height int, pattern string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with a pattern based on the pattern parameter
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			switch pattern {
			case "solid":
				img.Set(x, y, color.RGBA{100, 150, 200, 255})
			case "gradient":
				img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
			case "checkerboard":
				if (x/10+y/10)%2 == 0 {
					img.Set(x, y, color.RGBA{255, 255, 255, 255})
				} else {
					img.Set(x, y, color.RGBA{0, 0, 0, 255})
				}
			default:
				img.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8((x + y) % 256), 255})
			}
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestUnitCompressPNGToJPEG(t *testing.T) {
	tests := []struct {
		name    string
		pngData []byte
		quality int
		wantErr bool
	}{
		{
			name:    "valid PNG at quality 80",
			pngData: createTestPNG(100, 100, "gradient"),
			quality: 80,
			wantErr: false,
		},
		{
			name:    "valid PNG at quality 40",
			pngData: createTestPNG(100, 100, "gradient"),
			quality: 40,
			wantErr: false,
		},
		{
			name:    "invalid PNG data",
			pngData: []byte("not a png"),
			quality: 80,
			wantErr: true,
		},
		{
			name:    "empty data",
			pngData: []byte{},
			quality: 80,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := compressPNGToJPEG(tt.pngData, tt.quality)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify result is valid JPEG (starts with FFD8FF)
			if len(result) < 3 || result[0] != 0xFF || result[1] != 0xD8 || result[2] != 0xFF {
				t.Error("result is not valid JPEG data")
			}
		})
	}
}

func TestUnitCompressImageIfNeeded(t *testing.T) {
	smallPNG := createTestPNG(50, 50, "solid")
	largePNG := createTestPNG(500, 500, "gradient")

	// Create a JPEG for testing (compress a PNG first)
	jpegData, _ := compressPNGToJPEG(createTestPNG(100, 100, "solid"), 80)

	tests := []struct {
		name          string
		data          []byte
		mimeType      string
		budget        int
		wantConverted bool
		wantMimeType  string
	}{
		{
			name:          "under budget PNG still converted to JPEG",
			data:          smallPNG,
			mimeType:      "image/png",
			budget:        len(smallPNG) + 1000,
			wantConverted: true,
			wantMimeType:  "image/jpeg",
		},
		{
			name:          "over budget PNG gets compressed",
			data:          largePNG,
			mimeType:      "image/png",
			budget:        len(largePNG) / 2, // Force compression
			wantConverted: true,
			wantMimeType:  "image/jpeg",
		},
		{
			name:          "JPEG unchanged even if over budget",
			data:          jpegData,
			mimeType:      "image/jpeg",
			budget:        100, // Way under budget
			wantConverted: false,
			wantMimeType:  "image/jpeg",
		},
		{
			name:          "GIF unchanged",
			data:          []byte("fake gif data"),
			mimeType:      "image/gif",
			budget:        1,
			wantConverted: false,
			wantMimeType:  "image/gif",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CompressImageIfNeeded(tt.data, tt.mimeType, tt.budget)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result.WasConverted != tt.wantConverted {
				t.Errorf("WasConverted = %v, want %v", result.WasConverted, tt.wantConverted)
			}

			if result.MimeType != tt.wantMimeType {
				t.Errorf("MimeType = %v, want %v", result.MimeType, tt.wantMimeType)
			}

			if result.OriginalSize != len(tt.data) {
				t.Errorf("OriginalSize = %d, want %d", result.OriginalSize, len(tt.data))
			}
		})
	}
}

func TestUnitCompressPNGToJPEG_QualityAffectsSize(t *testing.T) {
	png := createTestPNG(200, 200, "gradient")

	jpeg80, err := compressPNGToJPEG(png, 80)
	if err != nil {
		t.Fatalf("compression at 80 failed: %v", err)
	}

	jpeg40, err := compressPNGToJPEG(png, 40)
	if err != nil {
		t.Fatalf("compression at 40 failed: %v", err)
	}

	// Lower quality should produce smaller file
	if len(jpeg40) >= len(jpeg80) {
		t.Errorf("quality 40 (%d bytes) should be smaller than quality 80 (%d bytes)",
			len(jpeg40), len(jpeg80))
	}
}
