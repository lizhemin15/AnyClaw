package onboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_Begin_Poll(t *testing.T) {
	var step int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/v1/app/registration" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		action := r.PostFormValue("action")
		switch action {
		case "init":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"supported_auth_methods": []string{"client_secret"},
			})
		case "begin":
			_ = json.NewEncoder(w).Encode(BeginResponse{
				DeviceCode:              "dev-1",
				VerificationURIComplete: "https://example.com/verify",
				Interval:                1,
				ExpireIn:                60,
			})
		case "poll":
			step++
			if step < 2 {
				_ = json.NewEncoder(w).Encode(PollResponse{Error: "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(PollResponse{
				ClientID:     "cli_test",
				ClientSecret: "sec_test",
			})
		default:
			t.Fatalf("unknown action %q", action)
		}
	}))
	t.Cleanup(srv.Close)

	c := &Client{
		HTTP:    srv.Client(),
		BaseURL: strings.TrimSuffix(srv.URL, "/"),
	}
	ctx := context.Background()

	if _, err := c.Init(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := c.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if b.DeviceCode != "dev-1" {
		t.Fatalf("device_code: %q", b.DeviceCode)
	}
	pr, err := c.Poll(ctx, b.DeviceCode)
	if err != nil || pr.Error != "authorization_pending" {
		t.Fatalf("poll1: %+v %v", pr, err)
	}
	pr, err = c.Poll(ctx, b.DeviceCode)
	if err != nil {
		t.Fatal(err)
	}
	if pr.ClientID != "cli_test" || pr.ClientSecret != "sec_test" {
		t.Fatalf("poll2: %+v", pr)
	}
}

func TestClient_postForm_BeginError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{HTTP: srv.Client(), BaseURL: strings.TrimSuffix(srv.URL, "/")}
	_, err := c.Begin(context.Background())
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected 400 error, got %v", err)
	}
}
