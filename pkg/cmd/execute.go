package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// Execute runs root and reports its outcome as a process exit code,
// printing any error to the command's error stream. Generated roots
// silence cobra's own error and usage printing, so this is the single
// place errors surface; os.Exit stays in the consumer's main.
func Execute(ctx context.Context, root *cobra.Command) int {
	if err := root.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(root.ErrOrStderr(), "error:", err)
		return 1
	}
	return 0
}
