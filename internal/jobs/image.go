package jobs

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"time"

	// Register decoders
	_ "image/gif"
)

// Image size limits for HTTP JSON (base64) transport.
// Raw decoded payload should stay under MaxImageBytes; base64 expands ~4/3.
const (
	MaxImageBytes     = 12 << 20 // 12 MiB raw input/output
	MaxImageBase64Len = 16 << 20 // ~16 MiB base64 string safety cap
	DefaultMaxEdge    = 1920
	DefaultQuality    = 80
)

// ImageResizePayload is one shard: a single image + options.
type ImageResizePayload struct {
	// ImageBase64 is preferred for LAN JSON protocol.
	ImageBase64 string `json:"imageBase64,omitempty"`
	// FileName is used for outbox naming (basename only).
	FileName string `json:"fileName,omitempty"`
	// MaxEdge scales so max(width,height) <= MaxEdge (keep aspect). 0 = default 1920.
	MaxEdge int `json:"maxEdge"`
	// Quality is JPEG quality 1–100 (ignored for PNG).
	Quality int `json:"quality"`
	// Format is "jpeg" | "png" | "webp". webp is best-effort: Mac falls back to jpeg.
	Format string `json:"format"`
}

// ImageResizeResult is returned by workers.
type ImageResizeResult struct {
	ImageBase64  string `json:"imageBase64"`
	FileName     string `json:"fileName,omitempty"`
	Format       string `json:"format"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	BytesIn      int    `json:"bytesIn"`
	BytesOut     int    `json:"bytesOut"`
	ElapsedMs    int64  `json:"elapsedMs"`
	SkippedScale bool   `json:"skippedScale,omitempty"`
	Note         string `json:"note,omitempty"`
}

func runImageResize(payload json.RawMessage) (json.RawMessage, Metrics, error) {
	var p ImageResizePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, Metrics{}, fmt.Errorf("image_resize payload: %w", err)
	}
	res, err := ResizeImage(p)
	if err != nil {
		return nil, Metrics{}, err
	}
	b, err := json.Marshal(res)
	if err != nil {
		return nil, Metrics{}, err
	}
	return b, Metrics{ElapsedMs: res.ElapsedMs}, nil
}

// ResizeImage decodes, scales to maxEdge, and re-encodes. Pure stdlib (jpeg/png).
func ResizeImage(p ImageResizePayload) (ImageResizeResult, error) {
	start := time.Now()
	raw, err := decodeImageBytes(p)
	if err != nil {
		return ImageResizeResult{}, err
	}
	if len(raw) > MaxImageBytes {
		return ImageResizeResult{}, fmt.Errorf("image too large: %d bytes (limit %d / ~12MB)", len(raw), MaxImageBytes)
	}
	img, formatHint, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return ImageResizeResult{}, fmt.Errorf("decode image: %w", err)
	}
	_ = formatHint

	maxEdge := p.MaxEdge
	if maxEdge <= 0 {
		maxEdge = DefaultMaxEdge
	}
	if maxEdge > 8192 {
		maxEdge = 8192
	}
	quality := p.Quality
	if quality <= 0 {
		quality = DefaultQuality
	}
	if quality > 100 {
		quality = 100
	}
	outFmt := normalizeFormat(p.Format)
	note := ""
	if outFmt == "webp" {
		outFmt = "jpeg"
		note = "webp encode not available on this worker; used jpeg"
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	skipped := false
	outImg := img
	nw, nh := w, h
	if w > maxEdge || h > maxEdge {
		if w >= h {
			nw = maxEdge
			nh = int(float64(h)*float64(maxEdge)/float64(w) + 0.5)
		} else {
			nh = maxEdge
			nw = int(float64(w)*float64(maxEdge)/float64(h) + 0.5)
		}
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
		outImg = scaleNearest(img, nw, nh)
	} else {
		skipped = true
	}

	var buf bytes.Buffer
	switch outFmt {
	case "png":
		enc := png.Encoder{CompressionLevel: png.DefaultCompression}
		if err := enc.Encode(&buf, outImg); err != nil {
			return ImageResizeResult{}, fmt.Errorf("encode png: %w", err)
		}
	default:
		outFmt = "jpeg"
		if err := jpeg.Encode(&buf, outImg, &jpeg.Options{Quality: quality}); err != nil {
			return ImageResizeResult{}, fmt.Errorf("encode jpeg: %w", err)
		}
	}
	outBytes := buf.Bytes()
	if len(outBytes) > MaxImageBytes {
		return ImageResizeResult{}, fmt.Errorf("output too large: %d bytes (limit %d)", len(outBytes), MaxImageBytes)
	}

	fileName := sanitizeFileName(p.FileName, outFmt)
	elapsed := time.Since(start).Milliseconds()
	if elapsed <= 0 {
		elapsed = 1
	}
	return ImageResizeResult{
		ImageBase64:  base64.StdEncoding.EncodeToString(outBytes),
		FileName:     fileName,
		Format:       outFmt,
		Width:        nw,
		Height:       nh,
		BytesIn:      len(raw),
		BytesOut:     len(outBytes),
		ElapsedMs:    elapsed,
		SkippedScale: skipped,
		Note:         note,
	}, nil
}

func decodeImageBytes(p ImageResizePayload) ([]byte, error) {
	b64 := strings.TrimSpace(p.ImageBase64)
	if b64 == "" {
		return nil, fmt.Errorf("image_resize: imageBase64 is required")
	}
	if len(b64) > MaxImageBase64Len {
		return nil, fmt.Errorf("imageBase64 too large (%d chars; limit ~%d)", len(b64), MaxImageBase64Len)
	}
	// Strip data URL prefix if present
	if i := strings.Index(b64, ","); i >= 0 && strings.Contains(strings.ToLower(b64[:i]), "base64") {
		b64 = b64[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(b64, "="))
		if err != nil {
			return nil, fmt.Errorf("imageBase64 decode: %w", err)
		}
	}
	return raw, nil
}

func normalizeFormat(f string) string {
	f = strings.ToLower(strings.TrimSpace(f))
	switch f {
	case "jpg", "jpeg":
		return "jpeg"
	case "png":
		return "png"
	case "webp":
		return "webp"
	case "":
		return "jpeg"
	default:
		return "jpeg"
	}
}

func sanitizeFileName(name, format string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || name == "." || name == ".." {
		name = "image"
	}
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	ext := ".jpg"
	if format == "png" {
		ext = ".png"
	} else if format == "webp" {
		ext = ".webp"
	}
	return name + ext
}

func scaleNearest(src image.Image, nw, nh int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + y*sh/nh
		if sy >= b.Max.Y {
			sy = b.Max.Y - 1
		}
		for x := 0; x < nw; x++ {
			sx := b.Min.X + x*sw/nw
			if sx >= b.Max.X {
				sx = b.Max.X - 1
			}
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// ImageExtOK reports whether a filename looks like a supported input image.
func ImageExtOK(name string) bool {
	n := strings.ToLower(name)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".heic", ".heif"} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return false
}
