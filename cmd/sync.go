package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/sync"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Import agent logs",
}

var syncClaudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Import Claude Code logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()
		return sync.ImportClaudeLogs(cmd.Context(), store, claudeLogsPath)
	},
}

var claudeLogsPath string

func init() {
	home, err := os.UserHomeDir()
	defaultClaudePath := filepath.Join(home, ".claude", "projects")
	if err != nil {
		defaultClaudePath = ".claude/projects"
	}
	syncClaudeCmd.Flags().StringVar(&claudeLogsPath, "path", defaultClaudePath, "Claude projects directory")
	syncCmd.AddCommand(syncClaudeCmd)
}
