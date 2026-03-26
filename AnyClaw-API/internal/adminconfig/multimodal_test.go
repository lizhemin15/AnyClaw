package adminconfig

import (
	"strings"
	"testing"
)

func TestMoonshotLikeAPIBase(t *testing.T) {
	cases := []struct {
		base string
		want bool
	}{
		{"https://api.moonshot.cn/v1", true},
		{"https://api.moonshot.ai/v1", true},
		{"https://api.openai.com/v1", false},
	}
	for _, tc := range cases {
		if got := moonshotLikeAPIBase(tc.base); got != tc.want {
			t.Errorf("moonshotLikeAPIBase(%q)=%v want %v", tc.base, got, tc.want)
		}
	}
}

func TestBuildMultimodalImageChatBody(t *testing.T) {
	b, err := buildMultimodalImageChatBody("kimi-k2.5")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "image_url") || !strings.Contains(s, "data:image/png;base64,") {
		pl := 200
		if len(s) < pl {
			pl = len(s)
		}
		t.Fatalf("unexpected body: %s", s[:pl])
	}
}
