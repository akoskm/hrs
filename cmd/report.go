package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/akoskm/hrs/internal/db"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Show reports",
	RunE: func(cmd *cobra.Command, args []string) error {
		start, end, err := resolveReportRange()
		if err != nil {
			return err
		}
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		result, err := store.RangeReport(cmd.Context(), start, end)
		if err != nil {
			return err
		}
		if reportJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), formatReport(result))
		return err
	},
}

var (
	reportWeek  bool
	reportMonth bool
	reportYear  bool
	reportJSON  bool
	reportFrom  string
	reportTo    string
	reportNow   = time.Now
)

func init() {
	reportCmd.Flags().BoolVar(&reportWeek, "week", false, "show current week summary")
	reportCmd.Flags().BoolVar(&reportMonth, "month", false, "show current month summary")
	reportCmd.Flags().BoolVar(&reportYear, "year", false, "show current year summary")
	reportCmd.Flags().BoolVar(&reportJSON, "json", false, "output JSON")
	reportCmd.Flags().StringVar(&reportFrom, "from", "", "report start date (YYYY-MM-DD)")
	reportCmd.Flags().StringVar(&reportTo, "to", "", "report end date inclusive (YYYY-MM-DD)")
}

func resolveReportRange() (time.Time, time.Time, error) {
	modeCount := 0
	if reportWeek {
		modeCount++
	}
	if reportMonth {
		modeCount++
	}
	if reportYear {
		modeCount++
	}
	if reportFrom != "" || reportTo != "" {
		modeCount++
	}
	if modeCount > 1 {
		return time.Time{}, time.Time{}, fmt.Errorf("choose one of --week, --month, --year, or --from/--to")
	}
	if reportFrom != "" || reportTo != "" {
		if reportFrom == "" || reportTo == "" {
			return time.Time{}, time.Time{}, fmt.Errorf("--from and --to required together")
		}
		start, err := parseReportDate(reportFrom)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end, err := parseReportDate(reportTo)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if end.Before(start) {
			return time.Time{}, time.Time{}, fmt.Errorf("--to must be on or after --from")
		}
		return start, end.AddDate(0, 0, 1), nil
	}
	now := reportNow()
	if reportMonth {
		start, end := currentMonthRange(now)
		return start, end, nil
	}
	if reportYear {
		start, end := currentYearRange(now)
		return start, end, nil
	}
	start, end := currentWeekRange(now)
	return start, end, nil
}

func parseReportDate(value string) (time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse date %q: %w", value, err)
	}
	return parsed, nil
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

func currentMonthRange(now time.Time) (time.Time, time.Time) {
	loc := now.Location()
	start := time.Date(now.In(loc).Year(), now.In(loc).Month(), 1, 0, 0, 0, 0, loc)
	return start, start.AddDate(0, 1, 0)
}

func currentYearRange(now time.Time) (time.Time, time.Time) {
	loc := now.Location()
	start := time.Date(now.In(loc).Year(), time.January, 1, 0, 0, 0, 0, loc)
	return start, start.AddDate(1, 0, 0)
}

func formatReport(result db.ReportResult) string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf("Range: %s..%s\n\n", result.Range.From, result.Range.To))
	out.WriteString("Summary\n")
	out.WriteString(fmt.Sprintf("Total hours: %.1f\n", float64(result.Summary.TotalSecs)/3600))
	out.WriteString(fmt.Sprintf("Billable hours: %.1f\n", float64(result.Summary.BillableSecs)/3600))
	out.WriteString(fmt.Sprintf("Non-billable hours: %.1f\n", float64(result.Summary.NonBillableSecs)/3600))
	out.WriteString(fmt.Sprintf("Active days: %d\n", result.Summary.ActiveDays))
	out.WriteString(fmt.Sprintf("Average daily hours: %.1f\n\n", float64(result.Summary.AverageDailySecs)/3600))
	out.WriteString("By project\n")
	for _, row := range result.Projects {
		out.WriteString(fmt.Sprintf("%s %.1fh billable %.1fh %s\n", row.ProjectName, float64(row.TotalSecs)/3600, float64(row.BillableSecs)/3600, row.Currency))
	}
	out.WriteString("\nBy day\n")
	for _, row := range result.Days {
		out.WriteString(fmt.Sprintf("%s %.1fh\n", row.Date, float64(row.TotalSecs)/3600))
	}
	return strings.TrimRight(out.String(), "\n")
}
