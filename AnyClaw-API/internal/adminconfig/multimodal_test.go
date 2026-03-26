package adminconfig

import (
	"strings"
	"testing"
)

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
