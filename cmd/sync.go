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

var syncCodexCmd = &cobra.Command{
	Use:   "codex",
	Short: "Import Codex logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()
		return sync.ImportCodexLogs(cmd.Context(), store, codexLogsPath)
	},
}

var syncOpenCodeCmd = &cobra.Command{
	Use:   "opencode",
	Short: "Import OpenCode logs",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()
		return sync.ImportOpenCodeLogs(cmd.Context(), store, opencodeDBPath)
	},
}

var claudeLogsPath string
var codexLogsPath string
var opencodeDBPath string

func init() {
	home, err := os.UserHomeDir()
	defaultClaudePath := filepath.Join(home, ".claude", "projects")
	defaultCodexPath := filepath.Join(home, ".codex", "sessions")
	defaultOpenCodeDBPath := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	if err != nil {
		defaultClaudePath = ".claude/projects"
		defaultCodexPath = ".codex/sessions"
		defaultOpenCodeDBPath = ".local/share/opencode/opencode.db"
	}
	syncClaudeCmd.Flags().StringVar(&claudeLogsPath, "path", defaultClaudePath, "Claude projects directory")
	syncCodexCmd.Flags().StringVar(&codexLogsPath, "path", defaultCodexPath, "Codex sessions directory")
	syncOpenCodeCmd.Flags().StringVar(&opencodeDBPath, "path", defaultOpenCodeDBPath, "OpenCode SQLite database path")
	syncCmd.AddCommand(syncClaudeCmd, syncCodexCmd, syncOpenCodeCmd)
}
