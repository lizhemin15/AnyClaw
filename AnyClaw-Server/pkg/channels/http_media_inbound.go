package channels

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyclaw/anyclaw-server/pkg/logger"
	"github.com/anyclaw/anyclaw-server/pkg/media"
	"github.com/anyclaw/anyclaw-server/pkg/utils"
)

// InboundHTTPMediaMaxBytes caps downloads from user-provided URLs (web / WeCom link).
const InboundHTTPMediaMaxBytes = 20 << 20

// AppendImageMediaPlaceholder appends the same routing tag as Feishu/Telegram for vision.
func AppendImageMediaPlaceholder(userText string) string {
	tag := "[image: photo]"
	t := strings.TrimSpace(userText)
	if t == "" {
		return tag
	}
	return t + " " + tag
}

// DownloadHTTPURLToMediaStore downloads a URL to a temp file and registers it in the store.
func DownloadHTTPURLToMediaStore(
	ctx context.Context,
	client *http.Client,
	store media.MediaStore,
	rawURL, scope, filename, contentType, source string,
) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return "", fmt.Errorf("empty url")
	}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("http %d: %s", resp.StatusCode, string(slurp))
	}
	ct := contentType
	if ct == "" {
		ct = resp.Header.Get("Content-Type")
		ct = strings.TrimSpace(strings.Split(ct, ";")[0])
	}
	if ct == "" || ct == "application/octet-stream" {
		ct = sniffImageContentTypeFromFilename(filename)
	}

	mediaDir := filepath.Join(os.TempDir(), "ANYCLAW_media")
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		return "", err
	}
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = extForImageMIME(ct)
	}
	localPath := filepath.Join(mediaDir, utils.SanitizeFilename(source+"-dl-"+filename+ext))
	out, err := os.Create(localPath)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(out, io.LimitReader(resp.Body, InboundHTTPMediaMaxBytes+1))
	out.Close()
	if err != nil {
		_ = os.Remove(localPath)
		return "", err
	}
	if n > InboundHTTPMediaMaxBytes {
		_ = os.Remove(localPath)
		return "", fmt.Errorf("download too large")
	}
	ref, err := store.Store(localPath, media.MediaMeta{
		Filename:    filepath.Base(localPath),
		ContentType: ct,
		Source:      source,
	}, scope)
	if err != nil {
		_ = os.Remove(localPath)
		return "", err
	}
	logger.DebugCF("channels", "inbound http media stored", map[string]any{"source": source, "ref": ref})
	return ref, nil
}

func sniffImageContentTypeFromFilename(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

func extForImageMIME(mime string) string {
	switch strings.ToLower(mime) {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}
