package cmd_test

import (
	"path/filepath"
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
				require.ErrorContains(t, err, "active context")
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

func TestContextStore_RoundTrip(t *testing.T) {

	store := cmd.ContextStore{Path: filepath.Join(t.TempDir(), "contexts.yaml")}

	// A missing file loads as an empty document.
	f, err := store.Load()
	require.NoError(t, err)
	assert.Empty(t, f.Names())
	assert.Empty(t, f.ActiveValues())

	f.Set("dev", "tenant_id", "t-dev")
	f.Set("dev", "issuer_id", "i-dev")
	f.Set("prod", "tenant_id", "t-prod")
	require.NoError(t, f.Activate("dev"))
	require.NoError(t, store.Save(f))

	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"dev", "prod"}, loaded.Names())
	assert.Equal(t, "dev", loaded.Active)
	assert.Equal(t, map[string]string{"tenant_id": "t-dev", "issuer_id": "i-dev"}, loaded.ActiveValues())

	err = loaded.Activate("staging")
	require.ErrorContains(t, err, `unknown context "staging"`)
	require.ErrorContains(t, err, "dev, prod")
}
