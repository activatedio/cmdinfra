package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewConfigCommand returns the `config contexts` command group over the
// named-context store: list, activate, and set.
func NewConfigCommand(store ContextStore) *cobra.Command {

	config := &cobra.Command{Use: "config", Short: "Manage CLI configuration"}
	contexts := &cobra.Command{Use: "contexts", Short: "Manage named contexts"}
	config.AddCommand(contexts)

	contexts.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the named contexts",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			f, err := store.Load()
			if err != nil {
				return err
			}
			for _, name := range f.Names() {
				marker := " "
				if name == f.Active {
					marker = "*"
				}
				if _, err := fmt.Fprintf(c.OutOrStdout(), "%s %s\n", marker, name); err != nil {
					return err
				}
			}
			return nil
		},
	})

	contexts.AddCommand(&cobra.Command{
		Use:   "activate <name>",
		Short: "Activate a named context",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			f, err := store.Load()
			if err != nil {
				return err
			}
			if err := f.Activate(args[0]); err != nil {
				return err
			}
			if err := store.Save(f); err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Activated %s\n", args[0])
			return err
		},
	})

	contexts.AddCommand(&cobra.Command{
		Use:   "set <context> <key> <value>",
		Short: "Set a value in a named context (e.g. set dev tenant_id t-1)",
		Args:  cobra.ExactArgs(3),
		RunE: func(c *cobra.Command, args []string) error {
			f, err := store.Load()
			if err != nil {
				return err
			}
			f.Set(args[0], args[1], args[2])
			if err := store.Save(f); err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Set %s.%s\n", args[0], args[1])
			return err
		},
	})

	return config
}
