package voice

import "testing"

func TestNormalizeXiaomiMisformedStyleTag(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"<Happy>明天真好", "<style>Happy</style>明天真好"},
		{"  <Whisper>  悄悄话", "<style>Whisper</style>悄悄话"},
		{"<style>Happy</style>OK", "<style>Happy</style>OK"},
		{"plain text", "plain text"},
		{"<not closed", "<not closed"},
	}
	for _, tc := range tests {
		got := NormalizeXiaomiMisformedStyleTag(tc.in)
		if got != tc.want {
			t.Errorf("in=%q got=%q want=%q", tc.in, got, tc.want)
		}
	}
}
