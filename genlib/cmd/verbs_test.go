package cmd_test

import (
	"testing"

	gentf "github.com/activatedio/tfinfra/genlib/tf"
	"github.com/stretchr/testify/assert"

	"github.com/activatedio/cmdinfra/genlib/cmd"
)

func TestVerbsFor(t *testing.T) {

	type s struct {
		arrange func() gentf.Ops
		assert  func(t *testing.T, verbs []cmd.Verb)
	}

	cases := map[string]s{
		"zero ops means all verbs, patch-first": {
			arrange: func() gentf.Ops { return gentf.OpAll },
			assert: func(t *testing.T, verbs []cmd.Verb) {
				assert.Equal(t, []cmd.Verb{
					{Name: "create", Op: gentf.OpCreate},
					{Name: "delete", Op: gentf.OpDelete},
					{Name: "describe", Op: gentf.OpGet},
					{Name: "edit", Op: gentf.OpPatch},
					{Name: "list", Op: gentf.OpList},
					{Name: "update", Op: gentf.OpPatch},
				}, verbs)
			},
		},
		"read-only surface gets describe and list only": {
			arrange: func() gentf.Ops { return gentf.OpGet | gentf.OpList },
			assert: func(t *testing.T, verbs []cmd.Verb) {
				assert.Equal(t, []cmd.Verb{
					{Name: "describe", Op: gentf.OpGet},
					{Name: "list", Op: gentf.OpList},
				}, verbs)
			},
		},
		"full-replace update drives update and edit when patch is absent": {
			arrange: func() gentf.Ops { return gentf.OpGet | gentf.OpUpdate },
			assert: func(t *testing.T, verbs []cmd.Verb) {
				assert.Equal(t, []cmd.Verb{
					{Name: "describe", Op: gentf.OpGet},
					{Name: "edit", Op: gentf.OpUpdate},
					{Name: "update", Op: gentf.OpUpdate},
				}, verbs)
			},
		},
		"patch wins over update for the mutate verbs": {
			arrange: func() gentf.Ops { return gentf.OpGet | gentf.OpPatch | gentf.OpUpdate },
			assert: func(t *testing.T, verbs []cmd.Verb) {
				assert.Equal(t, []cmd.Verb{
					{Name: "describe", Op: gentf.OpGet},
					{Name: "edit", Op: gentf.OpPatch},
					{Name: "update", Op: gentf.OpPatch},
				}, verbs)
			},
		},
		"edit requires get": {
			arrange: func() gentf.Ops { return gentf.OpPatch },
			assert: func(t *testing.T, verbs []cmd.Verb) {
				assert.Equal(t, []cmd.Verb{
					{Name: "update", Op: gentf.OpPatch},
				}, verbs)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			ops := tc.arrange()

			tc.assert(t, cmd.VerbsFor(ops))
		})
	}
}
