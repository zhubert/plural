//go:build !darwin || (darwin && !cgo)

package clipboard

import (
	"bytes"
	"fmt"
	"image"
	"image/png"

	"golang.design/x/clipboard"

	"github.com/zhubert/plural/internal/logger"
)

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
// On Linux/Windows, this uses the golang.design/x/clipboard library.
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

// WriteText writes text to the clipboard.
func WriteText(text string) error {
	if !initialized {
		if err := Init(); err != nil {
			return err
		}
	}

	clipboard.Write(clipboard.FmtText, []byte(text))
	logger.Log("Clipboard: Wrote %d bytes of text", len(text))
	return nil
}
