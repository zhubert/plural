//go:build darwin && cgo

package clipboard

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#include <stdlib.h>

// Helper function to check if a file path points to an image file
int isImageFile(NSString *path) {
    NSArray *imageExtensions = @[@"png", @"jpg", @"jpeg", @"gif", @"webp", @"tiff", @"tif", @"bmp"];
    NSString *ext = [[path pathExtension] lowercaseString];
    return [imageExtensions containsObject:ext];
}

// readImageFromPasteboard reads image data from the macOS pasteboard.
// It tries multiple formats (PNG, TIFF, etc.) and returns PNG-encoded data.
// If a file URL pointing to an image is in the clipboard, it reads that file.
// Returns the data length, or 0 if no image is available.
unsigned long readImageFromPasteboard(void **outData) {
    @autoreleasepool {
        NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];

        NSData *imageData = nil;
        NSBitmapImageRep *bitmapRep = nil;

        // First, check if clipboard contains a file URL pointing to an image
        // This happens when user copies an image file in Finder
        NSArray *fileURLs = [pasteboard readObjectsForClasses:@[[NSURL class]]
                                                      options:@{NSPasteboardURLReadingFileURLsOnlyKey: @YES}];
        if (fileURLs != nil && [fileURLs count] > 0) {
            NSURL *fileURL = fileURLs[0];
            NSString *path = [fileURL path];
            if (isImageFile(path)) {
                // Read the actual image file contents
                NSData *fileData = [NSData dataWithContentsOfURL:fileURL];
                if (fileData != nil) {
                    // Load as NSImage to convert to PNG
                    NSImage *image = [[NSImage alloc] initWithData:fileData];
                    if (image != nil) {
                        NSData *tiffData = [image TIFFRepresentation];
                        if (tiffData != nil) {
                            bitmapRep = [NSBitmapImageRep imageRepWithData:tiffData];
                            if (bitmapRep != nil) {
                                NSData *pngData = [bitmapRep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
                                if (pngData != nil) {
                                    unsigned long len = [pngData length];
                                    *outData = malloc(len);
                                    if (*outData == NULL) {
                                        return 0;
                                    }
                                    memcpy(*outData, [pngData bytes], len);
                                    return len;
                                }
                            }
                        }
                    }
                }
            }
        }

        // Try to read PNG directly (from actual image copy, not file copy)
        imageData = [pasteboard dataForType:NSPasteboardTypePNG];
        if (imageData != nil) {
            // Already PNG, return as-is
            unsigned long len = [imageData length];
            *outData = malloc(len);
            if (*outData == NULL) {
                return 0;
            }
            memcpy(*outData, [imageData bytes], len);
            return len;
        }

        // Try TIFF (what macOS screenshots use when copied directly)
        // But only if we didn't already find a file URL (to avoid getting file icons)
        if (fileURLs == nil || [fileURLs count] == 0) {
            imageData = [pasteboard dataForType:NSPasteboardTypeTIFF];
            if (imageData != nil) {
                // Convert TIFF to PNG
                bitmapRep = [NSBitmapImageRep imageRepWithData:imageData];
                if (bitmapRep != nil) {
                    NSData *pngData = [bitmapRep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
                    if (pngData != nil) {
                        unsigned long len = [pngData length];
                        *outData = malloc(len);
                        if (*outData == NULL) {
                            return 0;
                        }
                        memcpy(*outData, [pngData bytes], len);
                        return len;
                    }
                }
            }

            // Try to create NSImage from any available image type
            // This handles formats like PDF, EPS, etc.
            if ([pasteboard canReadObjectForClasses:@[[NSImage class]] options:nil]) {
                NSArray *images = [pasteboard readObjectsForClasses:@[[NSImage class]] options:nil];
                if (images != nil && [images count] > 0) {
                    NSImage *image = images[0];
                    NSData *tiffData = [image TIFFRepresentation];
                    if (tiffData != nil) {
                        bitmapRep = [NSBitmapImageRep imageRepWithData:tiffData];
                        if (bitmapRep != nil) {
                            NSData *pngData = [bitmapRep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
                            if (pngData != nil) {
                                unsigned long len = [pngData length];
                                *outData = malloc(len);
                                if (*outData == NULL) {
                                    return 0;
                                }
                                memcpy(*outData, [pngData bytes], len);
                                return len;
                            }
                        }
                    }
                }
            }
        }

        *outData = NULL;
        return 0;
    }
}

void freeImageData(void *data) {
    free(data);
}

// writeTextToPasteboard writes text to the macOS pasteboard.
// Returns 1 on success, 0 on failure.
int writeTextToPasteboard(const char *text, unsigned long length) {
    @autoreleasepool {
        NSPasteboard *pasteboard = [NSPasteboard generalPasteboard];
        [pasteboard clearContents];

        NSString *string = [[NSString alloc] initWithBytes:text length:length encoding:NSUTF8StringEncoding];
        if (string == nil) {
            return 0;
        }

        BOOL success = [pasteboard setString:string forType:NSPasteboardTypeString];
        return success ? 1 : 0;
    }
}
*/
import "C"

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"unsafe"

	"github.com/zhubert/plural/internal/logger"
)

// readNativeImage reads image data from the macOS pasteboard using native APIs.
// This handles TIFF (used by screenshots), PNG, and other image formats.
func readNativeImage() ([]byte, error) {
	var dataPtr unsafe.Pointer
	length := C.readImageFromPasteboard(&dataPtr)

	if length == 0 || dataPtr == nil {
		return nil, nil // No image data
	}

	// Copy data to Go slice and free C memory
	data := C.GoBytes(dataPtr, C.int(length))
	C.freeImageData(dataPtr)

	return data, nil
}

// ReadImage attempts to read an image from the clipboard.
// Returns nil if clipboard doesn't contain an image.
func ReadImage() (*ImageData, error) {
	logger.Log("Clipboard: Reading image using native macOS API")

	// Use native macOS implementation that handles TIFF, PNG, etc.
	imgBytes, err := readNativeImage()
	if err != nil {
		logger.Log("Clipboard: Native read error: %v", err)
		return nil, err
	}

	if len(imgBytes) == 0 {
		logger.Log("Clipboard: No image data found in pasteboard")
		return nil, nil
	}

	logger.Log("Clipboard: Read %d bytes of PNG image data from pasteboard", len(imgBytes))

	// Decode the PNG to get dimensions and validate
	img, format, err := image.Decode(bytes.NewReader(imgBytes))
	if err != nil {
		logger.Log("Clipboard: Failed to decode image: %v", err)
		return nil, fmt.Errorf("failed to decode clipboard image: %w", err)
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	logger.Log("Clipboard: Image decoded: %dx%d, format=%s", width, height, format)

	// Re-encode to ensure consistent PNG format
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		logger.Log("Clipboard: Failed to re-encode as PNG: %v", err)
		return nil, fmt.Errorf("failed to encode image as PNG: %w", err)
	}

	pngBytes := pngBuf.Bytes()
	logger.Log("Clipboard: Final PNG: %d bytes", len(pngBytes))

	return &ImageData{
		Data:      pngBytes,
		MediaType: "image/png",
		Width:     width,
		Height:    height,
	}, nil
}

// WriteText writes text to the clipboard.
func WriteText(text string) error {
	logger.Log("Clipboard: Writing text using native macOS API")

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	result := C.writeTextToPasteboard(cText, C.ulong(len(text)))
	if result == 0 {
		return fmt.Errorf("failed to write text to clipboard")
	}

	logger.Log("Clipboard: Wrote %d bytes of text", len(text))
	return nil
}
