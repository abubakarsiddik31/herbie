package httpapi

import (
	"encoding/base64"
	"fmt"

	"github.com/abubakarsiddik31/golem/model"
)

const (
	maxImagesPerMessage = 4
	maxImageBytes       = 4 << 20 // per decoded image
)

var allowedImageTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

// imageInput is one base64-encoded attachment in a send-message body.
type imageInput struct {
	MediaType string `json:"mediaType"`
	Data      string `json:"data"` // standard base64, no data-URL prefix
}

// decodeImages validates and decodes attachments into golem image parts.
// Inline bytes (not URLs) are the portable path: Gemini's URL form expects
// provider-reachable file URIs, while inline base64 works on every provider.
func decodeImages(in []imageInput) ([]model.Part, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > maxImagesPerMessage {
		return nil, fmt.Errorf("at most %d images per message", maxImagesPerMessage)
	}
	parts := make([]model.Part, 0, len(in))
	for i, img := range in {
		if !allowedImageTypes[img.MediaType] {
			return nil, fmt.Errorf("image %d: unsupported type %q", i+1, img.MediaType)
		}
		data, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			return nil, fmt.Errorf("image %d: invalid base64", i+1)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("image %d: empty", i+1)
		}
		if len(data) > maxImageBytes {
			return nil, fmt.Errorf("image %d: larger than %d bytes", i+1, maxImageBytes)
		}
		parts = append(parts, model.ImageData(img.MediaType, data))
	}
	return parts, nil
}
