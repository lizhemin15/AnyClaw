package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tencentyun/cos-go-sdk-v5"

	"github.com/anyclaw/anyclaw-api/internal/config"
)

// UploadToCOS 上传文件到腾讯云 COS，返回可访问的 URL
func UploadToCOS(ctx context.Context, cfg *config.COSConfig, reader io.Reader, filename, contentType string) (string, error) {
	if cfg == nil || !cfg.Enabled || cfg.SecretID == "" || cfg.SecretKey == "" || cfg.Bucket == "" || cfg.Region == "" {
		return "", fmt.Errorf("COS not configured")
	}
	ext := path.Ext(filename)
	if ext == "" {
		ext = ".bin"
	}
	// 防止 path_prefix 含 .. 导致路径穿越
	prefix := strings.Trim(strings.TrimSpace(cfg.PathPrefix), "/")
	if prefix == "" || strings.Contains(prefix, "..") {
		prefix = "media"
	}
	key := prefix
	if key != "" {
		key += "/"
	}
	key += time.Now().Format("2006/01/02") + "/" + uuid.New().String() + ext

	bucketURL, _ := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", cfg.Bucket, cfg.Region))
	client := cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.SecretID,
			SecretKey: cfg.SecretKey,
		},
	})

	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType:   contentType,
			XOptionHeader: &http.Header{"x-cos-acl": []string{"public-read"}},
		},
	}
	_, err := client.Object.Put(ctx, key, reader, opt)
	if err != nil {
		return "", fmt.Errorf("COS upload: %w", err)
	}

	if cfg.Domain != "" {
		domain := strings.TrimSuffix(strings.TrimSpace(cfg.Domain), "/")
		if strings.HasPrefix(domain, "https://") || strings.HasPrefix(domain, "http://") {
			return domain + "/" + key, nil
		}
	}
	return fmt.Sprintf("https://%s.cos.%s.myqcloud.com/%s", cfg.Bucket, cfg.Region, key), nil
}
