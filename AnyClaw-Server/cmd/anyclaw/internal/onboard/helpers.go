package onboard

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal"
	"github.com/anyclaw/anyclaw-server/pkg/config"
)

func onboard() {
	configPath := internal.GetConfigPath()

	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Config already exists at %s\n", configPath)
		fmt.Print("Overwrite? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" {
			fmt.Println("Aborted.")
			return
		}
	}

	cfg := config.DefaultConfig()
	if err := config.SaveConfig(configPath, cfg); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	workspace := cfg.WorkspacePath()
	createWorkspaceTemplates(workspace)

	fmt.Printf("%s OpenClaw is ready!\n", internal.Logo)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Add your API key to", configPath)
	fmt.Println("")
	fmt.Println("     Recommended:")
	fmt.Println("     - OpenRouter: https://openrouter.ai/keys (access 100+ models)")
	fmt.Println("     - Ollama:     https://ollama.com (local, free)")
	fmt.Println("")
	fmt.Println("     See README.md for 17+ supported providers.")
	fmt.Println("")
	fmt.Println("  2. Chat: openclaw agent -m \"Hello!\"")
}

func createWorkspaceTemplates(workspace string) {
	err := copyEmbeddedToTarget(workspace)
	if err != nil {
		fmt.Printf("Error copying workspace templates: %v\n", err)
	}
}

func copyEmbeddedToTarget(targetDir string) error {
	return walkEmbedded(targetDir, func(_ string) bool { return true })
}

// syncWorkspace merges embedded workspace files into targetDir.
//
// Overwrite policy:
//   - files under skills/ are always overwritten (system skills owned by AnyClaw)
//   - all other files are written only when they do not yet exist (preserves user edits)
func syncWorkspace(targetDir string) error {
	return walkEmbedded(targetDir, func(relPath string) bool {
		return strings.HasPrefix(filepath.ToSlash(relPath), "skills/")
	})
}

// walkEmbedded copies every embedded workspace file to targetDir.
// shouldOverwrite(relPath) controls per-file overwrite behaviour.
func walkEmbedded(targetDir string, shouldOverwrite func(relPath string) bool) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	return fs.WalkDir(embeddedFiles, "workspace", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := embeddedFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}

		relPath, err := filepath.Rel("workspace", path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		targetPath := filepath.Join(targetDir, relPath)

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(targetPath), err)
		}

		if !shouldOverwrite(relPath) {
			if _, statErr := os.Stat(targetPath); statErr == nil {
				return nil // file exists and user may have edited it – skip
			}
		}

		if err := os.WriteFile(targetPath, data, 0o644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", targetPath, err)
		}

		return nil
	})
}
