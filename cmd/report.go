package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/akoskm/hrs/internal/db"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Show reports",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !reportWeek {
			return fmt.Errorf("--week required")
		}
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		start, end := currentWeekRange(reportNow())
		rows, err := store.WeekReport(cmd.Context(), start, end)
		if err != nil {
			return err
		}
		if reportJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(rows)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), formatWeekReport(rows))
		return err
	},
}

var (
	reportWeek bool
	reportJSON bool
	reportNow  = time.Now
)

func init() {
	reportCmd.Flags().BoolVar(&reportWeek, "week", false, "show current week summary")
	reportCmd.Flags().BoolVar(&reportJSON, "json", false, "output JSON")
}

func currentWeekRange(now time.Time) (time.Time, time.Time) {
	loc := now.Location()
	day := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 0, 0, 0, loc)
	weekday := int(day.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start := day.AddDate(0, 0, -(weekday - 1))
	return start, start.AddDate(0, 0, 7)
}

func formatWeekReport(rows []db.WeekReportRow) string {
	var out string
	out += "Project               Total  Billable NonBill Human  Agent  Earn\n"
	out += "---------------------------------------------------------------\n"
	for _, row := range rows {
		earn := float64(row.BillableSecs) / 3600 * float64(row.HourlyRate) / 100
		out += fmt.Sprintf("%-20s %5.1f %8.1f %7.1f %6.1f %6.1f %7.2f %s\n",
			row.ProjectName,
			float64(row.TotalSecs)/3600,
			float64(row.BillableSecs)/3600,
			float64(row.NonBillableSecs)/3600,
			float64(row.HumanSecs)/3600,
			float64(row.AgentSecs)/3600,
			earn,
			row.Currency,
		)
	}
	return out
}
