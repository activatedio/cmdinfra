package cmd

import (
	gentf "github.com/activatedio/tfinfra/genlib/tf"
)

// Spec is the root input to generation: one CLI surface over an AIP-shaped
// gRPC API, mirroring the gcloud command shape
// (<root> <service> <resource-plural> <verb>).
type Spec struct {
	// Package is the Go package name of the generated files.
	Package string
	// Root describes the generated CLI's root command.
	Root Root
	// Entries describe the API resources, one per pb message type, reusing
	// tfinfra's Entry so a surface can eventually be declared once and
	// shared between the CLI and Terraform specs. Implementations carry the
	// CLI markers (Resource, Columns, FieldFlags, Associate, Search).
	//
	// PENDING: the command generators are not yet implemented — declaring
	// entries panics.
	Entries []gentf.Entry
	// Groups declare per-group behaviour of the service command groups the
	// entries name in Resource.Group. Today that is the group's context:
	// the scope defaults its commands fall back to, stored in the selected
	// profile. A group with no declaration reads no context.
	Groups []Group
}

// Group declares one service command group's context — the AWS
// account-and-region analog for an AIP scope tree:
//
//	gencmd.Group{Name: "identity", Context: []string{"tenant", "issuer", "realm", "audience"}}
//	gencmd.Group{Name: "access", SharesContext: "identity"}
//
// A group with Context gets a generated `<group> context set|show|clear`
// whose flags are exactly those levels (--tenant, --issuer, ...), and its
// verbs fill an absent --<level>-id from the profile's context in the
// group's namespace. A level is worth storing when an operator works inside
// it for a session; the thing a command is about stays a flag.
type Group struct {
	// Name is the group, as entries name it in Resource.Group.
	Name string
	// Context lists the storable scope levels: identifier attributes
	// without the _id suffix ("tenant" for tenant_id). Each must be a
	// scope level of at least one of the group's entries.
	Context []string
	// SharesContext names another group whose namespace this one reads
	// and writes, for two groups over one scope tree. Its context verb is
	// an alias of that group's. Exclusive with Context.
	SharesContext string
}

// Root declares the root command of the generated CLI.
type Root struct {
	// Use is the one-word usage line — the binary name.
	Use string
	// Short is the description shown in help output.
	Short string
}
