package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/proto"
)

// The generated verbs delegate to these runners: cobra boilerplate stays
// generated, behavior stays in one reviewed runtime.

// AddOutputFlags registers the read-side output flags: gcloud-style
// --format plus the --yaml/--json compatibility aliases, and --fields to
// override the default columns.
func AddOutputFlags(c *cobra.Command) {
	c.Flags().String("format", "", "output format: table (default), yaml, json")
	c.Flags().Bool("yaml", false, "shorthand for --format=yaml")
	c.Flags().Bool("json", false, "shorthand for --format=json")
	c.Flags().String("fields", "", "comma-separated output columns, overriding the defaults")
}

// AddFileFlag registers --file for entity-based create and update.
func AddFileFlag(c *cobra.Command) {
	c.Flags().String("file", "", "path to a yaml or json document for the entity")
}

// ChangedRecord builds the string record from the flags the user actually
// set — unset flags stay out, so patch masks cover exactly what was given.
// fields maps flag name to proto field name.
func ChangedRecord(flags *pflag.FlagSet, fields map[string]string) StringsRecord {

	rec := StringsRecord{}
	for flag, proto := range fields {
		if flags.Changed(flag) {
			rec[proto] = flags.Lookup(flag).Value.String()
		}
	}
	return rec
}

// EditorArgv resolves the editor command: $EDITOR, then $VISUAL, then vi.
func EditorArgv() []string {
	for _, env := range []string{"EDITOR", "VISUAL"} {
		if v := os.Getenv(env); v != "" {
			return strings.Fields(v)
		}
	}
	return []string{"vi"}
}

// renderParamsFrom resolves the output flags into a renderer plus the field
// list.
func renderParamsFrom(c *cobra.Command, svc BaseRetrievalService, masked []string) (Renderer, FieldList, error) {

	format, _ := c.Flags().GetString("format")
	if y, _ := c.Flags().GetBool("yaml"); y {
		format = "yaml"
	}
	if j, _ := c.Flags().GetBool("json"); j {
		format = "json"
	}

	fields := svc.DefaultFieldList()
	if override, _ := c.Flags().GetString("fields"); override != "" {
		fields = FieldList(strings.Split(override, ","))
	}

	r, err := NewRenderer(RendererParams{Format: format, Masked: masked})
	return r, fields, err
}

// fileEntity decodes --file when given.
func fileEntity[E proto.Message](c *cobra.Command) (E, error) {

	var none E

	path, _ := c.Flags().GetString("file")
	if path == "" {
		return none, nil
	}

	data, err := os.ReadFile(path) //nolint:gosec // reading the user's --file argument is the feature
	if err != nil {
		return none, err
	}
	return DecodeEntity[E](data)
}

// RunList lists the parent's entities and renders them.
func RunList(c *cobra.Command, svc ListService, r Resolver, masked []string) error {

	parent, err := r.Parent()
	if err != nil {
		return err
	}

	records, err := svc.List(c.Context(), ListParams{RetrievalParams: RetrievalParams{Parent: parent}})
	if err != nil {
		return err
	}

	renderer, fields, err := renderParamsFrom(c, svc, masked)
	if err != nil {
		return err
	}
	return renderer.RenderList(records, fields, c.OutOrStdout())
}

// RunDescribe reads one entity by short ID or full name and renders it.
func RunDescribe(c *cobra.Command, svc GetService, r Resolver, collection, arg string, masked []string) error {

	name, err := r.Name(collection, arg)
	if err != nil {
		return err
	}

	record, err := svc.Get(c.Context(), GetParams{Name: name})
	if err != nil {
		return err
	}

	renderer, fields, err := renderParamsFrom(c, svc, masked)
	if err != nil {
		return err
	}
	return renderer.RenderSingle(record, fields, c.OutOrStdout())
}

// RunCreate creates an entity from the set flags (and --file, when given)
// under the resolved parent.
func RunCreate[E proto.Message](c *cobra.Command, svc CreateService[E], r Resolver, fields map[string]string) error {

	parent, err := r.Parent()
	if err != nil {
		return err
	}

	entity, err := fileEntity[E](c)
	if err != nil {
		return err
	}

	key, err := svc.Create(c.Context(), CreateParams[E]{
		Record: ChangedRecord(c.Flags(), fields),
		Entity: entity,
		Parent: parent,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(c.OutOrStdout(), "Created %s\n", key)
	return err
}

// RunPatch updates exactly the set flags' fields (the field-mask patch
// behind the update verb).
func RunPatch[E proto.Message](c *cobra.Command, svc UpdateService[E], r Resolver, collection, arg string, fields map[string]string) error {

	name, err := r.Name(collection, arg)
	if err != nil {
		return err
	}

	key, err := svc.Patch(c.Context(), PatchParams{
		Record: ChangedRecord(c.Flags(), fields),
		Name:   name,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(c.OutOrStdout(), "Updated %s\n", key)
	return err
}

// RunUpdateFile replaces the entity from --file (the full-replace update
// verb for surfaces without Patch).
func RunUpdateFile[E proto.Message](c *cobra.Command, svc UpdateService[E], r Resolver, collection, arg string) error {

	name, err := r.Name(collection, arg)
	if err != nil {
		return err
	}

	entity, err := fileEntity[E](c)
	if err != nil {
		return err
	}
	if !entity.ProtoReflect().IsValid() {
		return fmt.Errorf("update requires --file with the full entity")
	}

	key, err := svc.Update(c.Context(), UpdateParams[E]{Entity: entity, Name: name})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(c.OutOrStdout(), "Updated %s\n", key)
	return err
}

// RunEdit reads the entity, runs the $EDITOR session, and patches exactly
// the fields the user touched.
func RunEdit[E proto.Message](c *cobra.Command, svc *Crud[E], r Resolver, collection, arg string) error {

	name, err := r.Name(collection, arg)
	if err != nil {
		return err
	}

	current, err := svc.GetEntity(c.Context(), name)
	if err != nil {
		return err
	}

	edited, mask, err := Edit(c.Context(), EditParams{Editor: EditorArgv()}, current)
	if err != nil {
		return err
	}
	if len(mask) == 0 {
		_, err = fmt.Fprintln(c.OutOrStdout(), "No changes.")
		return err
	}

	key, err := svc.PatchEntity(c.Context(), name, edited, mask)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(c.OutOrStdout(), "Updated %s\n", key)
	return err
}

// RunEditReplace is the edit flow for surfaces without Patch: any change
// replaces the whole entity.
func RunEditReplace[E proto.Message](c *cobra.Command, svc *Crud[E], r Resolver, collection, arg string) error {

	name, err := r.Name(collection, arg)
	if err != nil {
		return err
	}

	current, err := svc.GetEntity(c.Context(), name)
	if err != nil {
		return err
	}

	edited, mask, err := Edit(c.Context(), EditParams{Editor: EditorArgv()}, current)
	if err != nil {
		return err
	}
	if len(mask) == 0 {
		_, err = fmt.Fprintln(c.OutOrStdout(), "No changes.")
		return err
	}

	key, err := svc.Update(c.Context(), UpdateParams[E]{Entity: edited, Name: name})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(c.OutOrStdout(), "Updated %s\n", key)
	return err
}

// RunDelete deletes the entity by short ID or full name.
func RunDelete(c *cobra.Command, svc DeleteService, r Resolver, collection, arg string) error {

	name, err := r.Name(collection, arg)
	if err != nil {
		return err
	}

	key, err := svc.Delete(c.Context(), DeleteParams{Name: name})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(c.OutOrStdout(), "Deleted %s\n", key)
	return err
}

// CompleteNames is the positional-argument completion for verbs taking a
// resource name: it lists under the context-resolved parent and offers the
// full names. Errors mean no completions, never a broken shell.
func CompleteNames(c *cobra.Command, services func() (ListService, Resolver, error)) ([]string, cobra.ShellCompDirective) {

	svc, r, err := services()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	parent, err := r.Parent()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	records, err := svc.List(c.Context(), ListParams{RetrievalParams: RetrievalParams{Parent: parent}})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, rec := range records {
		if name, ok := rec[NameField].(string); ok && name != "" {
			names = append(names, name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
