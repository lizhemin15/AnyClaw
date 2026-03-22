// Package onboard implements Feishu app registration via device flow, matching
// @larksuite/openclaw-lark-tools (POST /oauth/v1/app/registration).
package onboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var feishuAccountsBase = map[string]string{
	"prod": "https://accounts.feishu.cn",
	"boe":  "https://accounts.feishu-boe.cn",
	"pre":  "https://accounts.feishu-pre.cn",
}

var larkAccountsBase = map[string]string{
	"prod": "https://accounts.larksuite.com",
	"boe":  "https://accounts.larksuite-boe.com",
	"pre":  "https://accounts.larksuite-pre.com",
}

// Client calls Feishu/Lark account registration endpoints.
type Client struct {
	HTTP    *http.Client
	BaseURL string
	Lane    string
}

// InitResponse is returned by action=init.
type InitResponse struct {
	SupportedAuthMethods []string `json:"supported_auth_methods"`
}

// BeginResponse is returned by action=begin.
type BeginResponse struct {
	DeviceCode               string `json:"device_code"`
	VerificationURIComplete  string `json:"verification_uri_complete"`
	Interval                 int    `json:"interval"`
	ExpireIn                 int    `json:"expire_in"`
}

// PollResponse is returned by action=poll (including error-shaped bodies).
type PollResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	UserInfo     struct {
		OpenID       string `json:"open_id"`
		TenantBrand  string `json:"tenant_brand"`
	} `json:"user_info"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// NewClient returns a client for env (prod|boe|pre) on Feishu China accounts host.
func NewClient(env string) *Client {
	if env == "" {
		env = "prod"
	}
	base := feishuAccountsBase[env]
	if base == "" {
		base = feishuAccountsBase["prod"]
	}
	return &Client{
		HTTP:    &http.Client{Timeout: 15 * time.Second},
		BaseURL: base,
	}
}

func (c *Client) postForm(ctx context.Context, form url.Values) (statusCode int, body []byte, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/oauth/v1/app/registration", strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.Lane != "" {
		req.Header.Set("x-tt-env", c.Lane)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	body, err = io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, nil, err
	}
	return res.StatusCode, body, nil
}

// Init checks supported auth methods (optional; mirrors official CLI).
func (c *Client) Init(ctx context.Context) (*InitResponse, error) {
	status, body, err := c.postForm(ctx, url.Values{"action": {"init"}})
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("init HTTP %d: %s", status, strings.TrimSpace(string(body)))
	}
	var out InitResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("init decode: %w", err)
	}
	return &out, nil
}

// Begin starts device registration; user should open verification_uri_complete (QR encodes this URL).
func (c *Client) Begin(ctx context.Context) (*BeginResponse, error) {
	form := url.Values{
		"action":              {"begin"},
		"archetype":           {"PersonalAgent"},
		"auth_method":         {"client_secret"},
		"request_user_info":   {"open_id"},
	}
	status, body, err := c.postForm(ctx, form)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("begin HTTP %d: %s", status, strings.TrimSpace(string(body)))
	}
	var out BeginResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("begin decode: %w", err)
	}
	if out.DeviceCode == "" || out.VerificationURIComplete == "" {
		return nil, fmt.Errorf("begin: missing device_code or verification_uri_complete in response")
	}
	if out.Interval <= 0 {
		out.Interval = 5
	}
	if out.ExpireIn <= 0 {
		out.ExpireIn = 600
	}
	return &out, nil
}

// Poll checks registration status (including authorization_pending in JSON body).
func (c *Client) Poll(ctx context.Context, deviceCode string) (*PollResponse, error) {
	_, body, err := c.postForm(ctx, url.Values{
		"action":      {"poll"},
		"device_code": {deviceCode},
	})
	if err != nil {
		return nil, err
	}
	var out PollResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("poll decode: %w (body %q)", err, strings.TrimSpace(string(body)))
	}
	return &out, nil
}

// UseLarkInternational switches base URL to Lark (international) accounts host.
func (c *Client) UseLarkInternational(env string) {
	if env == "" {
		env = "prod"
	}
	base := larkAccountsBase[env]
	if base == "" {
		base = larkAccountsBase["prod"]
	}
	c.BaseURL = base
}
