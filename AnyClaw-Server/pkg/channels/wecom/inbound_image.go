package wecom

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyclaw/anyclaw-server/pkg/channels"
	"github.com/anyclaw/anyclaw-server/pkg/utils"
)

// DownloadWeComAppMediaFile fetches media by media_id using the corporate API and writes a temp file.
func DownloadWeComAppMediaFile(ctx context.Context, client *http.Client, accessToken, mediaID string) (path string, contentType string, err error) {
	u := fmt.Sprintf("%s/cgi-bin/media/get?access_token=%s&media_id=%s",
		wecomAPIBase, url.QueryEscape(accessToken), url.QueryEscape(mediaID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", "", fmt.Errorf("media get %d: %s", resp.StatusCode, string(b))
	}
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(strings.ToLower(ct), "application/json") || strings.Contains(strings.ToLower(ct), "text/plain") {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", "", fmt.Errorf("media get api error: %s", string(b))
	}
	ct = strings.TrimSpace(strings.Split(ct, ";")[0])
	if ct == "" {
		ct = "image/jpeg"
	}

	mediaDir := filepath.Join(os.TempDir(), "ANYCLAW_media")
	if err := os.MkdirAll(mediaDir, 0o700); err != nil {
		return "", "", err
	}
	ext := ".jpg"
	switch strings.ToLower(ct) {
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	}
	localPath := filepath.Join(mediaDir, utils.SanitizeFilename("wecom-app-"+mediaID+ext))
	out, err := os.Create(localPath)
	if err != nil {
		return "", "", err
	}
	_, err = io.Copy(out, io.LimitReader(resp.Body, channels.InboundHTTPMediaMaxBytes+1))
	out.Close()
	if err != nil {
		_ = os.Remove(localPath)
		return "", "", err
	}
	fi, statErr := os.Stat(localPath)
	if statErr != nil {
		_ = os.Remove(localPath)
		return "", "", statErr
	}
	if fi.Size() > channels.InboundHTTPMediaMaxBytes {
		_ = os.Remove(localPath)
		return "", "", fmt.Errorf("media file too large")
	}
	return localPath, ct, nil
}
