package cmd

import (
	"fmt"
	"strings"

	"github.com/activatedio/tfinfra/pkg/aip"
)

// Resolver resolves a resource's scope identifiers and composes AIP parent
// and name strings — the CLI analog of the provider's context resolution.
// Precedence per identifier: explicit flag value > active named context
// value > actionable error.
type Resolver struct {
	// Scope is the resource's position in the AIP hierarchy (shared
	// semantics with the Terraform side via tfinfra pkg/aip).
	Scope aip.Scope
	// Explicit holds identifier values given as flags, keyed by identifier
	// attribute name ("tenant_id"); empty values are absent.
	Explicit map[string]string
	// Defaults holds the active named context's values.
	Defaults map[string]string
}

// Identifiers resolves every scope identifier, or returns an error naming
// each missing one and how to supply it.
func (r Resolver) Identifiers() (map[string]string, error) {

	ids := map[string]string{}
	var missing []string

	for _, attr := range r.Scope.IdentifierAttributes() {
		v := r.Explicit[attr]
		if v == "" {
			v = r.Defaults[attr]
		}
		if v == "" {
			missing = append(missing, attr)
		}
		ids[attr] = v
	}

	if len(missing) > 0 {
		flags := make([]string, len(missing))
		for i, m := range missing {
			flags[i] = "--" + strings.ReplaceAll(m, "_", "-")
		}
		return nil, fmt.Errorf("missing %s: pass %s or set %s in the active context",
			strings.Join(missing, ", "), strings.Join(flags, ", "), strings.Join(missing, ", "))
	}

	return ids, nil
}

// Parent composes the AIP parent string from the resolved identifiers.
func (r Resolver) Parent() (string, error) {

	ids, err := r.Identifiers()
	if err != nil {
		return "", err
	}
	return r.Scope.ComposeParent(ids)
}

// Name resolves a verb's positional argument to a full AIP name. A full
// name (contains "/") is validated against the scope's pattern — no
// context needed; a short ID composes with the resolved identifiers.
func (r Resolver) Name(collection, arg string) (string, error) {

	if strings.Contains(arg, "/") {
		if _, _, err := r.Scope.ParseName(collection, arg); err != nil {
			return "", err
		}
		return arg, nil
	}

	ids, err := r.Identifiers()
	if err != nil {
		return "", err
	}
	return r.Scope.ComposeName(collection, ids, arg)
}

// ParseBack splits a full AIP name into its scope identifier values and the
// short ID, filling the identifiers a later call can reuse.
func (r Resolver) ParseBack(collection, name string) (map[string]string, string, error) {
	return r.Scope.ParseName(collection, name)
}
