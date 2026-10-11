package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/activatedio/tfinfra/pkg/aip"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// Deps carries what generated commands need at runtime. The consumer's
// main constructs it once: lazy client factories (connections depend on
// global flags and credentials) and the profile store.
type Deps struct {
	// Clients maps client keys (Resource.Client, default "default") to
	// lazy factories for the typed gRPC clients.
	Clients map[string]func(ctx context.Context) (any, error)
	// Profiles is the profile store.
	Profiles ProfileStore
	// Profile names the profile this invocation uses; empty means the
	// active one. BindProfileFlag fills it from --profile or its env
	// variable.
	Profile string
	// Terminal reports whether stderr is a terminal, which is when a write
	// says which scope it took from the context. Nil checks os.Stderr.
	Terminal func() bool
}

// Client resolves a client by key.
func (d *Deps) Client(ctx context.Context, key string) (any, error) {

	factory, ok := d.Clients[key]
	if !ok {
		return nil, fmt.Errorf("no client %q is configured", key)
	}
	return factory(ctx)
}

// BindProfileFlag registers the persistent --profile flag on root, defaulting
// to the env variable, so every command can run against one profile without
// activating it: two terminals do not fight over the active profile.
func (d *Deps) BindProfileFlag(root *cobra.Command, env string) {
	root.PersistentFlags().StringVar(&d.Profile, "profile", os.Getenv(env),
		"the profile this command uses, instead of the active one (or "+env+")")
}

// SelectedProfile loads the profile this invocation uses: --profile when
// given, the active one otherwise. With neither it returns "" and a nil
// profile, which has no settings and no context.
func (d *Deps) SelectedProfile() (string, *Profile, error) {

	f, err := d.Profiles.Load()
	if err != nil {
		return "", nil, err
	}
	return f.Select(d.Profile)
}

// Setting returns one setting of the selected profile, "" when unset or
// when there is no profile. Consumers layer their flags and env variables
// above it.
func (d *Deps) Setting(key string) (string, error) {

	_, p, err := d.SelectedProfile()
	if err != nil {
		return "", err
	}
	return p.Setting(key), nil
}

// BoolSetting is Setting for a ProfileField declared Bool.
func (d *Deps) BoolSetting(key string) (bool, error) {
	s, err := d.Setting(key)
	return parseBool(s), err
}

// ContextValues returns the selected profile's scope defaults in one
// namespace; no profile, or nothing stored, is an empty map. An empty
// namespace — a group that declares no context — is always empty.
func (d *Deps) ContextValues(namespace string) (map[string]string, error) {

	if namespace == "" {
		return map[string]string{}, nil
	}
	_, p, err := d.SelectedProfile()
	if err != nil {
		return nil, err
	}
	return p.Context(namespace), nil
}

// ResolverParams is the input to Deps.Resolver.
type ResolverParams struct {
	// Namespace is the context namespace the scope defaults come from; ""
	// reads none.
	Namespace string
	// Scope is the resource's position in the AIP hierarchy.
	Scope aip.Scope
	// Explicit holds the scope flag values, keyed by identifier attribute.
	Explicit map[string]string
	// SetCommand is the command that stores this namespace's context
	// ("awctl identity context set"), named by the missing-scope error.
	SetCommand string
}

// Resolver builds a command's scope resolver over the selected profile's
// context in the namespace.
func (d *Deps) Resolver(p ResolverParams) (Resolver, error) {

	r := Resolver{Scope: p.Scope, Explicit: p.Explicit, Defaults: map[string]string{}, SetCommand: p.SetCommand}
	if p.Namespace == "" {
		return r, nil
	}

	name, profile, err := d.SelectedProfile()
	if err != nil {
		return Resolver{}, err
	}
	r.Defaults = profile.Context(p.Namespace)
	r.Profile = name
	r.Echo = d.terminal()
	return r, nil
}

func (d *Deps) terminal() bool {
	if d.Terminal != nil {
		return d.Terminal()
	}
	return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
}
