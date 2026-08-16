package cmd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	petstorev1 "github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/activatedio/cmdinfra/pkg/cmd"
)

// scriptedEditor writes a shell script that replaces the temp file's
// content, standing in for an interactive $EDITOR.
func scriptedEditor(t *testing.T, replacement string) []string {

	t.Helper()

	script := filepath.Join(t.TempDir(), "editor.sh")
	content := "#!/bin/sh\ncat > \"$1\" <<'DONE'\n" + replacement + "DONE\n"
	require.NoError(t, os.WriteFile(script, []byte(content), 0o700))

	return []string{"/bin/sh", script}
}

func TestEdit(t *testing.T) {

	type s struct {
		arrange func(t *testing.T) (cmd.EditParams, *petstorev1.Pet)
		assert  func(t *testing.T, edited *petstorev1.Pet, mask []string, err error)
	}

	current := func() *petstorev1.Pet {
		return &petstorev1.Pet{
			Name:        "stores/s-1/pets/p-1",
			DisplayName: "Rex",
			Tags:        []string{"loud"},
		}
	}

	cases := map[string]s{
		"edit patches exactly the touched fields": {
			arrange: func(t *testing.T) (cmd.EditParams, *petstorev1.Pet) {
				return cmd.EditParams{Editor: scriptedEditor(t,
					"name: stores/s-1/pets/p-1\ndisplay_name: Lord Rex\ntags:\n  - loud\nvaccinated: true\n",
				)}, current()
			},
			assert: func(t *testing.T, edited *petstorev1.Pet, mask []string, err error) {
				require.NoError(t, err)
				assert.Equal(t, []string{"display_name", "vaccinated"}, mask)
				assert.Equal(t, "Lord Rex", edited.GetDisplayName())
				assert.True(t, edited.GetVaccinated())
			},
		},
		"no changes means an empty mask": {
			arrange: func(_ *testing.T) (cmd.EditParams, *petstorev1.Pet) {
				return cmd.EditParams{Editor: []string{"true"}}, current()
			},
			assert: func(t *testing.T, edited *petstorev1.Pet, mask []string, err error) {
				require.NoError(t, err)
				assert.Empty(t, mask)
				assert.True(t, proto.Equal(current(), edited))
			},
		},
		"no editor configured errors": {
			arrange: func(_ *testing.T) (cmd.EditParams, *petstorev1.Pet) {
				return cmd.EditParams{}, current()
			},
			assert: func(t *testing.T, _ *petstorev1.Pet, _ []string, err error) {
				require.ErrorContains(t, err, "no editor configured")
			},
		},
		"invalid edited document errors": {
			arrange: func(t *testing.T) (cmd.EditParams, *petstorev1.Pet) {
				return cmd.EditParams{Editor: scriptedEditor(t, "nope: true\n")}, current()
			},
			assert: func(t *testing.T, _ *petstorev1.Pet, _ []string, err error) {
				require.Error(t, err)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			params, pet := tc.arrange(t)

			edited, mask, err := cmd.Edit(context.Background(), params, pet)

			tc.assert(t, edited, mask, err)
		})
	}
}

func TestFieldDiff_ExcludesName(t *testing.T) {

	before := &petstorev1.Pet{Name: "stores/s-1/pets/p-1"}
	after := &petstorev1.Pet{Name: "stores/s-1/pets/p-2", Age: 4}

	assert.Equal(t, []string{"age"}, cmd.FieldDiff(before, after))
}
