package jobs

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func makeTestPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestImageResizeScalesAndEncodes(t *testing.T) {
	raw := makeTestPNG(800, 600)
	p := ImageResizePayload{
		ImageBase64: base64.StdEncoding.EncodeToString(raw),
		FileName:    "demo.png",
		MaxEdge:     200,
		Quality:     85,
		Format:      "jpeg",
	}
	res, err := ResizeImage(p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Width != 200 || res.Height != 150 {
		t.Fatalf("size got %dx%d want 200x150", res.Width, res.Height)
	}
	if res.Format != "jpeg" {
		t.Fatalf("format %s", res.Format)
	}
	if res.BytesOut <= 0 || res.ImageBase64 == "" {
		t.Fatalf("empty output")
	}
	if res.FileName != "demo.jpg" {
		t.Fatalf("filename %s", res.FileName)
	}
	out, err := base64.StdEncoding.DecodeString(res.ImageBase64)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 200 || b.Dy() != 150 {
		t.Fatalf("decoded %dx%d", b.Dx(), b.Dy())
	}
}

func TestImageResizeViaRun(t *testing.T) {
	raw := makeTestPNG(64, 64)
	pl, _ := json.Marshal(ImageResizePayload{
		ImageBase64: base64.StdEncoding.EncodeToString(raw),
		FileName:    "tiny.png",
		MaxEdge:     32,
		Format:      "png",
	})
	out, met, err := Run(TypeImageResize, pl)
	if err != nil {
		t.Fatal(err)
	}
	if met.ElapsedMs <= 0 {
		t.Fatalf("metrics")
	}
	var res ImageResizeResult
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatal(err)
	}
	if res.Width != 32 || res.Height != 32 {
		t.Fatalf("got %dx%d", res.Width, res.Height)
	}
}

func TestImageResizeRejectsHugeBase64(t *testing.T) {
	// Construct a payload claiming huge size without allocating 12MB+ of real image:
	// decode succeeds to empty-ish? Better: pass oversized decoded length via long base64 of zeros.
	big := make([]byte, MaxImageBytes+100)
	p := ImageResizePayload{
		ImageBase64: base64.StdEncoding.EncodeToString(big),
		MaxEdge:     100,
		Format:      "jpeg",
	}
	_, err := ResizeImage(p)
	if err == nil {
		t.Fatal("expected size error")
	}
}

func TestImageExtOK(t *testing.T) {
	if !ImageExtOK("a.JPG") || !ImageExtOK("b.png") {
		t.Fatal("expected ok")
	}
	if ImageExtOK("a.txt") {
		t.Fatal("txt should fail")
	}
}
