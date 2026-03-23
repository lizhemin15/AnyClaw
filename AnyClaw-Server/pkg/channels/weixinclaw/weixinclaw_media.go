package weixinclaw

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-server/pkg/bus"
	"github.com/anyclaw/anyclaw-server/pkg/channels"
	"github.com/anyclaw/anyclaw-server/pkg/logger"
	"github.com/anyclaw/anyclaw-server/pkg/media"
)

const (
	weixinMediaMaxBytes = 100 << 20
	// Same default as @tencent-weixin/openclaw-weixin auth/accounts.ts CDN_BASE_URL
	defaultWeixinCdnBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
)

// UploadMediaType values (ilink getuploadurl), see plugin api/types.ts
const (
	uploadMediaImage = 1
	uploadMediaVideo = 2
	uploadMediaFile  = 3
)

func (c *Channel) effectiveCdnBase() string {
	s := strings.TrimSpace(c.cdnBase)
	if s != "" {
		return strings.TrimRight(s, "/")
	}
	return strings.TrimRight(defaultWeixinCdnBaseURL, "/")
}

func isWeixinMediaItemType(t int) bool {
	return t == 2 || t == 3 || t == 4 || t == 5
}

func bodyFromWeixinItems(items []messageItem) string {
	for _, item := range items {
		if item.Type == 1 && item.TextItem != nil {
			text := strings.TrimSpace(item.TextItem.Text)
			ref := item.RefMsg
			if ref == nil {
				return text
			}
			if ref.MessageItem != nil && isWeixinMediaItemType(ref.MessageItem.Type) {
				return text
			}
			var parts []string
			if t := strings.TrimSpace(ref.Title); t != "" {
				parts = append(parts, t)
			}
			if ref.MessageItem != nil {
				sub := bodyFromWeixinItems([]messageItem{*ref.MessageItem})
				if sub != "" {
					parts = append(parts, sub)
				}
			}
			if len(parts) == 0 {
				return text
			}
			return "[引用: " + strings.Join(parts, " | ") + "]\n" + text
		}
		if item.Type == 3 && item.VoiceItem != nil {
			if vt := strings.TrimSpace(item.VoiceItem.Text); vt != "" {
				return vt
			}
		}
	}
	return ""
}

// pickInboundMediaItem mirrors plugin process-message.ts priority (main list, then quoted ref).
func pickInboundMediaItem(items []messageItem) *messageItem {
	find := func(pred func(messageItem) bool) *messageItem {
		for i := range items {
			if pred(items[i]) {
				return &items[i]
			}
		}
		return nil
	}
	if it := find(func(it messageItem) bool {
		return it.Type == 2 && it.ImageItem != nil && it.ImageItem.Media != nil &&
			strings.TrimSpace(it.ImageItem.Media.EncryptQuery) != ""
	}); it != nil {
		return it
	}
	if it := find(func(it messageItem) bool {
		return it.Type == 5 && it.VideoItem != nil && it.VideoItem.Media != nil &&
			strings.TrimSpace(it.VideoItem.Media.EncryptQuery) != ""
	}); it != nil {
		return it
	}
	if it := find(func(it messageItem) bool {
		return it.Type == 4 && it.FileItem != nil && it.FileItem.Media != nil &&
			strings.TrimSpace(it.FileItem.Media.EncryptQuery) != ""
	}); it != nil {
		return it
	}
	if it := find(func(it messageItem) bool {
		if it.Type != 3 || it.VoiceItem == nil || it.VoiceItem.Media == nil {
			return false
		}
		if strings.TrimSpace(it.VoiceItem.Text) != "" {
			return false
		}
		return strings.TrimSpace(it.VoiceItem.Media.EncryptQuery) != ""
	}); it != nil {
		return it
	}
	if it := find(func(it messageItem) bool {
		if it.Type != 1 || it.RefMsg == nil || it.RefMsg.MessageItem == nil {
			return false
		}
		return isWeixinMediaItemType(it.RefMsg.MessageItem.Type)
	}); it != nil {
		return it.RefMsg.MessageItem
	}
	return nil
}

func fetchDecryptedItemBytes(ctx context.Context, it *messageItem, cdnBase, label string) ([]byte, string, error) {
	if it == nil {
		return nil, "", fmt.Errorf("nil item")
	}
	switch it.Type {
	case 2:
		img := it.ImageItem
		if img == nil || img.Media == nil {
			return nil, "", fmt.Errorf("no image_item")
		}
		enc := strings.TrimSpace(img.Media.EncryptQuery)
		if enc == "" {
			return nil, "", fmt.Errorf("no encrypt_query_param")
		}
		aesHex := strings.TrimSpace(img.AesKeyHex)
		var plain []byte
		var err error
		if aesHex != "" {
			key, derr := hex.DecodeString(aesHex)
			if derr != nil || len(key) != 16 {
				return nil, "", fmt.Errorf("invalid image aeskey hex")
			}
			raw, derr := downloadCdnBytes(ctx, buildCdnDownloadURL(enc, cdnBase))
			if derr != nil {
				return nil, "", derr
			}
			plain, err = aesDecryptECB(raw, key)
		} else if img.Media.AesKey != "" {
			plain, err = downloadAndDecryptCDN(ctx, enc, img.Media.AesKey, cdnBase, label+" image")
		} else {
			plain, err = downloadPlainCDN(ctx, enc, cdnBase, label+" image-plain")
		}
		if err != nil {
			return nil, "", err
		}
		ext := sniffImageExt(plain)
		return plain, "image" + ext, nil
	case 3:
		v := it.VoiceItem
		if v == nil || v.Media == nil || strings.TrimSpace(v.Media.AesKey) == "" {
			return nil, "", fmt.Errorf("voice missing aes_key")
		}
		if strings.TrimSpace(v.Media.EncryptQuery) == "" {
			return nil, "", fmt.Errorf("voice missing encrypt_query_param")
		}
		plain, err := downloadAndDecryptCDN(ctx, strings.TrimSpace(v.Media.EncryptQuery), v.Media.AesKey, cdnBase, label+" voice")
		if err != nil {
			return nil, "", err
		}
		return plain, ".silk", nil
	case 4:
		f := it.FileItem
		if f == nil || f.Media == nil || strings.TrimSpace(f.Media.AesKey) == "" {
			return nil, "", fmt.Errorf("file missing aes_key")
		}
		if strings.TrimSpace(f.Media.EncryptQuery) == "" {
			return nil, "", fmt.Errorf("file missing encrypt_query_param")
		}
		plain, err := downloadAndDecryptCDN(ctx, strings.TrimSpace(f.Media.EncryptQuery), f.Media.AesKey, cdnBase, label+" file")
		if err != nil {
			return nil, "", err
		}
		name := strings.TrimSpace(f.FileName)
		if name == "" {
			name = "file.bin"
		}
		return plain, name, nil
	case 5:
		v := it.VideoItem
		if v == nil || v.Media == nil || strings.TrimSpace(v.Media.AesKey) == "" {
			return nil, "", fmt.Errorf("video missing aes_key")
		}
		if strings.TrimSpace(v.Media.EncryptQuery) == "" {
			return nil, "", fmt.Errorf("video missing encrypt_query_param")
		}
		plain, err := downloadAndDecryptCDN(ctx, strings.TrimSpace(v.Media.EncryptQuery), v.Media.AesKey, cdnBase, label+" video")
		if err != nil {
			return nil, "", err
		}
		return plain, ".mp4", nil
	default:
		return nil, "", fmt.Errorf("unsupported item type %d", it.Type)
	}
}

func sniffImageExt(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xD8 {
		return ".jpg"
	}
	if len(b) >= 8 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 {
		return ".png"
	}
	if len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")) {
		return ".webp"
	}
	return ".img"
}

func weixinMediaTagForItem(it *messageItem) string {
	if it == nil {
		return "[attachment]"
	}
	switch it.Type {
	case 2:
		return "[image: photo]"
	case 3:
		return "[audio]"
	case 4:
		return "[file]"
	case 5:
		return "[video]"
	default:
		return "[attachment]"
	}
}

func (c *Channel) storeInboundMedia(scope string, it *messageItem, data []byte, nameHint string) (ref string, err error) {
	if len(data) > weixinMediaMaxBytes {
		return "", fmt.Errorf("media too large")
	}
	store := c.GetMediaStore()
	if store == nil {
		return "", fmt.Errorf("media store not configured")
	}
	dir := filepath.Join(os.TempDir(), "ANYCLAW_media")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	filename := filepath.Base(strings.TrimSpace(nameHint))
	if filename == "" || filename == "." {
		filename = "weixin-media.bin"
	}
	if strings.HasPrefix(filename, ".") {
		filename = "weixin" + filename
	}
	local := filepath.Join(dir, fmt.Sprintf("weixin-in-%d-%s", time.Now().UnixNano(), sanitizeWeixinFilename(filename)))
	if err := os.WriteFile(local, data, 0o600); err != nil {
		return "", err
	}
	ct := guessContentType(it, nameHint)
	meta := media.MediaMeta{
		Filename:    filename,
		ContentType: ct,
		Source:      "weixinclaw",
	}
	return store.Store(local, meta, scope)
}

func sanitizeWeixinFilename(s string) string {
	s = strings.ReplaceAll(s, string(filepath.Separator), "_")
	s = strings.ReplaceAll(s, "..", "_")
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func guessContentType(it *messageItem, nameHint string) string {
	if it != nil && it.Type == 3 {
		return "audio/silk"
	}
	if it != nil && it.Type == 5 {
		return "video/mp4"
	}
	ext := strings.ToLower(filepath.Ext(nameHint))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}

// SendMedia implements channels.MediaSender (image / video / file; audio sent as file).
func (c *Channel) SendMedia(ctx context.Context, msg bus.OutboundMediaMessage) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}
	acc := c.pickAccountForPeer(msg.ChatID)
	if acc == nil {
		return fmt.Errorf("%w: no weixin account", channels.ErrSendFailed)
	}
	store := c.GetMediaStore()
	if store == nil {
		return fmt.Errorf("%w: media store not configured", channels.ErrSendFailed)
	}
	ctxTok := c.getCtxTok(acc.ID, msg.ChatID)
	cdn := c.effectiveCdnBase()

	want := 0
	sent := 0
	for _, p := range msg.Parts {
		if strings.TrimSpace(p.Ref) == "" {
			continue
		}
		want++
		path, meta, err := store.ResolveWithMeta(p.Ref)
		if err != nil {
			logger.WarnCF("weixinclaw", "SendMedia resolve ref", map[string]any{"ref": p.Ref, "err": err.Error()})
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			logger.ErrorCF("weixinclaw", "SendMedia read file", map[string]any{"path": path, "err": err.Error()})
			return channels.ErrTemporary
		}
		filename := meta.Filename
		if filename == "" {
			filename = p.Filename
		}
		if filename == "" {
			filename = filepath.Base(path)
		}
		mediaType := strings.TrimSpace(p.Type)
		if mediaType == "" {
			mediaType = inferOutboundMediaType(meta.Filename, meta.ContentType)
		}
		caption := strings.TrimSpace(p.Caption)
		if err := c.uploadAndSendOne(ctx, acc, msg.ChatID, ctxTok, cdn, data, filename, mediaType, caption); err != nil {
			return err
		}
		sent++
	}
	if want > 0 && sent == 0 {
		return fmt.Errorf("%w: no media part could be resolved", channels.ErrSendFailed)
	}
	return nil
}

func inferOutboundMediaType(filename, contentType string) string {
	ct := strings.ToLower(contentType)
	fn := strings.ToLower(filename)
	switch {
	case strings.HasPrefix(ct, "image/"):
		return "image"
	case strings.HasPrefix(ct, "video/"):
		return "video"
	case strings.HasPrefix(ct, "audio/"):
		return "audio"
	}
	ext := filepath.Ext(fn)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return "image"
	case ".mp4", ".mov", ".webm", ".mkv":
		return "video"
	default:
		return "file"
	}
}

func (c *Channel) uploadAndSendOne(ctx context.Context, acc *weixinAccount, to, ctxTok, cdn string, data []byte, filename, mediaType, caption string) error {
	var uploadKind int
	switch mediaType {
	case "image":
		uploadKind = uploadMediaImage
	case "video":
		uploadKind = uploadMediaVideo
	default:
		uploadKind = uploadMediaFile
	}

	filekeyBytes := make([]byte, 16)
	aesKey := make([]byte, 16)
	if _, err := rand.Read(filekeyBytes); err != nil {
		return channels.ErrTemporary
	}
	if _, err := rand.Read(aesKey); err != nil {
		return channels.ErrTemporary
	}
	filekey := hex.EncodeToString(filekeyBytes)
	rawSize := len(data)
	rawMD5 := fmt.Sprintf("%x", md5.Sum(data))
	cipherSize := aesECBPaddedSize(rawSize)

	body := map[string]any{
		"filekey":        filekey,
		"media_type":     uploadKind,
		"to_user_id":     to,
		"rawsize":        rawSize,
		"rawfilemd5":     rawMD5,
		"filesize":       cipherSize,
		"no_need_thumb":  true,
		"aeskey":         hex.EncodeToString(aesKey),
		"base_info":      weixinBaseInfo(),
	}
	raw, err := c.postJSON(ctx, acc, "ilink/bot/getuploadurl", body, apiTimeout)
	if err != nil {
		logger.ErrorCF("weixinclaw", "getuploadurl failed", map[string]any{"err": err.Error()})
		return channels.ErrTemporary
	}
	if err := weixinJSONBizError(raw); err != nil {
		logger.ErrorCF("weixinclaw", "getuploadurl biz error", map[string]any{"err": err.Error()})
		return channels.ErrTemporary
	}
	var up struct {
		UploadParam string `json:"upload_param"`
	}
	if err := json.Unmarshal(raw, &up); err != nil {
		return channels.ErrTemporary
	}
	if strings.TrimSpace(up.UploadParam) == "" {
		return fmt.Errorf("%w: empty upload_param", channels.ErrSendFailed)
	}
	ciphertext, err := aesEncryptECB(data, aesKey)
	if err != nil {
		return channels.ErrTemporary
	}
	downloadEnc, err := uploadBufferToCDN(ctx, ciphertext, up.UploadParam, filekey, cdn, "weixin-out")
	if err != nil {
		logger.ErrorCF("weixinclaw", "CDN upload failed", map[string]any{"err": err.Error()})
		return channels.ErrTemporary
	}
	// Match openclaw-weixin send.ts: Buffer.from(uploaded.aeskey).toString("base64") where aeskey is hex string.
	aesKeyWire := wireAesKeyForSend(aesKey)

	if strings.TrimSpace(caption) != "" {
		tbody := buildSendBody(to, stripMarkdownLite(caption), ctxTok)
		rawCap, errCap := c.postJSON(ctx, acc, "ilink/bot/sendmessage", tbody, apiTimeout)
		if errCap != nil {
			return channels.ErrTemporary
		}
		if err := weixinJSONBizError(rawCap); err != nil {
			logger.ErrorCF("weixinclaw", "sendmessage caption biz error", map[string]any{"err": err.Error()})
			return channels.ErrTemporary
		}
	}

	switch uploadKind {
	case uploadMediaImage:
		msgBody := buildImageSendBody(to, ctxTok, downloadEnc, aesKeyWire, cipherSize)
		var rawSend []byte
		rawSend, err = c.postJSON(ctx, acc, "ilink/bot/sendmessage", msgBody, apiTimeout)
		if err == nil {
			err = weixinJSONBizError(rawSend)
		}
	case uploadMediaVideo:
		msgBody := buildVideoSendBody(to, ctxTok, downloadEnc, aesKeyWire, cipherSize)
		var rawSend []byte
		rawSend, err = c.postJSON(ctx, acc, "ilink/bot/sendmessage", msgBody, apiTimeout)
		if err == nil {
			err = weixinJSONBizError(rawSend)
		}
	default:
		msgBody := buildFileSendBody(to, ctxTok, downloadEnc, aesKeyWire, filepath.Base(filename), rawSize)
		var rawSend []byte
		rawSend, err = c.postJSON(ctx, acc, "ilink/bot/sendmessage", msgBody, apiTimeout)
		if err == nil {
			err = weixinJSONBizError(rawSend)
		}
	}
	if err != nil {
		logger.ErrorCF("weixinclaw", "sendmessage media failed", map[string]any{"err": err.Error()})
		return channels.ErrTemporary
	}
	return nil
}

// wireAesKeyForSend matches @tencent-weixin/openclaw-weixin send.ts sendImageMessageWeixin:
// uploaded.aeskey is a hex string; CDNMedia.aes_key = Buffer.from(uploaded.aeskey).toString("base64") (utf-8 bytes of hex, then base64).
func wireAesKeyForSend(aesKey []byte) string {
	return base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(aesKey)))
}

func buildImageSendBody(to, ctxTok, encParam, aesKeyWire string, midSize int) map[string]any {
	msg := map[string]any{
		"from_user_id":  "",
		"to_user_id":    to,
		"client_id":     randomClientID(),
		"message_type":  2,
		"message_state": 2,
		"item_list": []map[string]any{{
			"type": 2,
			"image_item": map[string]any{
				"media": map[string]any{
					"encrypt_query_param": encParam,
					"aes_key":             aesKeyWire,
					"encrypt_type":        1,
				},
				"mid_size": midSize,
			},
		}},
	}
	if strings.TrimSpace(ctxTok) != "" {
		msg["context_token"] = ctxTok
	}
	return map[string]any{
		"msg":       msg,
		"base_info": weixinBaseInfo(),
	}
}

func buildVideoSendBody(to, ctxTok, encParam, aesKeyWire string, videoCipherSize int) map[string]any {
	msg := map[string]any{
		"from_user_id":  "",
		"to_user_id":    to,
		"client_id":     randomClientID(),
		"message_type":  2,
		"message_state": 2,
		"item_list": []map[string]any{{
			"type": 5,
			"video_item": map[string]any{
				"media": map[string]any{
					"encrypt_query_param": encParam,
					"aes_key":             aesKeyWire,
					"encrypt_type":        1,
				},
				"video_size": videoCipherSize,
			},
		}},
	}
	if strings.TrimSpace(ctxTok) != "" {
		msg["context_token"] = ctxTok
	}
	return map[string]any{
		"msg":       msg,
		"base_info": weixinBaseInfo(),
	}
}

func buildFileSendBody(to, ctxTok, encParam, aesKeyWire, fileName string, rawLen int) map[string]any {
	msg := map[string]any{
		"from_user_id":  "",
		"to_user_id":    to,
		"client_id":     randomClientID(),
		"message_type":  2,
		"message_state": 2,
		"item_list": []map[string]any{{
			"type": 4,
			"file_item": map[string]any{
				"media": map[string]any{
					"encrypt_query_param": encParam,
					"aes_key":             aesKeyWire,
					"encrypt_type":        1,
				},
				"file_name": fileName,
				"len":       fmt.Sprintf("%d", rawLen),
			},
		}},
	}
	if strings.TrimSpace(ctxTok) != "" {
		msg["context_token"] = ctxTok
	}
	return map[string]any{
		"msg":       msg,
		"base_info": weixinBaseInfo(),
	}
}
