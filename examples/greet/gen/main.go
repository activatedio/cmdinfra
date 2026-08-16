// The generator entry point for the greet example. Regenerate with
// `go generate ./...` or `go run .` from this directory.
package main

//go:generate go run .

import (
	gencmd "github.com/activatedio/cmdinfra/genlib/cmd"
)

func main() {

	gencmd.NewRegistry().RunDirectoryPathHandler("../generated", &gencmd.Spec{
		Package: "generated",
		Root: gencmd.Root{
			Use:   "greet",
			Short: "A minimal cmdinfra-generated CLI",
		},
	})
}
