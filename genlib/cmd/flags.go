package cmd

import (
	"fmt"
	"reflect"
	"strings"

	gentf "github.com/activatedio/tfinfra/genlib/tf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Flag is the normalized view of one proto field as a command-line flag:
// tfinfra's protoreflect field normalization plus the CLI naming layer.
type Flag struct {
	// Field is the normalized proto field (identity, Go binding, kind, enum
	// values for completion, sensitivity).
	Field gentf.Field
	// Name is the flag name without dashes, kebab-case
	// ("display_name" → "display-name"), after any Rename override.
	Name string
}

// NormalizeFlags returns the entry's fields as flags in proto field-number
// order. The AIP "name" field is never a flag — it is the verb's positional
// argument. Message-typed fields (Any, Struct, concrete messages) surface
// as protojson string flags automatically; unlike the Terraform side there
// is no explicit JSON list, because a CLI has no typed attribute lane.
// Unknown field references in FieldFlags panic — generation failures must
// be loud.
func NormalizeFlags(e gentf.Entry, ff FieldFlags) []Flag {

	fields := normalizedFields(e)
	entity := entityType(e).Name()

	validateFieldFlags(entity, fields, ff)

	excluded := map[string]bool{}
	for _, n := range ff.Exclude {
		excluded[n] = true
	}
	sensitive := map[string]bool{}
	for _, n := range ff.Sensitive {
		sensitive[n] = true
	}

	flags := make([]Flag, 0, len(fields))
	seen := map[string]string{}

	for _, f := range fields {
		if f.ProtoName == gentf.NameField || excluded[f.ProtoName] {
			continue
		}
		f.Sensitive = f.Sensitive || sensitive[f.ProtoName]

		name := ff.Rename[f.ProtoName]
		if name == "" {
			name = strings.ReplaceAll(f.ProtoName, "_", "-")
		}
		if prior, ok := seen[name]; ok {
			panic(fmt.Sprintf("%s: flag name %q derived for both %q and %q", entity, name, prior, f.ProtoName))
		}
		seen[name] = f.ProtoName

		flags = append(flags, Flag{Field: f, Name: name})
	}

	return flags
}

// validateFieldFlags panics when a FieldFlags list references a proto field
// that does not exist on the entity.
func validateFieldFlags(entity string, fields []gentf.Field, ff FieldFlags) {

	byName := map[string]bool{}
	for _, f := range fields {
		byName[f.ProtoName] = true
	}

	validate := func(list []string, label string) {
		for _, n := range list {
			if !byName[n] {
				panic(fmt.Sprintf("%s: FieldFlags.%s references unknown field %q", entity, label, n))
			}
		}
	}
	validate(ff.Exclude, "Exclude")
	validate(ff.Sensitive, "Sensitive")

	renames := make([]string, 0, len(ff.Rename))
	for n := range ff.Rename {
		renames = append(renames, n)
	}
	validate(renames, "Rename")
}

// normalizedFields runs tfinfra's protoreflect normalization over the
// entry, auto-marking every message-typed field (except Timestamp) as
// JSON — the CLI's protojson string lane.
func normalizedFields(e gentf.Entry) []gentf.Field {
	return gentf.NormalizeFields(e, gentf.Resource{JSON: autoJSON(e)})
}

// autoJSON collects the message-typed field names that surface as protojson
// flags: Any, Struct, and any concrete message. Timestamps stay RFC 3339
// strings.
func autoJSON(e gentf.Entry) []string {

	t := entityType(e)
	msg, ok := reflect.New(t).Interface().(proto.Message)
	if !ok {
		panic(fmt.Sprintf("entry type %s is not a proto.Message", t))
	}

	fds := msg.ProtoReflect().Descriptor().Fields()

	var names []string
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		if fd.IsMap() || fd.IsList() || fd.Kind() != protoreflect.MessageKind {
			continue
		}
		if fd.Message().FullName() == "google.protobuf.Timestamp" {
			continue
		}
		names = append(names, string(fd.Name()))
	}
	return names
}

// entityType returns the entry's message struct type, unwrapping a pointer
// if the spec declared one.
func entityType(e gentf.Entry) reflect.Type {
	t := e.Type
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("entry type %s is not a struct", e.Type))
	}
	return t
}
