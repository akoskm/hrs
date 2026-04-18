package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

const testWorkflowFile = "test.yml"

var ciCmd = &cobra.Command{
	Use:   "ci",
	Short: "Run GitHub CI workflows",
}

var ciTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Trigger GitHub test workflow",
	RunE: func(cmd *cobra.Command, args []string) error {
		branch, err := currentGitBranch(cmd.Context())
		if err != nil {
			return err
		}
		output, err := runExternalCommand(cmd.Context(), "gh", "workflow", "run", testWorkflowFile, "--ref", branch)
		if err != nil {
			message := strings.TrimSpace(string(output))
			if message != "" {
				return fmt.Errorf("trigger test workflow: %s: %w", message, err)
			}
			return fmt.Errorf("trigger test workflow: %w", err)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Triggered GitHub tests for %s\n", branch)
		return err
	},
}

var currentGitBranch = defaultCurrentGitBranch
var runExternalCommand = defaultRunExternalCommand

func init() {
	ciCmd.AddCommand(ciTestCmd)
	rootCmd.AddCommand(ciCmd)
}

func defaultCurrentGitBranch(ctx context.Context) (string, error) {
	output, err := defaultRunExternalCommand(ctx, "git", "branch", "--show-current")
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(string(output))
	if branch == "" {
		return "", fmt.Errorf("resolve current git branch")
	}
	return branch, nil
}

func defaultRunExternalCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
