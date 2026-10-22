package safeimage

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func TestNormalizeStripsTrailingPayloadAndHandlesTransparency(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	im.Set(0, 0, color.NRGBA{R: 255, A: 128})
	var source bytes.Buffer
	if err := png.Encode(&source, im); err != nil {
		t.Fatal(err)
	}
	raw := append(source.Bytes(), []byte("<script>private metadata</script>")...)
	out, err := Normalize(raw)
	if err != nil || bytes.Contains(out, []byte("script")) {
		t.Fatal("normalization", err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := decoded.At(0, 0).RGBA()
	if alpha != 32896 {
		t.Fatal("transparency lost", alpha)
	}
	var jpg bytes.Buffer
	_ = jpeg.Encode(&jpg, im, nil)
	if _, err := Normalize(jpg.Bytes()); err != nil {
		t.Fatal("jpeg rejected", err)
	}
}
func TestImageLimitsAndMediaTypes(t *testing.T) {
	var source bytes.Buffer
	_ = png.Encode(&source, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString(source.Bytes())
	if _, err := DataURL(valid); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"data:image/svg+xml;base64,PHN2Zz4=", "https://example.invalid/avatar.png", strings.Replace(valid, "image/png", "image/jpeg", 1), valid + "?x", strings.Repeat("a", MaxDataURLBytes+1)} {
		if _, err := DataURL(value); err == nil {
			t.Fatal("accepted invalid image")
		}
	}
	for _, raw := range [][]byte{nil, []byte("GIF89a"), bytes.Repeat([]byte{0}, MaxBytes+1), source.Bytes()[:30]} {
		if _, err := Normalize(raw); err == nil {
			t.Fatal("accepted invalid bytes")
		}
	}
	// A valid CRC with huge dimensions must be rejected before pixel allocation.
	bomb := append([]byte(nil), source.Bytes()...)
	binary.BigEndian.PutUint32(bomb[16:20], 8192)
	binary.BigEndian.PutUint32(bomb[29:33], crc32.ChecksumIEEE(bomb[12:29]))
	if _, err := Normalize(bomb); err == nil {
		t.Fatal("accepted decompression bomb")
	}
}
