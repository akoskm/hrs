package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/sync"
	"github.com/akoskm/hrs/internal/tui"
)

var rootCmd = &cobra.Command{
	Use:   "hrs",
	Short: "Track agent work",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		if fixturesPath != "" {
			if err := sync.ImportClaudeFixtures(ctx, store, fixturesPath); err != nil {
				return err
			}
		}
		model, err := tui.NewAppModelWithSync(ctx, store, func() error {
			return syncAllSources(ctx, store)
		})
		if err != nil {
			return err
		}
		model.InitializeTodayTimelineView()

		_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
		return err
	},
}

var dbPath string
var fixturesPath string

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDBPath(), "sqlite database path")
	rootCmd.PersistentFlags().StringVar(&fixturesPath, "fixtures", "", "fixture dir to import before opening TUI")
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(clientCmd)
	rootCmd.AddCommand(pathCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(entryCmd)
	rootCmd.AddCommand(reportCmd)
}

func defaultDBPath() string {
	if env := os.Getenv("HRS_DB"); env != "" {
		return env
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "hrs.db"
	}
	path := filepath.Join(configDir, "hrs", "hrs.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return "hrs.db"
	}
	return path
}

func syncAllSources(ctx context.Context, store *db.Store) error {
	if err := sync.ImportClaudeLogs(ctx, store, claudeLogsPath); err != nil {
		return err
	}
	if err := sync.ImportCodexLogs(ctx, store, codexLogsPath); err != nil {
		return err
	}
	if err := sync.ImportOpenCodeLogs(ctx, store, opencodeDBPath); err != nil {
		return err
	}
	return nil
}
