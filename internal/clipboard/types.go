// Package clipboard provides image and text reading from the system clipboard.
package clipboard

import "fmt"

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
