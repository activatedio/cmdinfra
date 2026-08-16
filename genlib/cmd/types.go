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
}

// Root declares the root command of the generated CLI.
type Root struct {
	// Use is the one-word usage line — the binary name.
	Use string
	// Short is the description shown in help output.
	Short string
}
