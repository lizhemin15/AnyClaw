package media

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/request"
)

const maxUploadSize = 50 << 20 // 50MB

// Handler 媒体上传处理器
type Handler struct {
	db         *db.DB
	configPath string
}

// New 创建 Handler
func New(database *db.DB, configPath string) *Handler {
	return &Handler{db: database, configPath: configPath}
}

// UploadMedia 容器上传媒体到 COS，返回 URL
// 鉴权：Bearer <instance_token> 或 query token
func (h *Handler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	instanceIDStr := chi.URLParam(r, "id")
	token := ""
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimSpace(auth[7:])
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if instanceIDStr == "" || token == "" {
		http.Error(w, `{"error":"instance_id and token required"}`, http.StatusBadRequest)
		return
	}
	inst, err := h.db.GetInstanceByToken(token)
	if err != nil || inst == nil {
		http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
		return
	}
	instanceID, err := strconv.ParseInt(instanceIDStr, 10, 64)
	if err != nil || instanceID != inst.ID {
		http.Error(w, `{"error":"instance_id mismatch"}`, http.StatusForbidden)
		return
	}

	cfg, err := config.Load(h.configPath)
	if err != nil || cfg.COS == nil || !cfg.COS.Enabled {
		http.Error(w, `{"error":"COS not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "parse multipart: " + err.Error()})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing file"})
		return
	}
	defer file.Close()

	filename := strings.TrimSpace(header.Filename)
	if filename == "" || strings.ContainsAny(filename, "\x00") {
		filename = "file"
	} else {
		filename = filepath.Base(filename)
		if filename == "" || filename == "." || filename == ".." {
			filename = "file"
		}
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" || strings.ContainsAny(contentType, "\r\n") {
		contentType = "application/octet-stream"
	}

	fileURL, err := UploadToCOS(r.Context(), cfg.COS, file, filename, contentType)
	if err != nil {
		log.Printf("[media] COS upload failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "upload failed: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": fileURL, "filename": filename})
}

// UploadMediaAsUser 用户通过 JWT 鉴权上传媒体到 COS，返回 URL
func (h *Handler) UploadMediaAsUser(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	instanceIDStr := chi.URLParam(r, "id")
	instanceID, err := strconv.ParseInt(instanceIDStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid instance_id"}`, http.StatusBadRequest)
		return
	}
	inst, err := h.db.GetInstanceByID(instanceID)
	if err != nil || inst == nil {
		http.Error(w, `{"error":"instance not found"}`, http.StatusNotFound)
		return
	}
	if inst.UserID != claims.UserID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	cfg, err := config.Load(h.configPath)
	if err != nil || cfg.COS == nil || !cfg.COS.Enabled {
		http.Error(w, `{"error":"COS not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "parse multipart: " + err.Error()})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing file"})
		return
	}
	defer file.Close()

	filename := strings.TrimSpace(header.Filename)
	if filename == "" || strings.ContainsAny(filename, "\x00") {
		filename = "file"
	} else {
		filename = filepath.Base(filename)
		if filename == "" || filename == "." || filename == ".." {
			filename = "file"
		}
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" || strings.ContainsAny(contentType, "\r\n") {
		contentType = "application/octet-stream"
	}

	fileURL, err := UploadToCOS(r.Context(), cfg.COS, file, filename, contentType)
	if err != nil {
		log.Printf("[media] COS upload failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "upload failed: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": fileURL, "filename": filename})
}

// IsImageExt 是否图片扩展名
func IsImageExt(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" || ext == ".svg"
}

// IsVideoExt 是否视频扩展名
func IsVideoExt(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == ".mp4" || ext == ".webm" || ext == ".mov" || ext == ".ogg"
}

// IsAudioExt 是否音频扩展名
func IsAudioExt(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == ".mp3" || ext == ".wav" || ext == ".ogg" || ext == ".m4a"
}

// MediaRenderType 根据 URL 或文件名返回前端渲染类型
func MediaRenderType(urlOrFilename string) string {
	ext := strings.ToLower(filepath.Ext(urlOrFilename))
	if IsImageExt(ext) {
		return "image"
	}
	if IsVideoExt(ext) {
		return "video"
	}
	if IsAudioExt(ext) {
		return "audio"
	}
	return "file"
}
