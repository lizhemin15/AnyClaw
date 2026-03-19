package voice

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	// FeishuVoiceMaxDurationSeconds is the max duration (seconds) for native Feishu voice messages.
	// Longer audio is sent as file.
	FeishuVoiceMaxDurationSeconds = 60
)

// GetAudioDuration returns the duration of an audio file in seconds using ffprobe.
// Returns (0, false) when ffprobe is unavailable or fails (caller should treat as unknown).
func GetAudioDuration(inputPath string) (seconds float64, ok bool) {
	ffprobePath, lookErr := exec.LookPath("ffprobe")
	if lookErr != nil {
		return 0, false
	}
	cmd := exec.Command(ffprobePath, "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", inputPath)
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, false
	}
	sec, parseErr := strconv.ParseFloat(s, 64)
	if parseErr != nil || sec < 0 {
		return 0, false
	}
	return sec, true
}

// ConvertToOpus converts an audio file to Opus format (OGG container) using ffmpeg.
// Returns the path to the converted temp file. The caller is responsible for removing it.
// Returns ("", false, nil) when ffmpeg is not available (graceful degradation).
// Returns ("", false, err) when ffmpeg is available but conversion fails.
func ConvertToOpus(inputPath string) (outputPath string, ok bool, err error) {
	ffmpegPath, lookErr := exec.LookPath("ffmpeg")
	if lookErr != nil {
		return "", false, nil // ffmpeg not installed, skip conversion
	}

	tmp, err := os.CreateTemp("", "anyclaw_opus_*.ogg")
	if err != nil {
		return "", false, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmp.Close()
	outputPath = tmp.Name()

	// -y: overwrite output, -i: input, -c:a libopus: encode to opus, -vn: no video
	cmd := exec.Command(ffmpegPath, "-y", "-i", inputPath, "-c:a", "libopus", "-vn", outputPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(outputPath)
		return "", false, fmt.Errorf("ffmpeg conversion failed: %w\noutput: %s", err, strings.TrimSpace(string(out)))
	}
	return outputPath, true, nil
}
