// Package safeimage normalizes small, untrusted UI images without retaining
// metadata, trailing payloads, animation, or executable formats.
package safeimage

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	_ "image/jpeg"
	"image/png"
	"strings"
)

const MaxBytes = 2 << 20
const MaxDimension = 4096
const MaxPixels = 4 << 20
const MaxDataURLBytes = ((MaxBytes + 2) / 3 * 4) + 64

var ErrInvalid = errors.New("image.invalid")

// Normalize accepts only PNG/JPEG. WebP uploads are converted by the browser
// before submission, so no external decoder or upstream storage is required.
func Normalize(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > MaxBytes {
		return nil, ErrInvalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "png" && format != "jpeg") || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxDimension || cfg.Height > MaxDimension || cfg.Width*cfg.Height > MaxPixels {
		return nil, ErrInvalid
	}
	im, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrInvalid
	}
	var out bytes.Buffer
	if err := png.Encode(&out, im); err != nil || out.Len() > MaxBytes {
		return nil, ErrInvalid
	}
	return out.Bytes(), nil
}

func DataURL(value string) ([]byte, error) {
	if len(value) > MaxDataURLBytes {
		return nil, ErrInvalid
	}
	prefix, encoded, found := strings.Cut(value, ",")
	if !found || (prefix != "data:image/png;base64" && prefix != "data:image/jpeg;base64") {
		return nil, ErrInvalid
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || (prefix == "data:image/png;base64" && !bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n"))) || (prefix == "data:image/jpeg;base64" && !bytes.HasPrefix(raw, []byte("\xff\xd8"))) {
		return nil, ErrInvalid
	}
	return Normalize(raw)
}
