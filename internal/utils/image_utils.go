package utils

import (
	"bytes"
	"image"
	"image/jpeg"
	"log"

	// Register format decoders
	_ "image/jpeg"
	_ "image/png"
)

// CompressImage takes raw image bytes (expected to be PNG or JPEG),
// decodes it, and re-encodes it as a low-quality JPEG for preview purposes.
func CompressImage(data []byte, quality int) ([]byte, error) {
	// Decode the image (auto-detects format)
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		log.Printf("CompressImage: Warning: Failed to decode image: %v", err)
		return nil, err
	}

	// Prepare buffer for output
	var buf bytes.Buffer

	// Encode as JPEG with specified quality
	err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		log.Printf("CompressImage: Error: Failed to encode jpeg: %v", err)
		return nil, err
	}

	return buf.Bytes(), nil
}
