package cmd_test

import (
	"strings"
	"testing"

	"github.com/activatedio/tfinfra/pkg/aip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/activatedio/cmdinfra/pkg/cmd"
)

func TestResolver(t *testing.T) {

	type s struct {
		arrange func() cmd.Resolver
		assert  func(t *testing.T, r cmd.Resolver)
	}

	threeLevel := aip.NewScope("tenants", "issuers", "audiences")

	cases := map[string]s{
		"explicit flags win over context defaults": {
			arrange: func() cmd.Resolver {
				return cmd.Resolver{
					Scope:    threeLevel,
					Explicit: map[string]string{"audience_id": "a-override"},
					Defaults: map[string]string{"tenant_id": "t-1", "issuer_id": "i-1", "audience_id": "a-1"},
				}
			},
			assert: func(t *testing.T, r cmd.Resolver) {
				parent, err := r.Parent()
				require.NoError(t, err)
				assert.Equal(t, "tenants/t-1/issuers/i-1/audiences/a-override", parent)
			},
		},
		"short id composes with resolved identifiers": {
			arrange: func() cmd.Resolver {
				return cmd.Resolver{
					Scope:    aip.NewScope("tenants"),
					Defaults: map[string]string{"tenant_id": "t-1"},
				}
			},
			assert: func(t *testing.T, r cmd.Resolver) {
				name, err := r.Name("realms", "r-9")
				require.NoError(t, err)
				assert.Equal(t, "tenants/t-1/realms/r-9", name)
			},
		},
		"full name validates and needs no context": {
			arrange: func() cmd.Resolver {
				return cmd.Resolver{Scope: aip.NewScope("tenants")}
			},
			assert: func(t *testing.T, r cmd.Resolver) {
				name, err := r.Name("realms", "tenants/t-1/realms/r-9")
				require.NoError(t, err)
				assert.Equal(t, "tenants/t-1/realms/r-9", name)
			},
		},
		"bad full name reports the pattern": {
			arrange: func() cmd.Resolver {
				return cmd.Resolver{Scope: aip.NewScope("tenants")}
			},
			assert: func(t *testing.T, r cmd.Resolver) {
				_, err := r.Name("realms", "issuers/i-1/realms/r-9")
				require.ErrorContains(t, err, "tenants/{tenant}/realms/{id}")
			},
		},
		"parse-back fills the identifiers": {
			arrange: func() cmd.Resolver {
				return cmd.Resolver{Scope: threeLevel}
			},
			assert: func(t *testing.T, r cmd.Resolver) {
				ids, id, err := r.ParseBack("roles", "tenants/t-1/issuers/i-1/audiences/a-1/roles/ro-1")
				require.NoError(t, err)
				assert.Equal(t, "ro-1", id)
				assert.Equal(t, map[string]string{"tenant_id": "t-1", "issuer_id": "i-1", "audience_id": "a-1"}, ids)
			},
		},
		"missing identifiers name every gap and both fixes": {
			arrange: func() cmd.Resolver {
				return cmd.Resolver{
					Scope:    threeLevel,
					Defaults: map[string]string{"tenant_id": "t-1"},
				}
			},
			assert: func(t *testing.T, r cmd.Resolver) {
				_, err := r.Parent()
				require.ErrorContains(t, err, "missing issuer_id, audience_id")
				require.ErrorContains(t, err, "--issuer-id, --audience-id")
				assert.NotContains(t, err.Error(), "context set")
			},
		},
		"missing identifiers name the context set when there is one": {
			arrange: func() cmd.Resolver {
				return cmd.Resolver{
					Scope:      threeLevel,
					Defaults:   map[string]string{"tenant_id": "t-1"},
					SetCommand: "awctl access context set",
				}
			},
			assert: func(t *testing.T, r cmd.Resolver) {
				_, err := r.Parent()
				require.EqualError(t, err, "missing issuer_id, audience_id: pass --issuer-id, --audience-id, "+
					"or store it in the profile with `awctl access context set --issuer <id> --audience <id>`")
			},
		},
		"defaulted lists the context-supplied identifiers in scope order": {
			arrange: func() cmd.Resolver {
				return cmd.Resolver{
					Scope:    threeLevel,
					Explicit: map[string]string{"issuer_id": "i-2"},
					Defaults: map[string]string{"tenant_id": "t-1", "issuer_id": "i-1", "audience_id": "a-1"},
				}
			},
			assert: func(t *testing.T, r cmd.Resolver) {
				assert.Equal(t, []string{"tenant_id", "audience_id"}, r.Defaulted())
			},
		},
		"top-level scope resolves an empty parent": {
			arrange: func() cmd.Resolver {
				return cmd.Resolver{Scope: aip.ScopeNone}
			},
			assert: func(t *testing.T, r cmd.Resolver) {
				parent, err := r.Parent()
				require.NoError(t, err)
				assert.Empty(t, parent)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			r := tc.arrange()

			tc.assert(t, r)
		})
	}
}

func TestResolver_EchoScope(t *testing.T) {

	type s struct {
		arrange func(r *cmd.Resolver) string
		want    string
	}

	cases := map[string]s{
		"a context-supplied scope is named, levels without _id": {
			arrange: func(*cmd.Resolver) string { return "u-1" },
			want:    "scope from profile test01: tenant t-1, realm r-emp\n",
		},
		"off a terminal nothing prints": {
			arrange: func(r *cmd.Resolver) string { r.Echo = false; return "u-1" },
		},
		"a full name brings its own scope": {
			arrange: func(*cmd.Resolver) string { return "tenants/t-1/realms/r-emp/users/u-1" },
		},
		"flags for every level leave nothing to say": {
			arrange: func(r *cmd.Resolver) string {
				r.Explicit = map[string]string{"tenant_id": "t-1", "realm_id": "r-1"}
				return ""
			},
		},
		"only the defaulted levels are named": {
			arrange: func(r *cmd.Resolver) string {
				r.Explicit = map[string]string{"tenant_id": "t-2"}
				return ""
			},
			want: "scope from profile test01: realm r-emp\n",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			r := cmd.Resolver{
				Scope:    aip.NewScope("tenants", "realms"),
				Defaults: map[string]string{"tenant_id": "t-1", "realm_id": "r-emp"},
				Profile:  "test01",
				Echo:     true,
			}
			arg := tc.arrange(&r)

			var b strings.Builder
			r.EchoScope(&b, arg)

			assert.Equal(t, tc.want, b.String())
		})
	}
}
