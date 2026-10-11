package cmd

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

// NewProfilesCommand returns the `config profiles` command group over the
// profile store: list, create, update, describe, activate and delete.
// fields are the consumer's settings — cmdinfra knows no endpoints of its
// own — and each becomes a flag on create and update. Contexts are not set
// here: each command group's generated `context` verb owns its namespace.
func NewProfilesCommand(deps *Deps, fields []ProfileField) *cobra.Command {

	for _, f := range fields {
		if f.Key == contextsKey {
			// A consumer wiring bug, caught on the first run of any command.
			panic(fmt.Sprintf("cmdinfra: ProfileField key %q is reserved", contextsKey))
		}
	}

	config := &cobra.Command{Use: "config", Short: "Manage CLI configuration"}
	profiles := &cobra.Command{
		Use:   "profiles",
		Short: "Manage profiles: where commands go and who they run as",
		Long: "A profile names an install: its endpoints and the issuer you log in at. It also\n" +
			"holds each command group's context, the scope defaults its commands fall back\n" +
			"to, so switching profile switches them too. One profile is active; --profile\n" +
			"picks another for a single command.",
	}
	config.AddCommand(profiles)

	profiles.AddCommand(
		newProfilesListCommand(deps),
		newProfilesCreateCommand(deps, fields),
		newProfilesUpdateCommand(deps, fields),
		newProfilesDescribeCommand(deps),
		newProfilesActivateCommand(deps),
		newProfilesDeleteCommand(deps),
	)
	return config
}

func newProfilesListCommand(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the profiles; * marks the active one",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			f, err := deps.Profiles.Load()
			if err != nil {
				return err
			}
			for _, name := range f.Names() {
				marker := " "
				if name == f.Active {
					marker = "*"
				}
				if _, err := fmt.Fprintf(c.OutOrStdout(), "%s %s\n", marker, name); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newProfilesCreateCommand(deps *Deps, fields []ProfileField) *cobra.Command {

	c := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a profile; the first one created becomes active",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {

			f, err := deps.Profiles.Load()
			if err != nil {
				return err
			}

			p := &Profile{}
			applyProfileFlags(c, fields, p)
			if err := f.Add(args[0], p); err != nil {
				return err
			}

			activated := f.Active == ""
			if activated {
				f.Active = args[0]
			}
			if err := deps.Profiles.Save(f); err != nil {
				return err
			}

			if activated {
				_, err = fmt.Fprintf(c.OutOrStdout(), "Created %s (active)\n", args[0])
			} else {
				_, err = fmt.Fprintf(c.OutOrStdout(), "Created %s\n", args[0])
			}
			return err
		},
	}
	addProfileFlags(c, fields)
	return c
}

func newProfilesUpdateCommand(deps *Deps, fields []ProfileField) *cobra.Command {

	c := &cobra.Command{
		Use:               "update <name>",
		Short:             "Change a profile's settings",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProfiles(deps),
		RunE: func(c *cobra.Command, args []string) error {

			f, err := deps.Profiles.Load()
			if err != nil {
				return err
			}
			p, err := f.Get(args[0])
			if err != nil {
				return err
			}
			if !applyProfileFlags(c, fields, p) {
				return errors.New("nothing to update: pass the settings to change")
			}
			if err := deps.Profiles.Save(f); err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Updated %s\n", args[0])
			return err
		},
	}
	addProfileFlags(c, fields)
	return c
}

func newProfilesDescribeCommand(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:               "describe [<name>]",
		Short:             "Show a profile, contexts included (default: the one in use)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeProfiles(deps),
		RunE: func(c *cobra.Command, args []string) error {

			f, err := deps.Profiles.Load()
			if err != nil {
				return err
			}
			explicit := deps.Profile
			if len(args) == 1 {
				explicit = args[0]
			}
			name, p, err := f.Select(explicit)
			if err != nil {
				return err
			}
			if p == nil {
				return errNoProfile(c)
			}
			return writeProfile(c.OutOrStdout(), name, p)
		},
	}
}

func newProfilesActivateCommand(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:               "activate <name>",
		Short:             "Make a profile the active one",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProfiles(deps),
		RunE: func(c *cobra.Command, args []string) error {
			f, err := deps.Profiles.Load()
			if err != nil {
				return err
			}
			if err := f.Activate(args[0]); err != nil {
				return err
			}
			if err := deps.Profiles.Save(f); err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Activated %s\n", args[0])
			return err
		},
	}
}

func newProfilesDeleteCommand(deps *Deps) *cobra.Command {
	return &cobra.Command{
		Use:               "delete <name>",
		Short:             "Delete a profile and its contexts (not the active one)",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeProfiles(deps),
		RunE: func(c *cobra.Command, args []string) error {
			f, err := deps.Profiles.Load()
			if err != nil {
				return err
			}
			if err := f.Delete(args[0]); err != nil {
				return err
			}
			if err := deps.Profiles.Save(f); err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Deleted %s\n", args[0])
			return err
		},
	}
}

// addProfileFlags registers one flag per consumer setting.
func addProfileFlags(c *cobra.Command, fields []ProfileField) {
	for _, f := range fields {
		if f.Bool {
			c.Flags().Bool(f.Flag(), false, f.Usage)
		} else {
			c.Flags().String(f.Flag(), "", f.Usage)
		}
	}
}

// applyProfileFlags copies the flags the user set into the profile and
// reports whether there were any. An empty string value removes the
// setting.
func applyProfileFlags(c *cobra.Command, fields []ProfileField, p *Profile) bool {

	changed := false
	for _, f := range fields {
		if !c.Flags().Changed(f.Flag()) {
			continue
		}
		changed = true
		v := c.Flags().Lookup(f.Flag()).Value.String()
		if f.Bool {
			v = strconv.FormatBool(parseBool(v))
		}
		if v == "" {
			delete(p.Settings, f.Key)
			continue
		}
		p.set(f.Key, v)
	}
	return changed
}

// writeProfile renders one profile as the document fragment it is stored
// as, under its name.
func writeProfile(w io.Writer, name string, p *Profile) error {

	data, err := yaml.Marshal(map[string]*Profile{name: p})
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// errNoProfile is the answer when a command needs a profile and none is
// selected.
func errNoProfile(c *cobra.Command) error {
	return fmt.Errorf("no profile is active: create one with `%s config profiles create <name>`", c.Root().Name())
}

// completeProfiles offers the profile names.
func completeProfiles(deps *Deps) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		f, err := deps.Profiles.Load()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return f.Names(), cobra.ShellCompDirectiveNoFileComp
	}
}
