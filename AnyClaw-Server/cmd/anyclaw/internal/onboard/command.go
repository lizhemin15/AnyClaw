package onboard

import (
	"embed"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal"
	"github.com/anyclaw/anyclaw-server/pkg/config"
)

//go:generate cp -r ../../../../workspace .
//go:embed workspace
var embeddedFiles embed.FS

func NewOnboardCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "onboard",
		Aliases: []string{"o"},
		Short:   "Initialize OpenClaw configuration and workspace",
		Run: func(cmd *cobra.Command, args []string) {
			onboard()
		},
	}

	return cmd
}

// NewSyncWorkspaceCommand returns the "sync-workspace" subcommand.
// It merges built-in workspace files into the live workspace:
//   - skills/ directory is always overwritten (system-managed)
//   - other files (IDENTITY.md etc.) are only written when they don't exist yet
//
// Safe to call on every container start; idempotent.
func NewSyncWorkspaceCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sync-workspace",
		Short: "Sync built-in workspace files (skills) to the live workspace",
		Long: `Merge built-in workspace files from the current binary into the live workspace.

Rules:
  - skills/   always overwritten  (system skills shipped with AnyClaw)
  - other files (IDENTITY.md, USER.md …) written only when absent (preserves user edits)

Called automatically by the container entrypoint on every start.`,
		Run: func(cmd *cobra.Command, args []string) {
			cfg := config.DefaultConfig()
			workspace := cfg.WorkspacePath()
			if err := syncWorkspace(workspace); err != nil {
				fmt.Printf("sync-workspace error: %v\n", err)
				return
			}
			fmt.Printf("%s Workspace skills synced to %s\n", internal.Logo, workspace)
		},
	}
}
