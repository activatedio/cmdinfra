package cmd

import (
	"fmt"
	"slices"

	gentf "github.com/activatedio/tfinfra/genlib/tf"
)

// groupContext is one service group's resolved context declaration.
type groupContext struct {
	// Namespace is where the group's context is stored: its own name, the
	// shared group's, or "" for a group that reads none.
	Namespace string
	// Levels are the namespace's storable levels.
	Levels []string
	// SharedWith is the owning group when the context is shared.
	SharedWith string
}

// validateGroups refuses a Group declaration that would generate a context
// verb nobody can use: an unknown group, a level no entry of the group has,
// or a share of a group with no context of its own.
func validateGroups(spec *Spec) {

	levels := map[string][]string{}
	for _, e := range spec.Entries {
		res, _ := gentf.GetImplementation[Resource](e)
		group, _ := CommandPath(e, res)
		levels[group] = append(levels[group], res.Scope.IdentifierAttributes()...)
	}

	declared := map[string]Group{}
	for _, g := range spec.Groups {
		if _, ok := declared[g.Name]; ok {
			panic(fmt.Sprintf("cmdinfra: Group %q is declared twice", g.Name))
		}
		validateGroup(g, levels)
		declared[g.Name] = g
	}

	for _, g := range spec.Groups {
		if g.SharesContext == "" {
			continue
		}
		if owner := declared[g.SharesContext]; len(owner.Context) == 0 {
			panic(fmt.Sprintf("cmdinfra: Group %q: SharesContext names %q, which declares no Context", g.Name, g.SharesContext))
		}
	}
}

// validateGroup checks one declaration against the scope levels of each
// group's entries.
func validateGroup(g Group, levels map[string][]string) {

	if _, ok := levels[g.Name]; !ok {
		panic(fmt.Sprintf("cmdinfra: Group %q: no entry declares Resource.Group %q", g.Name, g.Name))
	}
	if len(g.Context) > 0 && g.SharesContext != "" {
		panic(fmt.Sprintf("cmdinfra: Group %q: Context and SharesContext are exclusive", g.Name))
	}
	for _, level := range g.Context {
		if !slices.Contains(levels[g.Name], level+"_id") {
			panic(fmt.Sprintf("cmdinfra: Group %q: Context level %q is no scope level of a %s entry", g.Name, level, g.Name))
		}
	}
}

// contextFor resolves a service group's context.
func contextFor(spec *Spec, group string) groupContext {

	byName := map[string]Group{}
	for _, g := range spec.Groups {
		byName[g.Name] = g
	}

	g := byName[group]
	switch {
	case len(g.Context) > 0:
		return groupContext{Namespace: g.Name, Levels: g.Context}
	case g.SharesContext != "":
		return groupContext{Namespace: g.SharesContext, Levels: byName[g.SharesContext].Context, SharedWith: g.SharesContext}
	default:
		return groupContext{}
	}
}

// setCommand is the command the missing-scope error points at.
func (gc groupContext) setCommand(spec *Spec, group string) string {
	if gc.Namespace == "" {
		return ""
	}
	return spec.Root.Use + " " + group + " context set"
}
