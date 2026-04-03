package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/akoskm/hrs/internal/db"
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Manage clients",
}

var clientAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a client",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		client, err := store.CreateClient(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", client.Name)
		return err
	},
}

var clientListCmd = &cobra.Command{
	Use:   "list",
	Short: "List clients",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := db.Open(dbPath)
		if err != nil {
			return err
		}
		defer store.Close()

		clients, err := store.ListClients(cmd.Context())
		if err != nil {
			return err
		}
		for _, client := range clients {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", client.Name); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	clientCmd.AddCommand(clientAddCmd, clientListCmd)
}
