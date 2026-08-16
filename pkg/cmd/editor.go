package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/yaml"
)

// EditParams configures the $EDITOR session.
type EditParams struct {
	// Editor is the editor argv, e.g. strings.Fields(os.Getenv("EDITOR")).
	Editor []string
}

// Edit runs the editor patch-diff flow for one entity: render the current
// entity to a temporary YAML file, run the editor on it, decode the
// result, and compute the update mask from the fields that actually
// changed — so an edit patches exactly what the user touched instead of
// replacing the entity. An empty mask means the user changed nothing.
func Edit[E proto.Message](ctx context.Context, params EditParams, current E) (E, []string, error) {

	var none E

	if len(params.Editor) == 0 {
		return none, nil, fmt.Errorf("no editor configured (set $EDITOR)")
	}

	jsonData, err := recordMarshaller.Marshal(current)
	if err != nil {
		return none, nil, err
	}
	yamlData, err := yaml.JSONToYAML(jsonData)
	if err != nil {
		return none, nil, err
	}

	file, err := os.CreateTemp("", "*.yaml")
	if err != nil {
		return none, nil, err
	}
	defer func() { _ = os.Remove(file.Name()) }()

	if _, err := file.Write(yamlData); err != nil {
		return none, nil, err
	}
	if err := file.Close(); err != nil {
		return none, nil, err
	}

	//nolint:gosec // running the user's own $EDITOR on the temp file is the feature
	editor := exec.CommandContext(ctx, params.Editor[0], append(params.Editor[1:], file.Name())...)
	editor.Stdin = os.Stdin
	editor.Stdout = os.Stdout
	editor.Stderr = os.Stderr
	if err := editor.Run(); err != nil {
		return none, nil, fmt.Errorf("editor: %w", err)
	}

	edited, err := os.ReadFile(file.Name())
	if err != nil {
		return none, nil, err
	}

	entity, err := DecodeEntity[E](edited)
	if err != nil {
		return none, nil, err
	}

	return entity, FieldDiff(current, entity), nil
}

// FieldDiff returns the top-level proto field names (the update mask) whose
// values differ between the two messages. The AIP "name" field is identity,
// not payload, and never appears in the mask.
func FieldDiff(before, after proto.Message) []string {

	b := before.ProtoReflect()
	a := after.ProtoReflect()
	fds := b.Descriptor().Fields()

	var mask []string

	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		if string(fd.Name()) == NameField {
			continue
		}

		// Isolate the field on fresh messages so proto.Equal compares just
		// this field, whatever its kind.
		fb := b.New()
		if b.Has(fd) {
			fb.Set(fd, b.Get(fd))
		}
		fa := a.New()
		if a.Has(fd) {
			fa.Set(fd, a.Get(fd))
		}

		if !proto.Equal(fb.Interface(), fa.Interface()) {
			mask = append(mask, string(fd.Name()))
		}
	}

	return mask
}

// NameField is the AIP resource name field.
const NameField = "name"
