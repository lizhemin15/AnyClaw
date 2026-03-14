// AnyClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 AnyClaw contributors

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal"
	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal/agent"
	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal/auth"
	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal/cron"
	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal/gateway"
	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal/migrate"
	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal/onboard"
	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal/skills"
	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal/status"
	"github.com/anyclaw/anyclaw-server/cmd/anyclaw/internal/version"
)

func NewAnyClawCommand() *cobra.Command {
	short := fmt.Sprintf("%s OpenClaw - Personal AI Assistant v%s\n\n", internal.Logo, internal.GetVersion())

	cmd := &cobra.Command{
		Use:     "openclaw",
		Short:   short,
		Example: "openclaw version",
	}

	cmd.AddCommand(
		onboard.NewOnboardCommand(),
		onboard.NewSyncWorkspaceCommand(),
		agent.NewAgentCommand(),
		auth.NewAuthCommand(),
		gateway.NewGatewayCommand(),
		status.NewStatusCommand(),
		cron.NewCronCommand(),
		migrate.NewMigrateCommand(),
		skills.NewSkillsCommand(),
		version.NewVersionCommand(),
	)

	return cmd
}

const (
	colorBlue = "\033[1;38;2;62;93;185m"
	colorReset = "\033[0m"
	// AnyClaw: "Any" in blue, "Claw" in default
	banner = "\r\n" +
		colorBlue + " \u2588\u2588\u2588\u2588\u2588\u2588\u2557 \u2588\u2588\u2557   \u2588\u2588\u2557\u2588\u2588\u2557   \u2588\u2588\u2557" + colorReset + " \u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2557\u2588\u2588\u2557      \u2588\u2588\u2588\u2588\u2588\u2557 \u2588\u2588\u2557    \u2588\u2588\u2557\n" +
		colorBlue + "\u2588\u2588\u2554\u2550\u2550\u2550\u2550\u2588\u2588\u2557\u2588\u2588\u2588\u2588\u2557  \u2588\u2588\u2557\u2554\u2588\u2588\u2557 \u2588\u2588\u2554\u2550" + colorReset + "\u2588\u2588\u2554\u2550\u2550\u2550\u2550\u2550\u2588\u2588\u2557\u2588\u2588\u2557     \u2588\u2588\u2554\u2550\u2550\u2588\u2588\u2557\u2588\u2588\u2557    \u2588\u2588\u2557\n" +
		colorBlue + "\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2557\u2588\u2588\u2554\u2588\u2588\u2557 \u2588\u2588\u2557 \u2554\u2588\u2588\u2588\u2588\u2588\u2554\u2550 " + colorReset + "\u2588\u2588\u2557     \u2588\u2588\u2557     \u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2557\u2588\u2588\u2557 \u2588\u2557 \u2588\u2588\u2557\n" +
		colorBlue + "\u2588\u2588\u2554\u2550\u2550\u2550\u2588\u2588\u2557\u2588\u2588\u2557\u2554\u2588\u2588\u2557\u2588\u2588\u2557  \u2554\u2588\u2588\u2554\u2550  " + colorReset + "\u2588\u2588\u2557     \u2588\u2588\u2557     \u2588\u2588\u2554\u2550\u2550\u2588\u2588\u2557\u2588\u2588\u2557\u2588\u2588\u2588\u2557\u2588\u2588\u2557\n" +
		colorBlue + "\u2588\u2588\u2557  \u2588\u2588\u2557\u2588\u2588\u2557 \u2554\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2557   \u2588\u2588\u2557   " + colorReset + "\u2554\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2557\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2588\u2557\u2588\u2588\u2557  \u2588\u2588\u2588\u2588\u2554\u2588\u2588\u2588\u2554\u2550\n" +
		colorBlue + "\u2554\u2550\u2550\u2550  \u2554\u2550\u2550\u2550\u2554 \u2554\u2550\u2550\u2550\u2550  \u2554\u2550\u2550\u2550\u2550   \u2554\u2550\u2550\u2550   " + colorReset + " \u2554\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2554\u2554\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2550\u2554\u2554\u2550\u2550  \u2554\u2550\u2550\u2550\u2554\u2550\u2550\u2550\u2554\u2550\u2550\n " +
		"\r\n"
)

func main() {
	fmt.Printf("%s", banner)
	cmd := NewAnyClawCommand()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
