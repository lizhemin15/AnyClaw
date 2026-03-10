package configstore

import (
	"errors"
	"os"
	"path/filepath"

	anyclawconfig "github.com/anyclaw/anyclaw-server/pkg/config"
)

const (
	configDirName  = ".anyclaw"
	configFileName = "config.json"
)

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName), nil
}

func Load() (*anyclawconfig.Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	return anyclawconfig.LoadConfig(path)
}

func Save(cfg *anyclawconfig.Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return anyclawconfig.SaveConfig(path, cfg)
}
