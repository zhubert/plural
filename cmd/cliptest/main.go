//go:build ignore

package main

import (
	"fmt"

	"github.com/zhubert/plural/internal/clipboard"
)

func main() {
	fmt.Println("Testing clipboard read...")
	img, err := clipboard.ReadImage()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if img == nil {
		fmt.Println("No image in clipboard")
		return
	}
	fmt.Printf("Image found: %dx%d, %d bytes\n", img.Width, img.Height, len(img.Data))
}
