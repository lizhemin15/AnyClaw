package voice

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/llm"
)

type TokenResolver interface {
	ResolveToken(token string) (instanceID, userID string, ok bool)
}

type Handler struct {
	configPath string
	scheduler  *llm.ModelScheduler
	resolver   TokenResolver
	client     *http.Client
}

func New(configPath string, scheduler *llm.ModelScheduler, resolver TokenResolver) *Handler {
	return &Handler{
		configPath: configPath,
		scheduler:  scheduler,
		resolver:   resolver,
		client:     &http.Client{Timeout: 120 * time.Second},
	}
}

func (h *Handler) HandleASR(w http.ResponseWriter, r *http.Request) {
	h.proxyVoice(w, r, "/audio/transcriptions")
}

func (h *Handler) HandleTTS(w http.ResponseWriter, r *http.Request) {
	h.proxyVoice(w, r, "/audio/speech")
}

func (h *Handler) proxyVoice(w http.ResponseWriter, r *http.Request, path string) {
	token := extractBearer(r)
	if token == "" {
		http.Error(w, `{"error":{"message":"missing authorization"}}`, http.StatusUnauthorized)
		return
	}
	if h.resolver != nil {
		if _, _, ok := h.resolver.ResolveToken(token); !ok {
			http.Error(w, `{"error":{"message":"invalid token"}}`, http.StatusUnauthorized)
			return
		}
	}

	cfg, err := config.Load(h.configPath)
	if err != nil {
		http.Error(w, `{"error":{"message":"config error"}}`, http.StatusInternalServerError)
		return
	}

	candidates := cfg.FindVoiceAPIEndpoints()
	if len(candidates) == 0 {
		http.Error(w, `{"error":{"message":"no voice api endpoint configured"}}`, http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":{"message":"bad request"}}`, http.StatusBadRequest)
		return
	}

	type attempt struct {
		statusCode int
		respHeader http.Header
		respBody   []byte
	}
	var final *attempt

	for try := 0; try < len(candidates); try++ {
		ep, ok := h.scheduler.Pick("voice", candidates)
		if !ok {
			break
		}

		reqURL := strings.TrimSuffix(ep.APIBase, "/") + path
		proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, reqURL, bytes.NewReader(body))
		if err != nil {
			h.scheduler.Done(ep)
			break
		}

		if ct := r.Header.Get("Content-Type"); ct != "" {
			proxyReq.Header.Set("Content-Type", ct)
		}
		proxyReq.Header.Set("Authorization", "Bearer "+ep.APIKey)
		if u, err := url.Parse(reqURL); err == nil && u.Host != "" {
			host := u.Hostname()
			if p := u.Port(); p != "" && p != "443" && p != "80" {
				host = u.Host
			}
			proxyReq.Host = host
		}

		resp, err := h.client.Do(proxyReq)
		h.scheduler.Done(ep)
		if err != nil {
			log.Printf("[voice] upstream error (try %d): endpoint=%s err=%v", try+1, ep.ChannelName, err)
			h.scheduler.RecordFailureUntil(ep, time.Now().Add(llm.CooldownTransient))
			continue
		}

		rb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			log.Printf("[voice] upstream error (try %d): endpoint=%s status=%d", try+1, ep.ChannelName, resp.StatusCode)
			h.scheduler.RecordFailureUntil(ep, time.Now().Add(llm.CooldownTransient))
			if try < len(candidates)-1 {
				continue
			}
		}

		final = &attempt{statusCode: resp.StatusCode, respHeader: resp.Header, respBody: rb}
		break
	}

	if final == nil {
		http.Error(w, `{"error":{"message":"all voice api endpoints failed"}}`, http.StatusBadGateway)
		return
	}

	for k, v := range final.respHeader {
		lower := strings.ToLower(k)
		if lower == "content-type" || lower == "content-length" || lower == "content-disposition" {
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
	}
	w.WriteHeader(final.statusCode)
	w.Write(final.respBody)
}

func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}
