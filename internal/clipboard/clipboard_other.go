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

	log := logger.WithComponent("clipboard")

	if err := clipboard.Init(); err != nil {
		log.Error("failed to initialize", "error", err)
		return fmt.Errorf("failed to initialize clipboard: %w", err)
	}

	initialized = true
	log.Debug("initialized successfully")
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

	log := logger.WithComponent("clipboard")

	// Read image bytes from clipboard
	imgBytes := clipboard.Read(clipboard.FmtImage)
	if len(imgBytes) == 0 {
		log.Debug("no image data found")
		return nil, nil // No image in clipboard, not an error
	}

	log.Debug("read image data", "bytes", len(imgBytes))

	// Decode the image to get dimensions
	img, format, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		log.Debug("failed to decode image", "error", err)
		return nil, fmt.Errorf("failed to decode clipboard image: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	log.Debug("image decoded", "width", width, "height", height, "format", format)

	// Re-encode as PNG for consistent format
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		log.Debug("failed to encode as PNG", "error", err)
		return nil, fmt.Errorf("failed to encode image as PNG: %w", err)
	}

	pngBytes := pngBuf.Bytes()
	log.Debug("re-encoded to PNG", "bytes", len(pngBytes))

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

	log := logger.WithComponent("clipboard")
	clipboard.Write(clipboard.FmtText, []byte(text))
	log.Debug("wrote text", "bytes", len(text))
	return nil
}
