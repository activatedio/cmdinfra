package cmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/activatedio/cmdinfra/genlib/cmd"
)

func TestRegistry_Generate(t *testing.T) {

	type s struct {
		arrange func() *cmd.Spec
		assert  func(t *testing.T, dir string, run func())
	}

	greetSpec := func() *cmd.Spec {
		return &cmd.Spec{
			Package: "generated",
			Root: cmd.Root{
				Use:   "greet",
				Short: "A minimal cmdinfra-generated CLI",
			},
		}
	}

	cases := map[string]s{
		"greet spec regenerates the golden byte-identically": {
			arrange: greetSpec,
			assert: func(t *testing.T, dir string, run func()) {
				run()
				got, err := os.ReadFile(filepath.Join(dir, "root_gen.go"))
				require.NoError(t, err)
				want, err := os.ReadFile(filepath.Join("..", "..", "examples", "greet", "generated", "root_gen.go"))
				require.NoError(t, err)
				assert.Equal(t, string(want), string(got))
			},
		},
		"missing package panics": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Package = ""
				return spec
			},
			assert: func(t *testing.T, _ string, run func()) {
				assert.PanicsWithValue(t, "cmdinfra: Spec.Package must be set", run)
			},
		},
		"missing root use panics": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Root.Use = ""
				return spec
			},
			assert: func(t *testing.T, _ string, run func()) {
				assert.PanicsWithValue(t, "cmdinfra: Spec.Root.Use must be set", run)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			spec := tc.arrange()
			dir := t.TempDir()

			tc.assert(t, dir, func() {
				cmd.NewRegistry().RunDirectoryPathHandler(dir, spec)
			})
		})
	}
}
