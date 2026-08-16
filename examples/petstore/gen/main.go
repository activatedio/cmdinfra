// The generator entry point for the petstore example CLI: the declarative
// spec over tfinfra's petstore pb types. Regenerate with `go generate
// ./...` or `go run .` from this directory.
package main

//go:generate go run .

import (
	"reflect"

	petstorev1 "github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1"
	gentf "github.com/activatedio/tfinfra/genlib/tf"
	"github.com/activatedio/tfinfra/pkg/aip"

	gencmd "github.com/activatedio/cmdinfra/genlib/cmd"
)

// The example service's scope table. Consumers declare their own; cmdinfra
// predefines none.
var scopeStore = aip.NewScope("stores")

func main() {

	gencmd.NewRegistry().RunDirectoryPathHandler("../generated", &gencmd.Spec{
		Package: "generated",
		Root: gencmd.Root{
			Use:   "petstore",
			Short: "A cmdinfra-generated petstore CLI",
		},
		Entries: []gentf.Entry{
			{
				Type: reflect.TypeFor[petstorev1.Pet](),
				Implementations: []any{
					gencmd.Resource{
						Scope:      scopeStore,
						Group:      "petstore",
						ClientType: reflect.TypeFor[petstorev1.PetStoreServiceClient](),
						Client:     "petstore",
					},
					gencmd.Columns{Default: []string{"name", "display_name", "type"}},
					gencmd.FieldFlags{Sensitive: []string{"metadata"}},
					gencmd.Associate{Target: reflect.TypeFor[petstorev1.Toy]()},
				},
			},
		},
	})
}
