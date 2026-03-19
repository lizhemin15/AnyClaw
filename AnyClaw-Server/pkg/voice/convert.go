package voice

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

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
