package cmd_test

import (
	"reflect"
	"testing"

	petstorev1 "github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1"
	gentf "github.com/activatedio/tfinfra/genlib/tf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/activatedio/cmdinfra/genlib/cmd"
)

func petEntry() gentf.Entry {
	return gentf.Entry{Type: reflect.TypeFor[petstorev1.Pet]()}
}

func TestNormalizeFlags(t *testing.T) {

	type s struct {
		arrange func() cmd.FieldFlags
		assert  func(t *testing.T, run func() []cmd.Flag)
	}

	flagByName := func(t *testing.T, flags []cmd.Flag, name string) cmd.Flag {
		t.Helper()
		for _, f := range flags {
			if f.Name == name {
				return f
			}
		}
		t.Fatalf("no flag named %q", name)
		return cmd.Flag{}
	}

	cases := map[string]s{
		"derives kebab-case names, skips the name field": {
			arrange: func() cmd.FieldFlags { return cmd.FieldFlags{} },
			assert: func(t *testing.T, run func() []cmd.Flag) {
				flags := run()
				names := make([]string, 0, len(flags))
				for _, f := range flags {
					names = append(names, f.Name)
				}
				assert.Equal(t, []string{
					"display-name", "type", "age", "vaccinated", "weight",
					"tags", "labels", "create-time", "config", "metadata",
				}, names)
			},
		},
		"enum flags carry completion values": {
			arrange: func() cmd.FieldFlags { return cmd.FieldFlags{} },
			assert: func(t *testing.T, run func() []cmd.Flag) {
				f := flagByName(t, run(), "type")
				assert.Equal(t, gentf.FieldEnum, f.Field.Kind)
				assert.Equal(t, []string{"PET_TYPE_UNSPECIFIED", "PET_TYPE_DOG", "PET_TYPE_CAT"}, f.Field.EnumValues)
			},
		},
		"message fields surface as JSON flags without an explicit list": {
			arrange: func() cmd.FieldFlags { return cmd.FieldFlags{} },
			assert: func(t *testing.T, run func() []cmd.Flag) {
				flags := run()
				assert.Equal(t, gentf.FieldAny, flagByName(t, flags, "config").Field.Kind)
				assert.Equal(t, gentf.FieldStruct, flagByName(t, flags, "metadata").Field.Kind)
				assert.Equal(t, gentf.FieldTimestamp, flagByName(t, flags, "create-time").Field.Kind)
			},
		},
		"rename and exclude apply": {
			arrange: func() cmd.FieldFlags {
				return cmd.FieldFlags{
					Exclude: []string{"metadata"},
					Rename:  map[string]string{"display_name": "title"},
				}
			},
			assert: func(t *testing.T, run func() []cmd.Flag) {
				flags := run()
				f := flagByName(t, flags, "title")
				assert.Equal(t, "display_name", f.Field.ProtoName)
				for _, f := range flags {
					require.NotEqual(t, "display-name", f.Name)
					require.NotEqual(t, "metadata", f.Field.ProtoName)
				}
			},
		},
		"sensitive fields are marked": {
			arrange: func() cmd.FieldFlags {
				return cmd.FieldFlags{Sensitive: []string{"display_name"}}
			},
			assert: func(t *testing.T, run func() []cmd.Flag) {
				assert.True(t, flagByName(t, run(), "display-name").Field.Sensitive)
			},
		},
		"unknown exclude panics": {
			arrange: func() cmd.FieldFlags {
				return cmd.FieldFlags{Exclude: []string{"nope"}}
			},
			assert: func(t *testing.T, run func() []cmd.Flag) {
				assert.PanicsWithValue(t, `Pet: FieldFlags.Exclude references unknown field "nope"`, func() { run() })
			},
		},
		"colliding rename panics": {
			arrange: func() cmd.FieldFlags {
				return cmd.FieldFlags{Rename: map[string]string{"age": "tags"}}
			},
			assert: func(t *testing.T, run func() []cmd.Flag) {
				assert.PanicsWithValue(t, `Pet: flag name "tags" derived for both "age" and "tags"`, func() { run() })
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			ff := tc.arrange()

			tc.assert(t, func() []cmd.Flag {
				return cmd.NormalizeFlags(petEntry(), ff)
			})
		})
	}
}
