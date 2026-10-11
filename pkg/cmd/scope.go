package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/activatedio/tfinfra/pkg/aip"
)

// Resolver resolves a resource's scope identifiers and composes AIP parent
// and name strings — the CLI analog of the provider's context resolution.
// Precedence per identifier: explicit flag value > the selected profile's
// context value > actionable error.
type Resolver struct {
	// Scope is the resource's position in the AIP hierarchy (shared
	// semantics with the Terraform side via tfinfra pkg/aip).
	Scope aip.Scope
	// Explicit holds identifier values given as flags, keyed by identifier
	// attribute name ("tenant_id"); empty values are absent.
	Explicit map[string]string
	// Defaults holds the selected profile's context values.
	Defaults map[string]string
	// Profile names the profile Defaults came from, for EchoScope.
	Profile string
	// Echo is set when EchoScope should print: stderr is a terminal.
	Echo bool
	// SetCommand is the command that stores the context Defaults come from
	// ("awctl identity context set"); empty when the scope has none.
	SetCommand string
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
		msg := fmt.Sprintf("missing %s: pass %s", strings.Join(missing, ", "), strings.Join(flags, ", "))
		if r.SetCommand != "" {
			sets := make([]string, len(missing))
			for i, m := range missing {
				sets[i] = "--" + strings.ReplaceAll(Level(m), "_", "-") + " <id>"
			}
			msg += fmt.Sprintf(", or store it in the profile with `%s %s`", r.SetCommand, strings.Join(sets, " "))
		}
		return nil, errors.New(msg)
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

// Defaulted returns the scope identifiers, in scope order, whose value comes
// from the context rather than a flag.
func (r Resolver) Defaulted() []string {

	var attrs []string
	for _, attr := range r.Scope.IdentifierAttributes() {
		if r.Explicit[attr] == "" && r.Defaults[attr] != "" {
			attrs = append(attrs, attr)
		}
	}
	return attrs
}

// EchoScope is what a write verb calls before it writes: when stderr is a
// terminal and the scope took any value from the context, it prints one
// line saying which, so a stale stored scope announces itself on the line
// above the result —
//
//	scope from profile test01: tenant t-1, realm r-emp
//
// arg is the verb's positional resource argument, "" when it has none. A
// full resource name in it supplies its own scope, so nothing prints.
// Reads stay quiet, and so does everything off a terminal.
func (r Resolver) EchoScope(w io.Writer, arg string) {

	if !r.Echo || strings.Contains(arg, "/") {
		return
	}
	attrs := r.Defaulted()
	if len(attrs) == 0 {
		return
	}
	parts := make([]string, len(attrs))
	for i, attr := range attrs {
		parts[i] = Level(attr) + " " + r.Defaults[attr]
	}
	_, _ = fmt.Fprintf(w, "scope from profile %s: %s\n", r.Profile, strings.Join(parts, ", "))
}

// Level is an identifier attribute's scope level: "tenant" for "tenant_id".
func Level(attr string) string {
	return strings.TrimSuffix(attr, "_id")
}
