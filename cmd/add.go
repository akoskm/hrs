package cmd

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/akoskm/hrs/internal/db"
)

var addCmd = &cobra.Command{
	Use:   "add [flags] <description>",
	Short: "Add a manual time entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		startedAt, endedAt, err := parseLocalRange(addFrom, addTo)
		if err != nil {
			return err
		}
		entry, err := store.CreateManualEntry(cmd.Context(), db.ManualEntryInput{
			ProjectIdent: strings.TrimSpace(addProject),
			Description:  args[0],
			StartedAt:    startedAt,
			EndedAt:      endedAt,
			Billable:     addBillable,
		})
		if err != nil {
			if errors.Is(err, db.ErrProjectNotFound) {
				return fmt.Errorf("project not found: %s", strings.TrimSpace(addProject))
			}
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%t\t%s\n", valueOrEmpty(entry.Description), startedAt.Format(time.RFC3339), endedAt.Format(time.RFC3339), entry.Status, entry.Billable, valueOrEmpty(entry.ProjectID))
		return err
	},
}

var (
	addProject  string
	addFrom     string
	addTo       string
	addBillable *bool
)

func init() {
	addCmd.Flags().StringVar(&addProject, "project", "", "project code or name")
	addCmd.Flags().StringVar(&addFrom, "from", "", "local start time, eg 09:00")
	addCmd.Flags().StringVar(&addTo, "to", "", "local end time, eg 11:00")
	addCmd.Flags().Bool("billable", false, "override billable flag")
	_ = addCmd.Flags().MarkHidden("billable")
	addCmd.Flags().Lookup("billable").NoOptDefVal = "true"
	addCmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(addProject) == "" {
			return fmt.Errorf("--project required")
		}
		if strings.TrimSpace(addFrom) == "" || strings.TrimSpace(addTo) == "" {
			return fmt.Errorf("--from and --to required")
		}
		if cmd.Flags().Changed("billable") {
			v, err := cmd.Flags().GetBool("billable")
			if err != nil {
				return err
			}
			addBillable = &v
		} else {
			addBillable = nil
		}
		return nil
	}
}

func parseLocalRange(from, to string) (time.Time, time.Time, error) {
	loc := time.Now().Location()
	day := time.Now().In(loc).Format("2006-01-02")
	startedAt, err := time.ParseInLocation("2006-01-02 15:04", day+" "+strings.TrimSpace(from), loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid --from: %w", err)
	}
	endedAt, err := time.ParseInLocation("2006-01-02 15:04", day+" "+strings.TrimSpace(to), loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid --to: %w", err)
	}
	if !endedAt.After(startedAt) {
		return time.Time{}, time.Time{}, fmt.Errorf("--to must be after --from")
	}
	return startedAt, endedAt, nil
}
