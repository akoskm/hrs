package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/akoskm/hrs/internal/db"
	"github.com/akoskm/hrs/internal/model"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
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
	projectCmd.AddCommand(projectAddCmd, projectListCmd, projectUpdateCmd)
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
