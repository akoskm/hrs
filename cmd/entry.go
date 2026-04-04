package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/akoskm/hrs/internal/db"
)

var entryCmd = &cobra.Command{
	Use:   "entry",
	Short: "Manage time entries",
}

var entryUpsertCmd = &cobra.Command{
	Use:   "upsert",
	Short: "Upsert an agent entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		startedAt, endedAt, err := parseLocalRange(entryDate, entryFrom, entryTo)
		if err != nil {
			return err
		}
		entry, err := store.UpsertAgentEntry(cmd.Context(), db.AgentEntryUpsertInput{
			ProjectIdent: strings.TrimSpace(entryProject),
			Description:  strings.TrimSpace(entryDescription),
			StartedAt:    startedAt,
			EndedAt:      endedAt,
			Operator:     strings.TrimSpace(entrySource),
			SourceRef:    strings.TrimSpace(entrySourceRef),
			GitBranch:    strings.TrimSpace(entryBranch),
			Cwd:          strings.TrimSpace(entryCwd),
			Metadata:     map[string]any{},
		})
		if err != nil {
			if errors.Is(err, db.ErrProjectNotFound) {
				return fmt.Errorf("project not found: %s", strings.TrimSpace(entryProject))
			}
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\n", entry.Operator, valueOrEmpty(entry.Description), startedAt.Format(time.RFC3339), endedAt.Format(time.RFC3339), valueOrEmpty(entry.SourceRef))
		return err
	},
}

var (
	entryProject     string
	entryDate        string
	entryFrom        string
	entryTo          string
	entryDescription string
	entrySource      string
	entrySourceRef   string
	entryBranch      string
	entryCwd         string
)

func init() {
	entryUpsertCmd.Flags().StringVar(&entryProject, "project", "", "project code or name")
	entryUpsertCmd.Flags().StringVar(&entryDate, "date", "", "entry date, eg 2026-04-02")
	entryUpsertCmd.Flags().StringVar(&entryFrom, "from", "", "local start time, eg 09:00")
	entryUpsertCmd.Flags().StringVar(&entryTo, "to", "", "local end time, eg 11:00")
	entryUpsertCmd.Flags().StringVar(&entryDescription, "description", "", "entry description")
	entryUpsertCmd.Flags().StringVar(&entrySource, "source", "", "entry source/operator")
	entryUpsertCmd.Flags().StringVar(&entrySourceRef, "source-ref", "", "stable source reference")
	entryUpsertCmd.Flags().StringVar(&entryBranch, "branch", "", "git branch")
	entryUpsertCmd.Flags().StringVar(&entryCwd, "cwd", "", "working directory")
	entryUpsertCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(entrySource) == "" {
			return fmt.Errorf("--source required")
		}
		if strings.TrimSpace(entrySourceRef) == "" {
			return fmt.Errorf("--source-ref required")
		}
		if strings.TrimSpace(entryDescription) == "" {
			return fmt.Errorf("--description required")
		}
		if strings.TrimSpace(entryFrom) == "" || strings.TrimSpace(entryTo) == "" {
			return fmt.Errorf("--from and --to required")
		}
		return nil
	}
	entryCmd.AddCommand(entryUpsertCmd)
}
