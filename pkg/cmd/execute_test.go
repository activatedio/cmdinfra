package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/activatedio/cmdinfra/pkg/cmd"
)

func TestExecute(t *testing.T) {

	type s struct {
		arrange func() *cobra.Command
		assert  func(t *testing.T, code int, stderr string)
	}

	cases := map[string]s{
		"success returns zero and prints nothing": {
			arrange: func() *cobra.Command {
				return &cobra.Command{
					Use:           "ok",
					SilenceUsage:  true,
					SilenceErrors: true,
					RunE:          func(*cobra.Command, []string) error { return nil },
				}
			},
			assert: func(t *testing.T, code int, stderr string) {
				assert.Equal(t, 0, code)
				assert.Empty(t, stderr)
			},
		},
		"error returns one and prints the error": {
			arrange: func() *cobra.Command {
				return &cobra.Command{
					Use:           "boom",
					SilenceUsage:  true,
					SilenceErrors: true,
					RunE:          func(*cobra.Command, []string) error { return errors.New("it broke") },
				}
			},
			assert: func(t *testing.T, code int, stderr string) {
				assert.Equal(t, 1, code)
				assert.Equal(t, "error: it broke\n", stderr)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			root := tc.arrange()
			var stderr bytes.Buffer
			root.SetErr(&stderr)

			code := cmd.Execute(context.Background(), root)

			tc.assert(t, code, stderr.String())
		})
	}
}
