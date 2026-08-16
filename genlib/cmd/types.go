package cmd

// Spec is the root input to generation: one CLI surface. The bootstrap
// shape carries only the root command; the spec model task grows it to
// services (identity, guard, access), resources, and verbs mirroring the
// gcloud command shape.
type Spec struct {
	// Package is the Go package name of the generated files.
	Package string
	// Root describes the generated CLI's root command.
	Root Root
}

// Root declares the root command of the generated CLI.
type Root struct {
	// Use is the one-word usage line — the binary name.
	Use string
	// Short is the description shown in help output.
	Short string
}
