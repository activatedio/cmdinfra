package cmd_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/activatedio/cmdinfra/pkg/cmd"
)

func TestRenderers(t *testing.T) {

	type s struct {
		arrange func() (cmd.RendererParams, []cmd.Record, cmd.FieldList)
		assert  func(t *testing.T, out string, err error)
	}

	cases := map[string]s{
		"table aligns columns and uppercases headers": {
			arrange: func() (cmd.RendererParams, []cmd.Record, cmd.FieldList) {
				return cmd.RendererParams{Format: "table"},
					[]cmd.Record{
						{"name": "r-1", "display_name": "Employees"},
						{"name": "r-100", "display_name": "Ops"},
					},
					cmd.FieldList{"name", "display_name"}
			},
			assert: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.Equal(t, "NAME   DISPLAY_NAME\nr-1    Employees\nr-100  Ops\n", out)
			},
		},
		"table walks dotted paths and blanks missing values": {
			arrange: func() (cmd.RendererParams, []cmd.Record, cmd.FieldList) {
				return cmd.RendererParams{},
					[]cmd.Record{
						{"name": "p-1", "buckle": map[string]any{"material": "brass"}},
						{"name": "p-2"},
					},
					cmd.FieldList{"name", "buckle.material"}
			},
			assert: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.Equal(t, "NAME  BUCKLE.MATERIAL\np-1   brass\np-2   \n", out)
			},
		},
		"table derives the id column from the name": {
			arrange: func() (cmd.RendererParams, []cmd.Record, cmd.FieldList) {
				return cmd.RendererParams{},
					[]cmd.Record{
						{"name": "tenants/t-01/issuers/i-01/audiences/a-01/clients/c-1i4r4a0k6qm82", "display_name": "Console"},
						{"name": "c-2"},
						{"display_name": "nameless"},
					},
					cmd.FieldList{"id", "display_name"}
			},
			assert: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.Equal(t, "ID               DISPLAY_NAME\nc-1i4r4a0k6qm82  Console\nc-2              \n                 nameless\n", out)
			},
		},
		"table prefers a real id field": {
			arrange: func() (cmd.RendererParams, []cmd.Record, cmd.FieldList) {
				return cmd.RendererParams{},
					[]cmd.Record{{"name": "users/u-1/devices/d-1", "id": "dev-x"}},
					cmd.FieldList{"id"}
			},
			assert: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.Equal(t, "ID\ndev-x\n", out)
			},
		},
		"table masks sensitive fields": {
			arrange: func() (cmd.RendererParams, []cmd.Record, cmd.FieldList) {
				return cmd.RendererParams{Masked: []string{"value"}},
					[]cmd.Record{{"name": "s-1", "value": "hunter2"}},
					cmd.FieldList{"name", "value"}
			},
			assert: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.Contains(t, out, "********")
				assert.NotContains(t, out, "hunter2")
			},
		},
		"table trims long values with an ellipsis": {
			arrange: func() (cmd.RendererParams, []cmd.Record, cmd.FieldList) {
				return cmd.RendererParams{},
					[]cmd.Record{{"description": strings.Repeat("x", 45)}},
					cmd.FieldList{"description"}
			},
			assert: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.Contains(t, out, strings.Repeat("x", 37)+"...")
				assert.NotContains(t, out, strings.Repeat("x", 38))
			},
		},
		"table never trims the name": {
			arrange: func() (cmd.RendererParams, []cmd.Record, cmd.FieldList) {
				return cmd.RendererParams{},
					[]cmd.Record{{"name": "tenants/t-01/issuers/i-01/audiences/a-01/clients/c-1i4r4a0k6qm82"}},
					cmd.FieldList{"name"}
			},
			assert: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.Equal(t, "NAME\ntenants/t-01/issuers/i-01/audiences/a-01/clients/c-1i4r4a0k6qm82\n", out)
			},
		},
		"yaml renders the full records": {
			arrange: func() (cmd.RendererParams, []cmd.Record, cmd.FieldList) {
				return cmd.RendererParams{Format: "yaml"},
					[]cmd.Record{{"name": "r-1", "display_name": "Employees"}},
					cmd.FieldList{"name"}
			},
			assert: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.Equal(t, "- display_name: Employees\n  name: r-1\n", out)
			},
		},
		"json renders the full records": {
			arrange: func() (cmd.RendererParams, []cmd.Record, cmd.FieldList) {
				return cmd.RendererParams{Format: "json"},
					[]cmd.Record{{"name": "r-1"}},
					cmd.FieldList{"name"}
			},
			assert: func(t *testing.T, out string, err error) {
				require.NoError(t, err)
				assert.JSONEq(t, `[{"name": "r-1"}]`, out)
			},
		},
		"unknown format errors": {
			arrange: func() (cmd.RendererParams, []cmd.Record, cmd.FieldList) {
				return cmd.RendererParams{Format: "csv"}, nil, nil
			},
			assert: func(t *testing.T, _ string, err error) {
				require.ErrorContains(t, err, `unknown format "csv"`)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			params, records, fields := tc.arrange()

			r, err := cmd.NewRenderer(params)
			var out strings.Builder
			if err == nil {
				err = r.RenderList(records, fields, &out)
			}

			tc.assert(t, out.String(), err)
		})
	}
}

func TestRenderSingle(t *testing.T) {

	r, err := cmd.NewRenderer(cmd.RendererParams{Format: "yaml"})
	require.NoError(t, err)

	var out strings.Builder
	require.NoError(t, r.RenderSingle(cmd.Record{"name": "r-1"}, nil, &out))

	assert.Equal(t, "name: r-1\n", out.String())
}
