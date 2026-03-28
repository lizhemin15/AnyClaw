package internal

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfigPath(t *testing.T) {
	t.Setenv("HOME", "/tmp/home")

	got := GetConfigPath()
	want := filepath.Join("/tmp/home", ".anyclaw", "config.json")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithANYCLAW_HOME(t *testing.T) {
	t.Setenv("ANYCLAW_HOME", "/custom/anyclaw")
	t.Setenv("HOME", "/tmp/home")

	got := GetConfigPath()
	want := filepath.Join("/custom/anyclaw", "config.json")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithANYCLAW_CONFIG(t *testing.T) {
	t.Setenv("ANYCLAW_CONFIG", "/custom/config.json")
	t.Setenv("ANYCLAW_HOME", "/custom/anyclaw")
	t.Setenv("HOME", "/tmp/home")

	got := GetConfigPath()
	want := "/custom/config.json"

	assert.Equal(t, want, got)
}

func TestFormatVersion_NonEmpty(t *testing.T) {
	s := FormatVersion()
	assert.NotEmpty(t, s)
}

func TestFormatBuildInfo_GoNonEmpty(t *testing.T) {
	_, goVer := FormatBuildInfo()
	assert.NotEmpty(t, goVer)
}

func TestGetConfigPath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific HOME behavior varies; run on windows")
	}

	testUserProfilePath := `C:\Users\Test`
	t.Setenv("USERPROFILE", testUserProfilePath)

	got := GetConfigPath()
	want := filepath.Join(testUserProfilePath, ".anyclaw", "config.json")

	require.True(t, strings.EqualFold(got, want), "GetConfigPath() = %q, want %q", got, want)
}

func TestGetVersion(t *testing.T) {
	assert.Equal(t, "dev", GetVersion())
}

func TestGetConfigPath_WithEnv(t *testing.T) {
	t.Setenv("ANYCLAW_CONFIG", "/tmp/custom/config.json")
	t.Setenv("HOME", "/tmp/home") // Also set home to ensure env is preferred

	got := GetConfigPath()
	want := "/tmp/custom/config.json"

	assert.Equal(t, want, got)
}
