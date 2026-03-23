package weixinclaw

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var reHex32 = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

func buildCdnDownloadURL(encryptedQueryParam, cdnBase string) string {
	base := strings.TrimRight(strings.TrimSpace(cdnBase), "/")
	return base + "/download?encrypted_query_param=" + url.QueryEscape(encryptedQueryParam)
}

func buildCdnUploadURL(uploadParam, filekey, cdnBase string) string {
	base := strings.TrimRight(strings.TrimSpace(cdnBase), "/")
	return base + "/upload?encrypted_query_param=" + url.QueryEscape(uploadParam) + "&filekey=" + url.QueryEscape(filekey)
}

func parseAesKeyBase64(aesKeyBase64 string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(aesKeyBase64))
	if err != nil {
		return nil, err
	}
	if len(decoded) == 16 {
		return decoded, nil
	}
	if len(decoded) == 32 {
		s := string(decoded)
		if reHex32.MatchString(s) {
			return hex.DecodeString(s)
		}
	}
	return nil, fmt.Errorf("aes_key: expected 16 raw bytes or 32-char hex after base64, got %d bytes", len(decoded))
}

func downloadCdnBytes(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CDN GET %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func downloadAndDecryptCDN(ctx context.Context, encryptQuery, aesKeyBase64, cdnBase, label string) ([]byte, error) {
	key, err := parseAesKeyBase64(aesKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	u := buildCdnDownloadURL(encryptQuery, cdnBase)
	enc, err := downloadCdnBytes(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return aesDecryptECB(enc, key)
}

func downloadPlainCDN(ctx context.Context, encryptQuery, cdnBase, label string) ([]byte, error) {
	u := buildCdnDownloadURL(encryptQuery, cdnBase)
	raw, err := downloadCdnBytes(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return raw, nil
}

func uploadBufferToCDN(ctx context.Context, ciphertext []byte, uploadParam, filekey, cdnBase, label string) (downloadEncryptedParam string, err error) {
	u := buildCdnUploadURL(uploadParam, filekey, cdnBase)
	const maxRetries = 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(ciphertext))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		req.ContentLength = int64(len(ciphertext))

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return "", fmt.Errorf("%s: CDN client error %d: %s", label, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		if resp.StatusCode != http.StatusOK {
			msg := resp.Header.Get("x-error-message")
			if msg == "" {
				body, _ := io.ReadAll(resp.Body)
				msg = strings.TrimSpace(string(body))
			} else {
				io.Copy(io.Discard, resp.Body)
			}
			resp.Body.Close()
			lastErr = fmt.Errorf("CDN status %d: %s", resp.StatusCode, msg)
			continue
		}
		dp := resp.Header.Get("x-encrypted-param")
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if dp == "" {
			lastErr = fmt.Errorf("missing x-encrypted-param")
			continue
		}
		return dp, nil
	}
	if lastErr != nil {
		return "", fmt.Errorf("%s: %w", label, lastErr)
	}
	return "", fmt.Errorf("%s: upload failed", label)
}
