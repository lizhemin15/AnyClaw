package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/anyclaw/anyclaw-server/pkg/config"
)

const Logo = "🦞"

var (
	version   = "dev"
	gitCommit string
	buildTime string
	goVersion string
)

// GetAnyClawHome returns the AnyClaw home directory.
// Priority: $ANYCLAW_HOME > ~/.anyclaw
func GetAnyClawHome() string {
	if home := os.Getenv("ANYCLAW_HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".anyclaw")
}

func GetConfigPath() string {
	if configPath := os.Getenv("ANYCLAW_CONFIG"); configPath != "" {
		return configPath
	}
	return filepath.Join(GetAnyClawHome(), "config.json")
}

func LoadConfig() (*config.Config, error) {
	return config.LoadConfig(GetConfigPath())
}

// FormatVersion returns the version string with optional git commit
func FormatVersion() string {
	v := version
	if gitCommit != "" {
		v += fmt.Sprintf(" (git: %s)", gitCommit)
	}
	return v
}

// FormatBuildInfo returns build time and go version info
func FormatBuildInfo() (string, string) {
	build := buildTime
	goVer := goVersion
	if goVer == "" {
		goVer = runtime.Version()
	}
	return build, goVer
}

// GetVersion returns the version string
func GetVersion() string {
	return version
}
