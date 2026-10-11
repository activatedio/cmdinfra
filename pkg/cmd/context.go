package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// ContextParams describes one command group's `context` verb, emitted by the
// generator from the spec's Group declarations.
type ContextParams struct {
	// Namespace is where the values are stored in the profile: the group's
	// own name, or the group it shares a scope tree with.
	Namespace string
	// Levels are the storable scope levels, in scope order ("tenant",
	// "issuer"). Each is a flag on set and clear, and is stored under its
	// identifier attribute ("tenant_id").
	Levels []string
	// SharedWith names the group whose context this one is, when the group
	// shares it; the help says so.
	SharedWith string
}

// NewContextCommand returns a group's `context` verb: set, show and clear
// over the namespace in the selected profile.
func NewContextCommand(deps *Deps, p ContextParams) *cobra.Command {

	short := "Manage the " + p.Namespace + " context: scope defaults stored in the profile"
	long := "The context holds the scope this group's commands fall back to when a --<level>-id\n" +
		"flag is absent. It lives in the profile, so switching profile switches it too;\n" +
		"--profile reads and writes another profile's."
	if p.SharedWith != "" {
		short = "Alias of `" + p.SharedWith + " context`: the two groups share one scope tree"
		long = short + ". Setting it here sets it there.\n\n" + long
	}

	c := &cobra.Command{Use: "context", Short: short, Long: long}
	c.AddCommand(
		newContextSetCommand(deps, p),
		newContextShowCommand(deps, p),
		newContextClearCommand(deps, p),
	)
	return c
}

func levelFlag(level string) string {
	return strings.ReplaceAll(level, "_", "-")
}

func newContextSetCommand(deps *Deps, p ContextParams) *cobra.Command {

	c := &cobra.Command{
		Use:   "set",
		Short: "Store scope defaults (one flag per level)",
		Long: "Store scope defaults in the profile. Levels not named are left as they are:\n" +
			"setting --" + levelFlag(p.Levels[0]) + " does not clear the levels below it, so check show after\n" +
			"switching.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return updateContext(c, deps, p, func(prof *Profile) error {
				set := false
				for _, level := range p.Levels {
					if !c.Flags().Changed(levelFlag(level)) {
						continue
					}
					v, _ := c.Flags().GetString(levelFlag(level))
					if v == "" || strings.Contains(v, "/") {
						return fmt.Errorf("--%s takes a bare id, not %q", levelFlag(level), v)
					}
					prof.SetContext(p.Namespace, level+"_id", v)
					set = true
				}
				if !set {
					return errors.New("nothing to set: pass at least one of " + flagList(p.Levels))
				}
				return nil
			})
		},
	}
	for _, level := range p.Levels {
		c.Flags().String(levelFlag(level), "", level+" id to store")
	}
	return c
}

func newContextShowCommand(deps *Deps, p ContextParams) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the stored scope defaults",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {

			name, prof, err := deps.SelectedProfile()
			if err != nil {
				return err
			}
			if prof == nil {
				return errNoProfile(c)
			}

			values := prof.Context(p.Namespace)
			out := c.OutOrStdout()
			if len(values) == 0 {
				_, err := fmt.Fprintf(out, "No %s context in profile %s.\n", p.Namespace, name)
				return err
			}

			width := 0
			for _, level := range p.Levels {
				width = max(width, len(level))
			}
			if _, err := fmt.Fprintf(out, "profile %s, %s context:\n", name, p.Namespace); err != nil {
				return err
			}
			for _, level := range p.Levels {
				if v := values[level+"_id"]; v != "" {
					if _, err := fmt.Fprintf(out, "  %-*s  %s\n", width, level, v); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
}

func newContextClearCommand(deps *Deps, p ContextParams) *cobra.Command {

	c := &cobra.Command{
		Use:   "clear",
		Short: "Clear the named levels, or the whole context with no flags",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return updateContext(c, deps, p, func(prof *Profile) error {
				var attrs []string
				for _, level := range p.Levels {
					if on, _ := c.Flags().GetBool(levelFlag(level)); on {
						attrs = append(attrs, level+"_id")
					}
				}
				prof.ClearContext(p.Namespace, attrs...)
				return nil
			})
		},
	}
	for _, level := range p.Levels {
		c.Flags().Bool(levelFlag(level), false, "clear the stored "+level)
	}
	return c
}

// updateContext loads the selected profile, applies change, and saves.
func updateContext(c *cobra.Command, deps *Deps, p ContextParams, change func(*Profile) error) error {

	f, err := deps.Profiles.Load()
	if err != nil {
		return err
	}
	name, prof, err := f.Select(deps.Profile)
	if err != nil {
		return err
	}
	if prof == nil {
		return errNoProfile(c)
	}

	if err := change(prof); err != nil {
		return err
	}
	if err := deps.Profiles.Save(f); err != nil {
		return err
	}
	_, err = fmt.Fprintf(c.OutOrStdout(), "Updated the %s context in profile %s\n", p.Namespace, name)
	return err
}

func flagList(levels []string) string {
	flags := make([]string, len(levels))
	for i, l := range levels {
		flags[i] = "--" + levelFlag(l)
	}
	return strings.Join(flags, ", ")
}
