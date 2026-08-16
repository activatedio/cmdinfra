package cmd

import (
	"context"
	"fmt"
)

// Deps carries what generated commands need at runtime. The consumer's
// main constructs it once: lazy client factories (connections depend on
// global flags and credentials) and the named-context store.
type Deps struct {
	// Clients maps client keys (Resource.Client, default "default") to
	// lazy factories for the typed gRPC clients.
	Clients map[string]func(ctx context.Context) (any, error)
	// Contexts is the named-context store.
	Contexts ContextStore
}

// Client resolves a client by key.
func (d *Deps) Client(ctx context.Context, key string) (any, error) {

	factory, ok := d.Clients[key]
	if !ok {
		return nil, fmt.Errorf("no client %q is configured", key)
	}
	return factory(ctx)
}

// ContextValues loads the active named context's values; no file or no
// active context is an empty map.
func (d *Deps) ContextValues() (map[string]string, error) {

	f, err := d.Contexts.Load()
	if err != nil {
		return nil, err
	}
	return f.ActiveValues(), nil
}
