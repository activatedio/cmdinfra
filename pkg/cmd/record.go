package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sigs.k8s.io/yaml"
)

// lastEqualsFields are the map fields whose pairs split on their last '='.
var lastEqualsFields sync.Map // protoreflect.FullName -> struct{}

// SplitMapOnLastEquals makes the named map<string, string> field's k=v pairs
// split on their last '=' instead of their first, for a field whose keys may
// carry '=' and whose values never do (an attribute match such as
// attribute:<name>=<value> mapped to a level). Every other map field keeps
// the first-'=' split. Call it before the command runs, typically while
// wiring the command tree.
func SplitMapOnLastEquals(field protoreflect.FullName) {
	lastEqualsFields.Store(field, struct{}{})
}

func splitsOnLastEquals(fd protoreflect.FieldDescriptor) bool {
	_, ok := lastEqualsFields.Load(fd.FullName())
	return ok
}

var recordMarshaller = protojson.MarshalOptions{
	UseProtoNames:     true,
	EmitDefaultValues: true,
}

// ToRecord renders an entity for output: its protojson form decoded into a
// generic map.
func ToRecord(m proto.Message) (Record, error) {

	data, err := recordMarshaller.Marshal(m)
	if err != nil {
		return nil, err
	}

	out := Record{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ToRecordList renders a list of entities for output.
func ToRecordList[E proto.Message](in []E) ([]Record, error) {

	out := make([]Record, len(in))
	for i, m := range in {
		r, err := ToRecord(m)
		if err != nil {
			return nil, err
		}
		out[i] = r
	}
	return out, nil
}

// DecodeEntity parses a JSON or YAML document (--file input) into a fresh
// entity via protojson; unknown fields are errors.
func DecodeEntity[E proto.Message](data []byte) (E, error) {

	e := zero[E]()

	jsonData, err := yaml.YAMLToJSON(data)
	if err != nil {
		return e, fmt.Errorf("parsing document: %w", err)
	}

	if err := protojson.Unmarshal(jsonData, e); err != nil {
		return e, err
	}
	return e, nil
}

// ApplyRecord sets a record's string values onto the entity's proto fields
// by proto field name (snake_case, the flag-derivation input). Value
// formats: scalars parse naturally; enums take a value name; timestamps
// take RFC 3339; repeated strings take a comma-separated list;
// map<string, string> takes comma-separated k=v pairs, split on the first
// '=' (on the last for a field named to SplitMapOnLastEquals); message-typed
// fields (Any, Struct, concrete messages) take protojson. Unknown fields
// and unparseable values are errors naming the field.
func ApplyRecord(m proto.Message, rec StringsRecord) error {

	ref := m.ProtoReflect()
	fds := ref.Descriptor().Fields()

	for _, key := range rec.Keys() {
		fd := fds.ByName(protoreflect.Name(key))
		if fd == nil {
			return fmt.Errorf("%s: unknown field %q", ref.Descriptor().Name(), key)
		}
		if err := applyField(ref, fd, rec[key]); err != nil {
			return fmt.Errorf("field %q: %w", key, err)
		}
	}
	return nil
}

func applyField(ref protoreflect.Message, fd protoreflect.FieldDescriptor, value string) error {

	switch {
	case fd.IsMap():
		return applyMapField(ref, fd, value)
	case fd.IsList():
		return applyListField(ref, fd, value)
	default:
		return applyScalarField(ref, fd, value)
	}
}

// applyMapField replaces a map<string, string> field from comma-separated
// k=v pairs, each split on its first '=', or its last for a field named to
// SplitMapOnLastEquals.
func applyMapField(ref protoreflect.Message, fd protoreflect.FieldDescriptor, value string) error {

	if fd.MapKey().Kind() != protoreflect.StringKind || fd.MapValue().Kind() != protoreflect.StringKind {
		return fmt.Errorf("only map<string, string> fields are supported")
	}

	ref.Clear(fd)
	if value == "" {
		return nil
	}

	last := splitsOnLastEquals(fd)
	mp := ref.Mutable(fd).Map()
	for _, pair := range strings.Split(value, ",") {
		i := strings.Index(pair, "=")
		if last {
			i = strings.LastIndex(pair, "=")
		}
		if i < 0 {
			return fmt.Errorf("map entry %q is not key=value", pair)
		}
		k, v := pair[:i], pair[i+1:]
		mp.Set(protoreflect.ValueOfString(k).MapKey(), protoreflect.ValueOfString(v))
	}
	return nil
}

// applyListField replaces a repeated string field from a comma-separated
// list.
func applyListField(ref protoreflect.Message, fd protoreflect.FieldDescriptor, value string) error {

	if fd.Kind() != protoreflect.StringKind {
		return fmt.Errorf("only repeated string fields are supported")
	}

	ref.Clear(fd)
	if value == "" {
		return nil
	}

	l := ref.Mutable(fd).List()
	for _, item := range strings.Split(value, ",") {
		l.Append(protoreflect.ValueOfString(item))
	}
	return nil
}

func applyScalarField(ref protoreflect.Message, fd protoreflect.FieldDescriptor, value string) error {

	switch fd.Kind() {
	case protoreflect.StringKind:
		ref.Set(fd, protoreflect.ValueOfString(value))
	case protoreflect.BoolKind:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%q is not a bool", value)
		}
		ref.Set(fd, protoreflect.ValueOfBool(b))
	case protoreflect.EnumKind:
		return applyEnumField(ref, fd, value)
	case protoreflect.MessageKind:
		return applyMessageField(ref, fd, value)
	default:
		return applyNumericField(ref, fd, value)
	}
	return nil
}

func applyNumericField(ref protoreflect.Message, fd protoreflect.FieldDescriptor, value string) error {

	switch fd.Kind() {
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return applyIntField(ref, fd, value)
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return applyFloatField(ref, fd, value)
	default:
		return fmt.Errorf("field kind %s is not supported", fd.Kind())
	}
}

func applyIntField(ref protoreflect.Message, fd protoreflect.FieldDescriptor, value string) error {

	switch fd.Kind() {
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		n, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return fmt.Errorf("%q is not an int32", value)
		}
		ref.Set(fd, protoreflect.ValueOfInt32(int32(n)))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return fmt.Errorf("%q is not a uint32", value)
		}
		ref.Set(fd, protoreflect.ValueOfUint32(uint32(n)))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		n, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return fmt.Errorf("%q is not a uint64", value)
		}
		ref.Set(fd, protoreflect.ValueOfUint64(n))
	default:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("%q is not an int64", value)
		}
		ref.Set(fd, protoreflect.ValueOfInt64(n))
	}
	return nil
}

func applyFloatField(ref protoreflect.Message, fd protoreflect.FieldDescriptor, value string) error {

	if fd.Kind() == protoreflect.FloatKind {
		f, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return fmt.Errorf("%q is not a float", value)
		}
		ref.Set(fd, protoreflect.ValueOfFloat32(float32(f)))
		return nil
	}

	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("%q is not a double", value)
	}
	ref.Set(fd, protoreflect.ValueOfFloat64(f))
	return nil
}

func applyEnumField(ref protoreflect.Message, fd protoreflect.FieldDescriptor, value string) error {

	values := fd.Enum().Values()
	ev := values.ByName(protoreflect.Name(value))
	if ev == nil {
		names := make([]string, 0, values.Len())
		for i := 0; i < values.Len(); i++ {
			names = append(names, string(values.Get(i).Name()))
		}
		return fmt.Errorf("%q is not one of %s", value, strings.Join(names, ", "))
	}
	ref.Set(fd, protoreflect.ValueOfEnum(ev.Number()))
	return nil
}

// applyMessageField sets a message-typed field: timestamps from RFC 3339,
// durations from Go duration syntax, everything else (Any, Struct, concrete
// messages) from protojson.
func applyMessageField(ref protoreflect.Message, fd protoreflect.FieldDescriptor, value string) error {

	switch fd.Message().FullName() {
	case "google.protobuf.Timestamp":
		ts, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return fmt.Errorf("%q is not an RFC 3339 timestamp", value)
		}
		ref.Set(fd, protoreflect.ValueOfMessage(timestamppb.New(ts).ProtoReflect()))
		return nil
	case "google.protobuf.Duration":
		d, err := parseDuration(value)
		if err != nil {
			return err
		}
		ref.Set(fd, protoreflect.ValueOfMessage(durationpb.New(d).ProtoReflect()))
		return nil
	}

	msg := ref.NewField(fd).Message()
	if err := protojson.Unmarshal([]byte(value), msg.Interface()); err != nil {
		return err
	}
	ref.Set(fd, protoreflect.ValueOfMessage(msg))
	return nil
}

// parseDuration reads Go duration syntax, a superset of the protojson form
// ("1.5s") that also takes "500ms" and "1m30s" — tfinfra's FieldDuration
// syntax, so a value means the same on the CLI and in Terraform. The quoted
// protojson string ("\"1.5s\"") is accepted too: it is what the JSON lane
// took before durations were typed.
func parseDuration(value string) (time.Duration, error) {

	s := value
	if unquoted, err := strconv.Unquote(value); err == nil {
		s = unquoted
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration (e.g. 5s, 1.5s, 500ms)", value)
	}
	return d, nil
}

// NameOf returns the entity's AIP resource name, or "" when the message has
// no string "name" field.
func NameOf(m proto.Message) string {

	ref := m.ProtoReflect()
	fd := ref.Descriptor().Fields().ByName("name")
	if fd == nil || fd.Kind() != protoreflect.StringKind || fd.IsList() || fd.IsMap() {
		return ""
	}
	return ref.Get(fd).String()
}

// zero returns a fresh entity of the pointer type E.
func zero[E proto.Message]() E {
	var e E
	return e.ProtoReflect().New().Interface().(E)
}
