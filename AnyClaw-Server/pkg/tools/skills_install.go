package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw-server/pkg/fileutil"
	"github.com/anyclaw/anyclaw-server/pkg/logger"
	"github.com/anyclaw/anyclaw-server/pkg/skills"
	"github.com/anyclaw/anyclaw-server/pkg/utils"
)

// InstallSkillTool allows the LLM agent to install skills from registries.
// It shares the same RegistryManager that FindSkillsTool uses,
// so all registries configured in config are available for installation.
type InstallSkillTool struct {
	registryMgr *skills.RegistryManager
	workspace   string
	mu          sync.Mutex
}

// NewInstallSkillTool creates a new InstallSkillTool.
// registryMgr is the shared registry manager (same instance as FindSkillsTool).
// workspace is the root workspace directory; skills install to {workspace}/skills/{slug}/.
func NewInstallSkillTool(registryMgr *skills.RegistryManager, workspace string) *InstallSkillTool {
	return &InstallSkillTool{
		registryMgr: registryMgr,
		workspace:   workspace,
		mu:          sync.Mutex{},
	}
}

func (t *InstallSkillTool) Name() string {
	return "install_skill"
}

func (t *InstallSkillTool) Description() string {
	return "Install a skill from a registry by slug, or from GitHub. When registry fails, use github_repo. For agent-browser: slug=agent-browser, github_repo=openclaw/skills, github_repo_subpath=skills/bodietron/openclaw-agent-browser"
}

func (t *InstallSkillTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"slug": map[string]any{
				"type":        "string",
				"description": "The unique slug of the skill to install (e.g., 'github', 'agent-browser')",
			},
			"version": map[string]any{
				"type":        "string",
				"description": "Specific version to install (optional, defaults to latest)",
			},
			"registry": map[string]any{
				"type":        "string",
				"description": "Registry to install from (e.g., 'clawhub'). Omit when using github_repo.",
			},
			"github_repo": map[string]any{
				"type":        "string",
				"description": "GitHub repo (owner/repo) when registry fails. For agent-browser use openclaw/skills with github_repo_subpath",
			},
			"github_repo_subpath": map[string]any{
				"type":        "string",
				"description": "Subpath in monorepo, e.g. 'skills/bodietron/openclaw-agent-browser' for agent-browser from openclaw/skills",
			},
			"force": map[string]any{
				"type":        "boolean",
				"description": "Force reinstall if skill already exists (default false)",
			},
		},
		"required": []string{"slug"},
	}
}

func (t *InstallSkillTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	// Install lock to prevent concurrent directory operations.
	t.mu.Lock()
	defer t.mu.Unlock()

	slug, _ := args["slug"].(string)
	if err := utils.ValidateSkillIdentifier(slug); err != nil {
		return ErrorResult(fmt.Sprintf("invalid slug %q: error: %s", slug, err.Error()))
	}

	githubRepo, _ := args["github_repo"].(string)
	githubRepo = strings.TrimSpace(githubRepo)
	githubSubpath, _ := args["github_repo_subpath"].(string)
	githubSubpath = strings.TrimSpace(githubSubpath)
	version, _ := args["version"].(string)
	force, _ := args["force"].(bool)

	skillsDir := filepath.Join(t.workspace, "skills")
	targetDir := filepath.Join(skillsDir, slug)

	if !force {
		if _, err := os.Stat(targetDir); err == nil {
			return ErrorResult(
				fmt.Sprintf("skill %q already installed at %s. Use force=true to reinstall.", slug, targetDir),
			)
		}
	} else {
		os.RemoveAll(targetDir)
	}

	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return ErrorResult(fmt.Sprintf("failed to create skills directory: %v", err))
	}

	// GitHub install (fallback when registry fails or user provides github_repo)
	if githubRepo != "" {
		opts := &skills.InstallFromGitHubOpts{TargetDir: targetDir}
		if githubSubpath != "" {
			opts.Subpath = githubSubpath
		}
		if err := skills.NewSkillInstaller(t.workspace).InstallFromGitHub(ctx, githubRepo, opts); err != nil {
			os.RemoveAll(targetDir)
			return ErrorResult(fmt.Sprintf("failed to install from GitHub %q: %v", githubRepo, err))
		}
		output := fmt.Sprintf("Successfully installed skill from GitHub %q.\nLocation: %s\n", githubRepo, targetDir)
		return SilentResult(output)
	}

	// Registry install
	registryName, _ := args["registry"].(string)
	if registryName == "" {
		return ErrorResult("registry or github_repo required")
	}
	if err := utils.ValidateSkillIdentifier(registryName); err != nil {
		return ErrorResult(fmt.Sprintf("invalid registry %q: error: %s", registryName, err.Error()))
	}

	registry := t.registryMgr.GetRegistry(registryName)
	if registry == nil {
		return ErrorResult(fmt.Sprintf("registry %q not found", registryName))
	}

	result, err := registry.DownloadAndInstall(ctx, slug, version, targetDir)
	if err != nil {
		// Clean up partial install.
		rmErr := os.RemoveAll(targetDir)
		if rmErr != nil {
			logger.ErrorCF("tool", "Failed to remove partial install",
				map[string]any{
					"tool":       "install_skill",
					"target_dir": targetDir,
					"error":      rmErr.Error(),
				})
		}
		return ErrorResult(fmt.Sprintf("failed to install %q: %v", slug, err))
	}

	// Moderation: block malware.
	if result.IsMalwareBlocked {
		rmErr := os.RemoveAll(targetDir)
		if rmErr != nil {
			logger.ErrorCF("tool", "Failed to remove partial install",
				map[string]any{
					"tool":       "install_skill",
					"target_dir": targetDir,
					"error":      rmErr.Error(),
				})
		}
		return ErrorResult(fmt.Sprintf("skill %q is flagged as malicious and cannot be installed", slug))
	}

	// Write origin metadata.
	if err := writeOriginMeta(targetDir, registry.Name(), slug, result.Version); err != nil {
		logger.ErrorCF("tool", "Failed to write origin metadata",
			map[string]any{
				"tool":     "install_skill",
				"error":    err.Error(),
				"target":   targetDir,
				"registry": registry.Name(),
				"slug":     slug,
				"version":  result.Version,
			})
		_ = err
	}

	// Build result with moderation warning if suspicious.
	var output string
	if result.IsSuspicious {
		output = fmt.Sprintf("⚠️ Warning: skill %q is flagged as suspicious (may contain risky patterns).\n\n", slug)
	}
	output += fmt.Sprintf("Successfully installed skill %q v%s from %s registry.\nLocation: %s\n",
		slug, result.Version, registry.Name(), targetDir)

	if result.Summary != "" {
		output += fmt.Sprintf("Description: %s\n", result.Summary)
	}
	output += "\nThe skill is now available and can be loaded in the current session."

	return SilentResult(output)
}

// originMeta tracks which registry a skill was installed from.
type originMeta struct {
	Version          int    `json:"version"`
	Registry         string `json:"registry"`
	Slug             string `json:"slug"`
	InstalledVersion string `json:"installed_version"`
	InstalledAt      int64  `json:"installed_at"`
}

func writeOriginMeta(targetDir, registryName, slug, version string) error {
	meta := originMeta{
		Version:          1,
		Registry:         registryName,
		Slug:             slug,
		InstalledVersion: version,
		InstalledAt:      time.Now().UnixMilli(),
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	// Use unified atomic write utility with explicit sync for flash storage reliability.
	return fileutil.WriteFileAtomic(filepath.Join(targetDir, ".skill-origin.json"), data, 0o600)
}
