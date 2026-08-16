package cmd

import (
	"reflect"

	gentf "github.com/activatedio/tfinfra/genlib/tf"
	runtimetf "github.com/activatedio/tfinfra/pkg/tf"
)

// Resource declares a CLI command group for the entry:
//
//	<root> <group> <resource-plural> <verb> [name] [flags]
//	awctl identity realms create --tenant-id=... --display-name=...
//
// The structural surface (flags, completion values) comes from the pb type
// via protoreflect; verbs derive from Ops. Field names in marker lists are
// proto field names (snake_case); referencing an unknown field panics at
// generation time.
type Resource struct {
	// Scope is the resource's position in the AIP hierarchy; it contributes
	// one scope flag per parent collection (e.g. "--tenant-id"), resolved
	// against the active context when omitted.
	Scope runtimetf.Scope
	// Ops selects which operations the API exposes; the zero value means
	// all (OpAll). Verbs derive from it — see VerbsFor.
	Ops gentf.Ops
	// Group is the service command group the resource lives under
	// (identity, access, ...). Required.
	Group string
	// Plural overrides the derived plural command group name (the kebab
	// plural of the entity name, e.g. "appearance-profiles").
	Plural string
	// ClientType is the gRPC client interface carrying this resource's
	// operations, e.g. reflect.TypeFor[identitypb.AuthwiseIdentityServiceClient]().
	// Required.
	ClientType reflect.Type
	// Client is the Deps.Clients key the generated commands read the
	// client from. Defaults to "default".
	Client string
	// Collection overrides the derived AIP collection name (lower-camel
	// plural of the entity name, e.g. "appearanceProfiles") used to compose
	// and parse resource names.
	Collection string
}

// Columns declares the default column set for list and describe table
// output, replacing per-consumer bespoke field generators. Absent, output
// defaults to "name" plus "display_name" when the message has one.
type Columns struct {
	// Default lists proto field names (snake_case) in display order.
	Default []string
}

// FieldFlags tunes flag derivation for the entry's fields.
type FieldFlags struct {
	// Exclude lists proto fields that get no flag.
	Exclude []string
	// Rename maps a proto field to an explicit flag name, replacing the
	// derived kebab-case name.
	Rename map[string]string
	// Sensitive lists proto fields whose values are prompted for and never
	// echoed, instead of being read from a visible flag argument.
	Sensitive []string
}

// Associate declares association verbs for the entry — the CLI surface of
// the AIP Associate*/List*By* RPC family:
//
//	awctl identity users add-roles <user> --roles=...
//	awctl identity users remove-roles <user> --roles=...
//
// PENDING: declaring it panics until the command generators land.
type Associate struct {
	// Target is the associated pb entity type, e.g.
	// reflect.TypeFor[identitypb.Role]().
	Target reflect.Type
	// VerbPrefix overrides the derived association noun: verbs are
	// "add-<noun>" and "remove-<noun>", the noun defaulting to the kebab
	// plural of the Target message name.
	VerbPrefix string
}

// Search declares a search verb backed by the service's Search RPC.
// PENDING: declaring it panics until the command generators land.
type Search struct{}
