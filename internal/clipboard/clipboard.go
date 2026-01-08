// Package clipboard provides image and text reading from the system clipboard.
package clipboard

import (
	"bytes"
	"fmt"
	"image"
	"image/png"

	"golang.design/x/clipboard"

	"github.com/zhubert/plural/internal/logger"
)

// MaxImageSize is the maximum allowed image size (3.75MB per Anthropic limits)
const MaxImageSize = 3750000

// MaxImageDimension is the maximum allowed width or height (8000px per Anthropic limits)
const MaxImageDimension = 8000

// SupportedFormats lists the image formats Claude supports
var SupportedFormats = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

// ImageData represents clipboard image data
type ImageData struct {
	Data      []byte // PNG encoded image data
	MediaType string // MIME type (always "image/png" since we encode to PNG)
	Width     int
	Height    int
}

// initialized tracks whether the clipboard has been initialized
var initialized bool

// Init initializes the clipboard. Must be called before other functions.
// This is safe to call multiple times.
func Init() error {
	if initialized {
		return nil
	}

	if err := clipboard.Init(); err != nil {
		logger.Log("Clipboard: Failed to initialize: %v", err)
		return fmt.Errorf("failed to initialize clipboard: %w", err)
	}

	initialized = true
	logger.Log("Clipboard: Initialized successfully")
	return nil
}

// ReadImage attempts to read an image from the clipboard.
// Returns nil if clipboard doesn't contain an image.
func ReadImage() (*ImageData, error) {
	if !initialized {
		if err := Init(); err != nil {
			return nil, err
		}
	}

	// Read image bytes from clipboard
	imgBytes := clipboard.Read(clipboard.FmtImage)
	if len(imgBytes) == 0 {
		logger.Log("Clipboard: No image data found")
		return nil, nil // No image in clipboard, not an error
	}

	logger.Log("Clipboard: Read %d bytes of image data", len(imgBytes))

	// Decode the image to get dimensions
	img, format, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		logger.Log("Clipboard: Failed to decode image: %v", err)
		return nil, fmt.Errorf("failed to decode clipboard image: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	logger.Log("Clipboard: Image decoded: %dx%d, format=%s", width, height, format)

	// Re-encode as PNG for consistent format
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		logger.Log("Clipboard: Failed to encode as PNG: %v", err)
		return nil, fmt.Errorf("failed to encode image as PNG: %w", err)
	}

	pngBytes := pngBuf.Bytes()
	logger.Log("Clipboard: Re-encoded to PNG: %d bytes", len(pngBytes))

	return &ImageData{
		Data:      pngBytes,
		MediaType: "image/png",
		Width:     width,
		Height:    height,
	}, nil
}

// ReadText reads text from the clipboard.
func ReadText() (string, error) {
	if !initialized {
		if err := Init(); err != nil {
			return "", err
		}
	}

	textBytes := clipboard.Read(clipboard.FmtText)
	if textBytes == nil {
		return "", nil
	}

	return string(textBytes), nil
}

// Validate checks if the image meets Anthropic's requirements.
func (img *ImageData) Validate() error {
	if len(img.Data) > MaxImageSize {
		return fmt.Errorf("image too large: %d bytes (max %d bytes / %.1fMB)",
			len(img.Data), MaxImageSize, float64(MaxImageSize)/1000000)
	}

	if img.Width > MaxImageDimension || img.Height > MaxImageDimension {
		return fmt.Errorf("image dimensions too large: %dx%d (max %dx%d)",
			img.Width, img.Height, MaxImageDimension, MaxImageDimension)
	}

	return nil
}

// SizeKB returns the image size in kilobytes
func (img *ImageData) SizeKB() int {
	return len(img.Data) / 1024
}
