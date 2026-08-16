package cmd_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/activatedio/cmdinfra/genlib/cmd"
)

func TestColumnsFor(t *testing.T) {

	type s struct {
		arrange func() cmd.Columns
		assert  func(t *testing.T, run func() []string)
	}

	cases := map[string]s{
		"declared columns are validated and returned in order": {
			arrange: func() cmd.Columns {
				return cmd.Columns{Default: []string{"display_name", "type", "age"}}
			},
			assert: func(t *testing.T, run func() []string) {
				assert.Equal(t, []string{"display_name", "type", "age"}, run())
			},
		},
		"absent marker defaults to name and display_name": {
			arrange: func() cmd.Columns { return cmd.Columns{} },
			assert: func(t *testing.T, run func() []string) {
				assert.Equal(t, []string{"name", "display_name"}, run())
			},
		},
		"unknown column panics": {
			arrange: func() cmd.Columns {
				return cmd.Columns{Default: []string{"nope"}}
			},
			assert: func(t *testing.T, run func() []string) {
				assert.PanicsWithValue(t, `Pet: Columns.Default references unknown field "nope"`, func() { run() })
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			cols := tc.arrange()

			tc.assert(t, func() []string {
				return cmd.ColumnsFor(petEntry(), cols)
			})
		})
	}
}

func TestCommandPath(t *testing.T) {

	type s struct {
		arrange func() cmd.Resource
		assert  func(t *testing.T, run func() (string, string))
	}

	cases := map[string]s{
		"derives the kebab plural group": {
			arrange: func() cmd.Resource { return cmd.Resource{Group: "petstore"} },
			assert: func(t *testing.T, run func() (string, string)) {
				group, plural := run()
				assert.Equal(t, "petstore", group)
				assert.Equal(t, "pets", plural)
			},
		},
		"plural override wins": {
			arrange: func() cmd.Resource {
				return cmd.Resource{Group: "petstore", Plural: "zoo-animals"}
			},
			assert: func(t *testing.T, run func() (string, string)) {
				_, plural := run()
				assert.Equal(t, "zoo-animals", plural)
			},
		},
		"missing group panics": {
			arrange: func() cmd.Resource { return cmd.Resource{} },
			assert: func(t *testing.T, run func() (string, string)) {
				assert.Panics(t, func() { run() })
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			res := tc.arrange()

			tc.assert(t, func() (string, string) {
				return cmd.CommandPath(petEntry(), res)
			})
		})
	}
}
