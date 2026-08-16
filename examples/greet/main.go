// The greet example: the minimal end-to-end consumer of the bootstrap
// surface — a generated root command with a hand-written subcommand
// attached, run through the pkg/cmd runtime.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/activatedio/cmdinfra/examples/greet/generated"
	pkgcmd "github.com/activatedio/cmdinfra/pkg/cmd"
)

func main() {
	os.Exit(pkgcmd.Execute(context.Background(), newRoot()))
}

func newRoot() *cobra.Command {

	root := generated.NewRootCommand()
	root.AddCommand(&cobra.Command{
		Use:   "hello [name]",
		Short: "Print a greeting",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := "world"
			if len(args) > 0 {
				name = args[0]
			}
			_, err := fmt.Fprintf(c.OutOrStdout(), "hello %s\n", name)
			return err
		},
	})
	return root
}
