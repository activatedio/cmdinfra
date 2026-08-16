// The petstore CLI acceptance tests: the generated command surface driven
// end to end — real cobra parsing, real gRPC wire — against an in-process
// fake AIP service.
package generated_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	petstorev1 "github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/activatedio/cmdinfra/examples/petstore/generated"
	pkgcmd "github.com/activatedio/cmdinfra/pkg/cmd"
)

type fakePetStore struct {
	petstorev1.UnimplementedPetStoreServiceServer

	mu            sync.Mutex
	pets          map[string]*petstorev1.Pet
	toys          map[string][]string
	seq           int
	lastPatchMask []string
}

func (f *fakePetStore) GetPet(_ context.Context, in *petstorev1.GetPetRequest) (*petstorev1.Pet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pets[in.GetName()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "pet %q not found", in.GetName())
	}
	return proto.Clone(p).(*petstorev1.Pet), nil
}

func (f *fakePetStore) ListPets(_ context.Context, in *petstorev1.ListPetsRequest) (*petstorev1.ListPetsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	names := make([]string, 0, len(f.pets))
	for name := range f.pets {
		names = append(names, name)
	}
	sort.Strings(names)

	start := 0
	if in.GetPageToken() != "" {
		if _, err := fmt.Sscanf(in.GetPageToken(), "page-%d", &start); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad page token")
		}
	}

	res := &petstorev1.ListPetsResponse{}
	end := min(start+2, len(names))
	for _, name := range names[start:end] {
		res.Pets = append(res.Pets, proto.Clone(f.pets[name]).(*petstorev1.Pet))
	}
	if end < len(names) {
		res.NextPageToken = fmt.Sprintf("page-%d", end)
	}
	return res, nil
}

func (f *fakePetStore) CreatePet(_ context.Context, in *petstorev1.CreatePetRequest) (*petstorev1.Pet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	p := proto.Clone(in.GetPet()).(*petstorev1.Pet)
	p.Name = fmt.Sprintf("%s/pets/p-%d", in.GetParent(), f.seq)
	f.pets[p.GetName()] = p
	return proto.Clone(p).(*petstorev1.Pet), nil
}

func (f *fakePetStore) PatchPet(_ context.Context, in *petstorev1.PatchPetRequest) (*petstorev1.Pet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.pets[in.GetName()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "pet %q not found", in.GetName())
	}
	f.lastPatchMask = in.GetUpdateMask().GetPaths()
	for _, path := range in.GetUpdateMask().GetPaths() {
		switch path {
		case "display_name":
			existing.DisplayName = in.GetPet().GetDisplayName()
		case "age":
			existing.Age = in.GetPet().GetAge()
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unsupported update_mask path %q", path)
		}
	}
	return proto.Clone(existing).(*petstorev1.Pet), nil
}

func (f *fakePetStore) DeletePet(_ context.Context, in *petstorev1.DeletePetRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.pets[in.GetName()]; !ok {
		return nil, status.Errorf(codes.NotFound, "pet %q not found", in.GetName())
	}
	delete(f.pets, in.GetName())
	return &emptypb.Empty{}, nil
}

func (f *fakePetStore) AssociateToysToPet(_ context.Context, in *petstorev1.AssociateToysToPetRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.pets[in.GetName()]; !ok {
		return nil, status.Errorf(codes.NotFound, "pet %q not found", in.GetName())
	}
	current := map[string]bool{}
	for _, t := range f.toys[in.GetName()] {
		current[t] = true
	}
	for _, t := range in.GetAssociation().GetSet() {
		current[t] = true
	}
	for _, t := range in.GetAssociation().GetRemove() {
		delete(current, t)
	}
	names := make([]string, 0, len(current))
	for t := range current {
		names = append(names, t)
	}
	sort.Strings(names)
	f.toys[in.GetName()] = names
	return &emptypb.Empty{}, nil
}

func (f *fakePetStore) ListToysByPet(_ context.Context, in *petstorev1.ListToysByPetRequest) (*petstorev1.ListToysByPetResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	res := &petstorev1.ListToysByPetResponse{}
	for _, name := range f.toys[in.GetName()] {
		res.Toys = append(res.Toys, &petstorev1.Toy{Name: name, DisplayName: "Toy " + name})
	}
	return res, nil
}

// harness boots the fake service and returns a CLI runner plus the fake for
// server-side assertions.
type harness struct {
	fake *fakePetStore
	deps *pkgcmd.Deps
}

func newHarness(t *testing.T) *harness {

	t.Helper()

	fake := &fakePetStore{pets: map[string]*petstorev1.Pet{}, toys: map[string][]string{}}

	lis, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	petstorev1.RegisterPetStoreServiceServer(s, fake)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := petstorev1.NewPetStoreServiceClient(conn)

	deps := &pkgcmd.Deps{
		Clients: map[string]func(ctx context.Context) (any, error){
			"petstore": func(context.Context) (any, error) { return client, nil },
		},
		Contexts: pkgcmd.ContextStore{Path: filepath.Join(t.TempDir(), "contexts.yaml")},
	}

	return &harness{fake: fake, deps: deps}
}

// run executes the CLI exactly as main wires it.
func (h *harness) run(t *testing.T, args ...string) (string, error) {

	t.Helper()

	root := generated.NewRootCommand()
	for _, c := range generated.Commands(h.deps) {
		root.AddCommand(c)
	}
	root.AddCommand(pkgcmd.NewConfigCommand(h.deps.Contexts))

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func TestCLI_Help(t *testing.T) {

	h := newHarness(t)

	out, err := h.run(t, "petstore", "pets", "--help")

	require.NoError(t, err)
	for _, verb := range []string{"create", "delete", "describe", "edit", "list", "update"} {
		assert.Contains(t, out, verb)
	}
	assert.Contains(t, out, "Manage pets")
}

func TestCLI_Lifecycle(t *testing.T) {

	h := newHarness(t)

	out, err := h.run(t, "petstore", "pets", "create",
		"--store-id", "s-1",
		"--display-name", "Rex",
		"--type", "PET_TYPE_DOG",
		"--tags", "loud,friendly",
		"--labels", "team=platform",
		"--config", `{"@type": "type.googleapis.com/petstore.v1.CollarConfig", "color": "red"}`,
	)
	require.NoError(t, err)
	assert.Equal(t, "Created stores/s-1/pets/p-1\n", out)

	server := h.fake.pets["stores/s-1/pets/p-1"]
	require.NotNil(t, server)
	assert.Equal(t, petstorev1.PetType_PET_TYPE_DOG, server.GetType())
	assert.Equal(t, []string{"loud", "friendly"}, server.GetTags())

	out, err = h.run(t, "petstore", "pets", "describe", "p-1", "--store-id", "s-1")
	require.NoError(t, err)
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "Rex")

	out, err = h.run(t, "petstore", "pets", "update", "p-1", "--store-id", "s-1", "--display-name", "Lord Rex")
	require.NoError(t, err)
	assert.Equal(t, "Updated stores/s-1/pets/p-1\n", out)
	assert.Equal(t, []string{"display_name"}, h.fake.lastPatchMask)
	assert.Equal(t, petstorev1.PetType_PET_TYPE_DOG, server.GetType())

	out, err = h.run(t, "petstore", "pets", "delete", "stores/s-1/pets/p-1")
	require.NoError(t, err)
	assert.Equal(t, "Deleted stores/s-1/pets/p-1\n", out)
	assert.Empty(t, h.fake.pets)
}

func TestCLI_ListPaginationAndContext(t *testing.T) {

	h := newHarness(t)

	// The active context supplies store_id — no --store-id flags below.
	_, err := h.run(t, "config", "contexts", "set", "dev", "store_id", "s-1")
	require.NoError(t, err)
	_, err = h.run(t, "config", "contexts", "activate", "dev")
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, err := h.run(t, "petstore", "pets", "create", "--display-name", fmt.Sprintf("Pet %d", i))
		require.NoError(t, err)
	}

	out, err := h.run(t, "petstore", "pets", "list")
	require.NoError(t, err)
	// Three pets over two pages, one table.
	assert.Equal(t, 4, strings.Count(out, "\n"))
	assert.Contains(t, out, "Pet 0")
	assert.Contains(t, out, "Pet 2")
}

func TestCLI_MissingScopeIsActionable(t *testing.T) {

	h := newHarness(t)

	_, err := h.run(t, "petstore", "pets", "list")

	require.ErrorContains(t, err, "missing store_id")
	require.ErrorContains(t, err, "--store-id")
}

func TestCLI_MaskedField(t *testing.T) {

	h := newHarness(t)

	_, err := h.run(t, "petstore", "pets", "create", "--store-id", "s-1",
		"--display-name", "Rex", "--metadata", `{"chip": "abc-123"}`)
	require.NoError(t, err)

	out, err := h.run(t, "petstore", "pets", "describe", "p-1", "--store-id", "s-1",
		"--fields", "name,metadata")
	require.NoError(t, err)
	assert.Contains(t, out, "********")
	assert.NotContains(t, out, "abc-123")

	// Machine formats print everything — they exist for piping.
	out, err = h.run(t, "petstore", "pets", "describe", "p-1", "--store-id", "s-1", "--json")
	require.NoError(t, err)
	assert.Contains(t, out, "abc-123")
}

func TestCLI_EditPatchesTouchedFields(t *testing.T) {

	h := newHarness(t)

	_, err := h.run(t, "petstore", "pets", "create", "--store-id", "s-1", "--display-name", "Rex")
	require.NoError(t, err)

	script := filepath.Join(t.TempDir(), "editor.sh")
	require.NoError(t, os.WriteFile(script, []byte(
		"#!/bin/sh\ncat > \"$1\" <<'DONE'\nname: stores/s-1/pets/p-1\ndisplay_name: Lord Rex\nDONE\n",
	), 0o700))
	t.Setenv("EDITOR", "/bin/sh "+script)

	out, err := h.run(t, "petstore", "pets", "edit", "p-1", "--store-id", "s-1")

	require.NoError(t, err)
	assert.Equal(t, "Updated stores/s-1/pets/p-1\n", out)
	assert.Equal(t, []string{"display_name"}, h.fake.lastPatchMask)
	assert.Equal(t, "Lord Rex", h.fake.pets["stores/s-1/pets/p-1"].GetDisplayName())
}

func TestCLI_NameCompletion(t *testing.T) {

	h := newHarness(t)

	_, err := h.run(t, "petstore", "pets", "create", "--store-id", "s-1", "--display-name", "Rex")
	require.NoError(t, err)

	out, err := h.run(t, "__complete", "petstore", "pets", "describe", "--store-id", "s-1", "")

	require.NoError(t, err)
	assert.Contains(t, out, "stores/s-1/pets/p-1")
}

func TestCLI_EnumFlagCompletion(t *testing.T) {

	h := newHarness(t)

	out, err := h.run(t, "__complete", "petstore", "pets", "create", "--type", "")

	require.NoError(t, err)
	assert.Contains(t, out, "PET_TYPE_DOG")
	assert.Contains(t, out, "PET_TYPE_CAT")
}

func TestCLI_Associations(t *testing.T) {

	h := newHarness(t)

	_, err := h.run(t, "petstore", "pets", "create", "--store-id", "s-1", "--display-name", "Rex")
	require.NoError(t, err)

	out, err := h.run(t, "petstore", "pets", "add-toys", "p-1", "--store-id", "s-1", "toys/ball", "toys/rope")
	require.NoError(t, err)
	assert.Equal(t, "Updated stores/s-1/pets/p-1\n", out)
	assert.Equal(t, []string{"toys/ball", "toys/rope"}, h.fake.toys["stores/s-1/pets/p-1"])

	out, err = h.run(t, "petstore", "pets", "list-toys", "p-1", "--store-id", "s-1")
	require.NoError(t, err)
	assert.Contains(t, out, "toys/ball")
	assert.Contains(t, out, "Toy toys/rope")

	_, err = h.run(t, "petstore", "pets", "remove-toys", "stores/s-1/pets/p-1", "toys/ball")
	require.NoError(t, err)
	assert.Equal(t, []string{"toys/rope"}, h.fake.toys["stores/s-1/pets/p-1"])
}
