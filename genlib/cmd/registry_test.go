package cmd_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	petstorev1 "github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1"
	gentf "github.com/activatedio/tfinfra/genlib/tf"
	"github.com/activatedio/tfinfra/pkg/aip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/activatedio/cmdinfra/genlib/cmd"
)

func TestRegistry_Generate(t *testing.T) {

	type s struct {
		arrange func() *cmd.Spec
		assert  func(t *testing.T, dir string, run func())
	}

	greetSpec := func() *cmd.Spec {
		return &cmd.Spec{
			Package: "generated",
			Root: cmd.Root{
				Use:   "greet",
				Short: "A minimal cmdinfra-generated CLI",
			},
		}
	}

	// petIn declares Pet under group, optionally with a Resource.Name — two
	// of them stand in for one message name from two packages.
	petIn := func(group, name string) gentf.Entry {
		return gentf.Entry{
			Type: reflect.TypeFor[petstorev1.Pet](),
			Implementations: []any{cmd.Resource{
				Scope:      aip.NewScope("stores"),
				Group:      group,
				ClientType: reflect.TypeFor[petstorev1.PetStoreServiceClient](),
				Name:       name,
			}},
		}
	}

	cases := map[string]s{
		"two entries with one message name panic": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Entries = []gentf.Entry{petIn("store", ""), petIn("vet", "")}
				return spec
			},
			assert: func(t *testing.T, _ string, run func()) {
				assert.PanicsWithValue(t, `github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1.Pet: generated name "Pet" is already used by github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1.Pet; set Resource.Name on one of them`, run)
			},
		},
		"Resource.Name separates the generated file and identifiers, not the command": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Entries = []gentf.Entry{petIn("store", ""), petIn("vet", "VetPet")}
				return spec
			},
			assert: func(t *testing.T, dir string, run func()) {
				run()
				require.FileExists(t, filepath.Join(dir, "pet_cmd_gen.go"))
				got, err := os.ReadFile(filepath.Join(dir, "vet_pet_cmd_gen.go"))
				require.NoError(t, err)
				assert.Contains(t, string(got), "func NewVetPetCommand(")
				assert.Contains(t, string(got), "var vetPetFlagFields = ")
				assert.Contains(t, string(got), `Use:   "pets",`)
				index, err := os.ReadFile(filepath.Join(dir, "index_cmd_gen.go"))
				require.NoError(t, err)
				assert.Contains(t, string(index), "NewPetCommand(deps)")
				assert.Contains(t, string(index), "NewVetPetCommand(deps)")
			},
		},
		"a Resource.Name that is not an exported identifier panics": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Entries = []gentf.Entry{petIn("vet", "vet-pet")}
				return spec
			},
			assert: func(t *testing.T, _ string, run func()) {
				assert.PanicsWithValue(t, `Pet: Resource.Name "vet-pet" must be an exported Go identifier`, run)
			},
		},
		"a Group context emits the context verb and a namespaced resolver": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Entries = []gentf.Entry{petIn("store", ""), petIn("vet", "VetPet")}
				spec.Groups = []cmd.Group{
					{Name: "store", Context: []string{"store"}},
					{Name: "vet", SharesContext: "store"},
				}
				return spec
			},
			assert: func(t *testing.T, dir string, run func()) {
				run()
				index, err := os.ReadFile(filepath.Join(dir, "index_cmd_gen.go"))
				require.NoError(t, err)
				assert.Contains(t, string(index), "Namespace: \"store\",\n\t}))")
				assert.Contains(t, string(index), "SharedWith: \"store\",")
				vet, err := os.ReadFile(filepath.Join(dir, "vet_pet_cmd_gen.go"))
				require.NoError(t, err)
				// The sharing group reads the owner's namespace, and its
				// error points at its own (alias) verb.
				assert.Contains(t, string(vet), `Namespace:  "store",`)
				assert.Contains(t, string(vet), `SetCommand: "greet vet context set",`)
			},
		},
		"a group without a Group declaration reads no context": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Entries = []gentf.Entry{petIn("store", "")}
				return spec
			},
			assert: func(t *testing.T, dir string, run func()) {
				run()
				pet, err := os.ReadFile(filepath.Join(dir, "pet_cmd_gen.go"))
				require.NoError(t, err)
				assert.NotContains(t, string(pet), "Namespace")
				assert.Contains(t, string(pet), `"store_id scope identifier")`)
				index, err := os.ReadFile(filepath.Join(dir, "index_cmd_gen.go"))
				require.NoError(t, err)
				assert.NotContains(t, string(index), "NewContextCommand")
			},
		},
		"a Context level no entry of the group has panics": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Entries = []gentf.Entry{petIn("store", "")}
				spec.Groups = []cmd.Group{{Name: "store", Context: []string{"store", "aisle"}}}
				return spec
			},
			assert: func(t *testing.T, _ string, run func()) {
				assert.PanicsWithValue(t, `cmdinfra: Group "store": Context level "aisle" is no scope level of a store entry`, run)
			},
		},
		"SharesContext naming an unknown group panics": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Entries = []gentf.Entry{petIn("store", "")}
				spec.Groups = []cmd.Group{{Name: "store", SharesContext: "identity"}}
				return spec
			},
			assert: func(t *testing.T, _ string, run func()) {
				assert.PanicsWithValue(t, `cmdinfra: Group "store": SharesContext names "identity", which declares no Context`, run)
			},
		},
		"a Group no entry belongs to panics": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Entries = []gentf.Entry{petIn("store", "")}
				spec.Groups = []cmd.Group{{Name: "stroe", Context: []string{"store"}}}
				return spec
			},
			assert: func(t *testing.T, _ string, run func()) {
				assert.PanicsWithValue(t, `cmdinfra: Group "stroe": no entry declares Resource.Group "stroe"`, run)
			},
		},
		"Context and SharesContext together panic": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Entries = []gentf.Entry{petIn("store", ""), petIn("vet", "VetPet")}
				spec.Groups = []cmd.Group{
					{Name: "store", Context: []string{"store"}},
					{Name: "vet", Context: []string{"store"}, SharesContext: "store"},
				}
				return spec
			},
			assert: func(t *testing.T, _ string, run func()) {
				assert.PanicsWithValue(t, `cmdinfra: Group "vet": Context and SharesContext are exclusive`, run)
			},
		},
		"greet spec regenerates the golden byte-identically": {
			arrange: greetSpec,
			assert: func(t *testing.T, dir string, run func()) {
				run()
				got, err := os.ReadFile(filepath.Join(dir, "root_gen.go"))
				require.NoError(t, err)
				want, err := os.ReadFile(filepath.Join("..", "..", "examples", "greet", "generated", "root_gen.go"))
				require.NoError(t, err)
				assert.Equal(t, string(want), string(got))
			},
		},
		"missing package panics": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Package = ""
				return spec
			},
			assert: func(t *testing.T, _ string, run func()) {
				assert.PanicsWithValue(t, "cmdinfra: Spec.Package must be set", run)
			},
		},
		"missing root use panics": {
			arrange: func() *cmd.Spec {
				spec := greetSpec()
				spec.Root.Use = ""
				return spec
			},
			assert: func(t *testing.T, _ string, run func()) {
				assert.PanicsWithValue(t, "cmdinfra: Spec.Root.Use must be set", run)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			spec := tc.arrange()
			dir := t.TempDir()

			tc.assert(t, dir, func() {
				cmd.NewRegistry().RunDirectoryPathHandler(dir, spec)
			})
		})
	}
}
