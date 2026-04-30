package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
}

var projectTimeOffCmd = &cobra.Command{
	Use:   "time-off",
	Short: "Manage project time off",
}

var projectTimeOffAllowanceCmd = &cobra.Command{
	Use:   "allowance",
	Short: "Manage yearly time off allowances",
}

var projectAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		project, err := store.CreateProject(cmd.Context(), db.ProjectCreateInput{
			ClientName: strings.TrimSpace(projectAddClient),
			Name:       args[0],
			Code:       strings.TrimSpace(projectAddCode),
			HourlyRate: projectAddRate,
			Currency:   model.Currency(strings.ToUpper(strings.TrimSpace(projectAddCurrency))),
		})
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%d\t%s\t%s\n", project.Name, valueOrEmpty(project.Code), project.HourlyRate, project.Currency, projectAddClient)
		return err
	},
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		projects, err := store.ListProjectsWithClient(cmd.Context())
		if err != nil {
			return err
		}
		for _, project := range projects {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%d\t%s\t%t\t%s\n", project.Name, valueOrEmpty(project.Code), project.HourlyRate, project.Currency, project.BillableDefault, valueOrEmpty(project.ClientName)); err != nil {
				return err
			}
		}
		return nil
	},
}

var projectUpdateCmd = &cobra.Command{
	Use:   "update <code>",
	Short: "Update a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		project, err := store.UpdateProjectBillableDefault(cmd.Context(), strings.TrimSpace(args[0]), projectUpdateBillable)
		if err != nil {
			if errors.Is(err, db.ErrProjectNotFound) {
				return fmt.Errorf("project not found: %s", strings.TrimSpace(args[0]))
			}
			return err
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%d\t%s\t%t\n", project.Name, valueOrEmpty(project.Code), project.HourlyRate, project.Currency, project.BillableDefault)
		return err
	},
}

var projectTimeOffAllowanceSetCmd = &cobra.Command{
	Use:   "set <project> <type> <year> <days>",
	Short: "Set yearly allowance for a project time off type",
	Args:  cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		project, err := store.ProjectByCodeOrName(cmd.Context(), strings.TrimSpace(args[0]))
		if err != nil {
			return err
		}
		if err := store.EnsureProjectDefaultTimeOffTypes(cmd.Context(), project.ID); err != nil {
			return err
		}
		typeItem, err := store.TimeOffTypeByProjectAndName(cmd.Context(), project.ID, strings.TrimSpace(args[1]))
		if err != nil {
			return err
		}
		year, err := strconv.Atoi(strings.TrimSpace(args[2]))
		if err != nil || year <= 0 {
			return fmt.Errorf("invalid year: %s", strings.TrimSpace(args[2]))
		}
		days, err := strconv.Atoi(strings.TrimSpace(args[3]))
		if err != nil || days < 0 {
			return fmt.Errorf("invalid days: %s", strings.TrimSpace(args[3]))
		}
		allowance, err := store.UpsertTimeOffAllowance(cmd.Context(), project.ID, typeItem.ID, year, days)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%d\t%d\n", project.Name, typeItem.Name, allowance.Year, allowance.AllowedDays)
		return err
	},
}

var projectTimeOffAllowanceListCmd = &cobra.Command{
	Use:   "list <project> [year]",
	Short: "List yearly allowances for a project",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		project, err := store.ProjectByCodeOrName(cmd.Context(), strings.TrimSpace(args[0]))
		if err != nil {
			return err
		}
		year := time.Now().In(time.Local).Year()
		if len(args) == 2 {
			year, err = strconv.Atoi(strings.TrimSpace(args[1]))
			if err != nil || year <= 0 {
				return fmt.Errorf("invalid year: %s", strings.TrimSpace(args[1]))
			}
		}
		summaries, err := store.ListTimeOffAllowanceSummaries(cmd.Context(), year)
		if err != nil {
			return err
		}
		for _, summary := range summaries {
			if summary.ProjectID != project.ID {
				continue
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%d\t%d\t%d\t%d\n", project.Name, summary.TimeOffType, summary.Year, summary.AllowedDays, summary.UsedDays, summary.RemainingDays); err != nil {
				return err
			}
		}
		return nil
	},
}

var projectTimeOffAllowanceClearCmd = &cobra.Command{
	Use:   "clear <project> <type> <year>",
	Short: "Clear yearly allowance for a project time off type",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		project, err := store.ProjectByCodeOrName(cmd.Context(), strings.TrimSpace(args[0]))
		if err != nil {
			return err
		}
		if err := store.EnsureProjectDefaultTimeOffTypes(cmd.Context(), project.ID); err != nil {
			return err
		}
		typeItem, err := store.TimeOffTypeByProjectAndName(cmd.Context(), project.ID, strings.TrimSpace(args[1]))
		if err != nil {
			return err
		}
		year, err := strconv.Atoi(strings.TrimSpace(args[2]))
		if err != nil || year <= 0 {
			return fmt.Errorf("invalid year: %s", strings.TrimSpace(args[2]))
		}
		if err := store.DeleteTimeOffAllowance(cmd.Context(), project.ID, typeItem.ID, year); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%d\tcleared\n", project.Name, typeItem.Name, year)
		return err
	},
}

var (
	projectAddCode        string
	projectAddClient      string
	projectAddRate        int
	projectAddCurrency    string
	projectUpdateBillable bool
)

func init() {
	projectAddCmd.Flags().StringVar(&projectAddClient, "client", "", "client name")
	projectAddCmd.Flags().StringVar(&projectAddCode, "code", "", "project code")
	projectAddCmd.Flags().IntVar(&projectAddRate, "rate", 0, "hourly rate in smallest currency unit")
	projectAddCmd.Flags().StringVar(&projectAddCurrency, "currency", "EUR", "currency code")
	projectUpdateCmd.Flags().BoolVar(&projectUpdateBillable, "billable", true, "default billable flag")
	projectTimeOffAllowanceCmd.AddCommand(projectTimeOffAllowanceSetCmd, projectTimeOffAllowanceListCmd, projectTimeOffAllowanceClearCmd)
	projectTimeOffCmd.AddCommand(projectTimeOffAllowanceCmd)
	projectCmd.AddCommand(projectAddCmd, projectListCmd, projectUpdateCmd, projectTimeOffCmd)
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
