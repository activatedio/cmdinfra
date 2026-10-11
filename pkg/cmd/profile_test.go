package cmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/activatedio/tfinfra/pkg/aip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/activatedio/cmdinfra/pkg/cmd"
)

func TestProfileStore_RoundTrip(t *testing.T) {

	store := cmd.ProfileStore{Path: filepath.Join(t.TempDir(), "awctl.d", "config.yaml")}

	// A missing file loads as an empty document, and selects nothing.
	f, err := store.Load()
	require.NoError(t, err)
	assert.Empty(t, f.Names())
	name, p, err := f.Select("")
	require.NoError(t, err)
	assert.Empty(t, name)
	assert.Nil(t, p)
	assert.Empty(t, p.Context("identity"))

	test01 := &cmd.Profile{Settings: map[string]string{"endpoint": "admin.test01:443", "insecure": "true"}}
	test01.SetContext("identity", "tenant_id", "t-1")
	test01.SetContext("identity", "realm_id", "r-emp")
	test01.SetContext("guard", "tenant_id", "gt-9")
	require.NoError(t, f.Add("test01", test01))
	require.NoError(t, f.Add("preview", &cmd.Profile{}))
	require.ErrorContains(t, f.Add("preview", &cmd.Profile{}), `profile "preview" already exists`)
	require.NoError(t, f.Activate("test01"))
	require.NoError(t, store.Save(f))

	// The directory is created, and the file is the owner's alone.
	info, err := os.Stat(store.Path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// Settings sit at the profile's top level beside its contexts.
	data, err := os.ReadFile(store.Path)
	require.NoError(t, err)
	assert.Equal(t, `active: test01
profiles:
  preview: {}
  test01:
    contexts:
      guard:
        tenant_id: gt-9
      identity:
        realm_id: r-emp
        tenant_id: t-1
    endpoint: admin.test01:443
    insecure: "true"
`, string(data))

	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"preview", "test01"}, loaded.Names())
	name, p, err = loaded.Select("")
	require.NoError(t, err)
	assert.Equal(t, "test01", name)
	assert.Equal(t, "admin.test01:443", p.Setting("endpoint"))
	// One profile, two namespaces: guard's tenant is not identity's.
	assert.Equal(t, map[string]string{"tenant_id": "t-1", "realm_id": "r-emp"}, p.Context("identity"))
	assert.Equal(t, map[string]string{"tenant_id": "gt-9"}, p.Context("guard"))

	// An explicit selection wins over the active one.
	name, p, err = loaded.Select("preview")
	require.NoError(t, err)
	assert.Equal(t, "preview", name)
	assert.Empty(t, p.Context("identity"))

	_, _, err = loaded.Select("staging")
	require.ErrorContains(t, err, `unknown profile "staging": known profiles are preview, test01`)

	require.ErrorContains(t, loaded.Delete("test01"), "is active")
	require.NoError(t, loaded.Delete("preview"))
	assert.Equal(t, []string{"test01"}, loaded.Names())
}

func TestProfile_Contexts(t *testing.T) {

	p := &cmd.Profile{}
	p.SetContext("identity", "tenant_id", "t-1")
	p.SetContext("identity", "realm_id", "r-1")

	p.ClearContext("identity", "realm_id")
	assert.Equal(t, map[string]string{"tenant_id": "t-1"}, p.Context("identity"))

	// Clearing the last value drops the namespace from the document.
	p.ClearContext("identity", "tenant_id")
	assert.Empty(t, p.Contexts)

	p.SetContext("guard", "tenant_id", "gt-1")
	p.ClearContext("guard")
	assert.Empty(t, p.Contexts)
}

func TestProfile_HandEditedScalars(t *testing.T) {

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("active: dev\nprofiles:\n  dev:\n    insecure: true\n    port: 443\n"), 0o600))

	f, err := cmd.ProfileStore{Path: path}.Load()
	require.NoError(t, err)
	_, p, err := f.Select("")
	require.NoError(t, err)
	assert.Equal(t, "true", p.Setting("insecure"))
	assert.Equal(t, "443", p.Setting("port"))

	require.NoError(t, os.WriteFile(path, []byte("profiles:\n  dev:\n    endpoint: [a, b]\n"), 0o600))
	_, err = cmd.ProfileStore{Path: path}.Load()
	require.ErrorContains(t, err, "endpoint: a profile setting is a single value")
}

func TestDeps_Resolver(t *testing.T) {

	store := cmd.ProfileStore{Path: filepath.Join(t.TempDir(), "config.yaml")}
	f := &cmd.ProfileFile{}
	dev := &cmd.Profile{Settings: map[string]string{"insecure": "true"}}
	dev.SetContext("identity", "tenant_id", "t-1")
	dev.SetContext("guard", "tenant_id", "gt-1")
	require.NoError(t, f.Add("dev", dev))
	require.NoError(t, f.Add("prod", &cmd.Profile{}))
	require.NoError(t, f.Activate("dev"))
	require.NoError(t, store.Save(f))

	deps := &cmd.Deps{Profiles: store, Terminal: func() bool { return true }}
	scope := aip.NewScope("tenants")

	r, err := deps.Resolver(cmd.ResolverParams{Namespace: "guard", Scope: scope})
	require.NoError(t, err)
	parent, err := r.Parent()
	require.NoError(t, err)
	assert.Equal(t, "tenants/gt-1", parent)
	assert.Equal(t, "dev", r.Profile)
	assert.True(t, r.Echo)

	// A group without a context reads none, whatever the profile holds.
	r, err = deps.Resolver(cmd.ResolverParams{Scope: scope})
	require.NoError(t, err)
	_, err = r.Parent()
	require.ErrorContains(t, err, "missing tenant_id")

	insecure, err := deps.BoolSetting("insecure")
	require.NoError(t, err)
	assert.True(t, insecure)

	// --profile reads another profile's settings and context.
	deps.Profile = "prod"
	r, err = deps.Resolver(cmd.ResolverParams{Namespace: "identity", Scope: scope})
	require.NoError(t, err)
	_, err = r.Parent()
	require.ErrorContains(t, err, "missing tenant_id")
	insecure, err = deps.BoolSetting("insecure")
	require.NoError(t, err)
	assert.False(t, insecure)
}
