package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/akoskm/hrs/internal/db"
)

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Manage project path mappings",
}

var pathAddCmd = &cobra.Command{
	Use:   "add <dir> <project>",
	Short: "Map a directory to a project",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		mapping, err := store.AddProjectPath(cmd.Context(), strings.TrimSpace(args[1]), filepath.Clean(args[0]))
		if err != nil {
			if errors.Is(err, db.ErrProjectNotFound) {
				return fmt.Errorf("project not found: %s", strings.TrimSpace(args[1]))
			}
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", mapping.Path, mapping.ProjectID)
		return err
	},
}

func init() {
	pathCmd.AddCommand(pathAddCmd)
}
