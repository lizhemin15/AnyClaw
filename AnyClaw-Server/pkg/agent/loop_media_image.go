// AnyClaw - Ultra-lightweight personal AI agent
// License: MIT

package agent

import (
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/anyclaw/anyclaw-server/pkg/logger"
)

func isRasterImageMIME(mime string) bool {
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func rgbaHasTransparency(img *image.RGBA) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 8 {
		for x := b.Min.X; x < b.Max.X; x += 8 {
			if img.RGBAAt(x, y).A < 0xff {
				return true
			}
		}
	}
	return false
}

// resizeRGBANearest scales src to newW x newH (both >= 1).
func resizeRGBANearest(src image.Image, newW, newH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw <= 0 || sh <= 0 {
		return dst
	}
	for y := 0; y < newH; y++ {
		sy := sb.Min.Y + (y*sh)/newH
		for x := 0; x < newW; x++ {
			sx := sb.Min.X + (x*sw)/newW
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

// shrinkImageFileIfNeeded returns a path to read for encoding (maybe a new temp file).
// If shrunk is true, caller must os.Remove(outPath) after use.
func shrinkImageFileIfNeeded(path, mime string, maxEdge, jpegQuality int) (outPath, outMime string, shrunk bool) {
	if maxEdge <= 0 || !isRasterImageMIME(mime) {
		return path, mime, false
	}
	f, err := os.Open(path)
	if err != nil {
		logger.WarnCF("agent", "shrink image: open", map[string]any{"path": path, "error": err.Error()})
		return path, mime, false
	}
	img, _, err := image.Decode(f)
	_ = f.Close()
	if err != nil {
		return path, mime, false
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return path, mime, false
	}
	maxDim := w
	if h > maxDim {
		maxDim = h
	}
	if maxDim <= maxEdge {
		return path, mime, false
	}
	scale := float64(maxEdge) / float64(maxDim)
	newW := int(math.Round(float64(w) * scale))
	newH := int(math.Round(float64(h) * scale))
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}
	dst := resizeRGBANearest(img, newW, newH)

	dir := filepath.Dir(path)
	tmpPath := filepath.Join(dir, "anyclaw-shrink-"+uuid.New().String())

	if rgbaHasTransparency(dst) {
		tmpPath += ".png"
		out, err := os.Create(tmpPath)
		if err != nil {
			logger.WarnCF("agent", "shrink image: create temp png", map[string]any{"error": err.Error()})
			return path, mime, false
		}
		if err := png.Encode(out, dst); err != nil {
			_ = out.Close()
			_ = os.Remove(tmpPath)
			logger.WarnCF("agent", "shrink image: encode png", map[string]any{"error": err.Error()})
			return path, mime, false
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(tmpPath)
			return path, mime, false
		}
		return tmpPath, "image/png", true
	}

	tmpPath += ".jpg"
	out, err := os.Create(tmpPath)
	if err != nil {
		logger.WarnCF("agent", "shrink image: create temp jpg", map[string]any{"error": err.Error()})
		return path, mime, false
	}
	if err := jpeg.Encode(out, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		_ = out.Close()
		_ = os.Remove(tmpPath)
		logger.WarnCF("agent", "shrink image: encode jpg", map[string]any{"error": err.Error()})
		return path, mime, false
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return path, mime, false
	}
	return tmpPath, "image/jpeg", true
}
