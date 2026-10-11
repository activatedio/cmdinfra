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
	"time"

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
	toyEntities   map[string]*petstorev1.Toy
	seq           int
	lastPatchMask []string
}

// --- Toy: the caller-named lane. The create request carries the row's id in
// the entity's name field, and the server composes the full name.

func (f *fakePetStore) CreateToy(_ context.Context, in *petstorev1.CreateToyRequest) (*petstorev1.Toy, error) {

	f.mu.Lock()
	defer f.mu.Unlock()

	id := in.GetToy().GetName()
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "toy name is required")
	}

	t := proto.Clone(in.GetToy()).(*petstorev1.Toy)
	t.Name = fmt.Sprintf("%s/toys/%s", in.GetParent(), id)
	if _, exists := f.toyEntities[t.GetName()]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "toy %q already exists", t.GetName())
	}
	f.toyEntities[t.GetName()] = t

	return proto.Clone(t).(*petstorev1.Toy), nil
}

func (f *fakePetStore) GetToy(_ context.Context, in *petstorev1.GetToyRequest) (*petstorev1.Toy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.toyEntities[in.GetName()]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "toy %q not found", in.GetName())
	}
	return proto.Clone(t).(*petstorev1.Toy), nil
}

func (f *fakePetStore) DeleteToy(_ context.Context, in *petstorev1.DeleteToyRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.toyEntities[in.GetName()]; !ok {
		return nil, status.Errorf(codes.NotFound, "toy %q not found", in.GetName())
	}
	delete(f.toyEntities, in.GetName())
	return &emptypb.Empty{}, nil
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
	// terminal stands in for stderr being a terminal, which is when a write
	// echoes the scope it took from the context.
	terminal bool
	// stderr is what the last run wrote there.
	stderr string
}

func newHarness(t *testing.T) *harness {

	t.Helper()

	fake := &fakePetStore{
		pets:        map[string]*petstorev1.Pet{},
		toys:        map[string][]string{},
		toyEntities: map[string]*petstorev1.Toy{},
	}

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

	h := &harness{fake: fake}
	h.deps = &pkgcmd.Deps{
		Clients: map[string]func(ctx context.Context) (any, error){
			"petstore": func(context.Context) (any, error) { return client, nil },
		},
		Profiles: pkgcmd.ProfileStore{Path: filepath.Join(t.TempDir(), "petstore.d", "config.yaml")},
		Terminal: func() bool { return h.terminal },
	}

	return h
}

// run executes the CLI exactly as main wires it.
func (h *harness) run(t *testing.T, args ...string) (string, error) {

	t.Helper()

	// --profile binds into Deps; each run starts from the env default, as
	// a fresh process would.
	h.deps.Profile = ""

	root := generated.NewRootCommand()
	h.deps.BindProfileFlag(root, "PETSTORE_PROFILE")
	for _, c := range generated.Commands(h.deps) {
		root.AddCommand(c)
	}
	root.AddCommand(pkgcmd.NewProfilesCommand(h.deps, []pkgcmd.ProfileField{
		{Key: "endpoint", Usage: "the petstore API"},
		{Key: "insecure", Usage: "plaintext", Bool: true},
	}))

	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)

	err := root.ExecuteContext(context.Background())
	h.stderr = errOut.String()
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
		"--grooming-interval", "1m30s",
	)
	require.NoError(t, err)
	assert.Equal(t, "Created stores/s-1/pets/p-1\n", out)

	server := h.fake.pets["stores/s-1/pets/p-1"]
	require.NotNil(t, server)
	assert.Equal(t, petstorev1.PetType_PET_TYPE_DOG, server.GetType())
	assert.Equal(t, []string{"loud", "friendly"}, server.GetTags())
	assert.Equal(t, 90*time.Second, server.GetGroomingInterval().AsDuration())

	out, err = h.run(t, "petstore", "pets", "describe", "p-1", "--store-id", "s-1")
	require.NoError(t, err)
	// describe shows the full name where list shows the bare id.
	assert.Regexp(t, `^NAME +DISPLAY_NAME +TYPE\nstores/s-1/pets/p-1 +Rex +PET_TYPE_DOG\n$`, out)

	out, err = h.run(t, "petstore", "pets", "describe", "p-1", "--store-id", "s-1", "--fields", "id,name")
	require.NoError(t, err)
	assert.Regexp(t, `^ID +NAME\np-1 +stores/s-1/pets/p-1\n$`, out)

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

	// The profile's context supplies store_id — no --store-id flags below.
	_, err := h.run(t, "config", "profiles", "create", "dev")
	require.NoError(t, err)
	_, err = h.run(t, "petstore", "context", "set", "--store", "s-1")
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, err := h.run(t, "petstore", "pets", "create", "--display-name", fmt.Sprintf("Pet %d", i))
		require.NoError(t, err)
	}

	out, err := h.run(t, "petstore", "pets", "list")
	require.NoError(t, err)
	// Three pets over two pages, one table.
	assert.Equal(t, 4, strings.Count(out, "\n"))
	// Rows lead with the bare id: the store is the scope the caller gave.
	assert.Regexp(t, `^ID +DISPLAY_NAME +TYPE\np-1 +Pet 0`, out)
	assert.NotContains(t, out, "stores/s-1")
	assert.Contains(t, out, "Pet 0")
	assert.Contains(t, out, "Pet 2")
}

func TestCLI_MissingScopeIsActionable(t *testing.T) {

	h := newHarness(t)

	_, err := h.run(t, "petstore", "pets", "list")

	require.ErrorContains(t, err, "missing store_id")
	require.ErrorContains(t, err, "--store-id")
	require.ErrorContains(t, err, "`petstore petstore context set --store <id>`")
}

// TestCLI_Profiles covers the two tiers: a profile is where (its settings)
// and which (its contexts), one is active, and --profile picks another for
// one call — taking its context with it.
func TestCLI_Profiles(t *testing.T) {

	h := newHarness(t)

	// The first profile becomes active; the second waits to be activated.
	out, err := h.run(t, "config", "profiles", "create", "dev", "--endpoint", "dev:443", "--insecure")
	require.NoError(t, err)
	assert.Equal(t, "Created dev (active)\n", out)
	out, err = h.run(t, "config", "profiles", "create", "prod", "--endpoint", "prod:443")
	require.NoError(t, err)
	assert.Equal(t, "Created prod\n", out)
	_, err = h.run(t, "config", "profiles", "create", "prod")
	require.ErrorContains(t, err, `profile "prod" already exists`)

	out, err = h.run(t, "config", "profiles", "list")
	require.NoError(t, err)
	assert.Equal(t, "* dev\n  prod\n", out)

	// Each profile has its own context.
	_, err = h.run(t, "petstore", "context", "set", "--store", "s-dev")
	require.NoError(t, err)
	_, err = h.run(t, "petstore", "context", "set")
	require.ErrorContains(t, err, "nothing to set: pass at least one of --store")
	_, err = h.run(t, "petstore", "context", "set", "--store", "stores/s-1")
	require.ErrorContains(t, err, "takes a bare id")

	out, err = h.run(t, "petstore", "context", "show")
	require.NoError(t, err)
	assert.Equal(t, "profile dev, petstore context:\n  store  s-dev\n", out)
	out, err = h.run(t, "--profile", "prod", "petstore", "context", "show")
	require.NoError(t, err)
	assert.Equal(t, "No petstore context in profile prod.\n", out)

	out, err = h.run(t, "config", "profiles", "describe")
	require.NoError(t, err)
	assert.Equal(t, "dev:\n  contexts:\n    petstore:\n      store_id: s-dev\n  endpoint: dev:443\n  insecure: \"true\"\n", out)

	_, err = h.run(t, "petstore", "pets", "create", "--display-name", "Rex")
	require.NoError(t, err)
	assert.Contains(t, h.fake.pets, "stores/s-dev/pets/p-1")

	// The other profile's context never leaks into a call against it:
	// --profile switches the scope defaults with the profile.
	_, err = h.run(t, "--profile", "prod", "petstore", "pets", "list")
	require.ErrorContains(t, err, "missing store_id")
	t.Setenv("PETSTORE_PROFILE", "prod")
	_, err = h.run(t, "petstore", "pets", "list")
	require.ErrorContains(t, err, "missing store_id")
	t.Setenv("PETSTORE_PROFILE", "")
	_, err = h.run(t, "--profile", "staging", "petstore", "pets", "list")
	require.ErrorContains(t, err, `unknown profile "staging": known profiles are dev, prod`)

	// update changes only what it is given; delete refuses the active one.
	_, err = h.run(t, "config", "profiles", "update", "prod", "--endpoint", "prod2:443")
	require.NoError(t, err)
	out, err = h.run(t, "config", "profiles", "describe", "prod")
	require.NoError(t, err)
	assert.Equal(t, "prod:\n  endpoint: prod2:443\n", out)
	_, err = h.run(t, "config", "profiles", "update", "prod")
	require.ErrorContains(t, err, "nothing to update")
	_, err = h.run(t, "config", "profiles", "delete", "dev")
	require.ErrorContains(t, err, `profile "dev" is active`)
	_, err = h.run(t, "config", "profiles", "activate", "prod")
	require.NoError(t, err)
	_, err = h.run(t, "config", "profiles", "delete", "dev")
	require.NoError(t, err)
	out, err = h.run(t, "config", "profiles", "list")
	require.NoError(t, err)
	assert.Equal(t, "* prod\n", out)

	// clear with a flag clears that level; with none, the whole context.
	_, err = h.run(t, "petstore", "context", "set", "--store", "s-prod")
	require.NoError(t, err)
	_, err = h.run(t, "petstore", "context", "clear", "--store")
	require.NoError(t, err)
	out, err = h.run(t, "petstore", "context", "show")
	require.NoError(t, err)
	assert.Equal(t, "No petstore context in profile prod.\n", out)
}

// TestCLI_ScopeEcho covers the line a write prints when its scope came from
// the context: only on a terminal, only for writes, and not when a flag or a
// full name supplied the scope.
func TestCLI_ScopeEcho(t *testing.T) {

	h := newHarness(t)

	_, err := h.run(t, "config", "profiles", "create", "dev")
	require.NoError(t, err)
	_, err = h.run(t, "petstore", "context", "set", "--store", "s-1")
	require.NoError(t, err)

	// Off a terminal nothing is said.
	_, err = h.run(t, "petstore", "pets", "create", "--display-name", "Rex")
	require.NoError(t, err)
	assert.Empty(t, h.stderr)

	h.terminal = true
	echo := "scope from profile dev: store s-1\n"

	_, err = h.run(t, "petstore", "pets", "create", "--display-name", "Fido")
	require.NoError(t, err)
	assert.Equal(t, echo, h.stderr)

	_, err = h.run(t, "petstore", "pets", "update", "p-1", "--display-name", "Lord Rex")
	require.NoError(t, err)
	assert.Equal(t, echo, h.stderr)

	_, err = h.run(t, "petstore", "toys", "create", "ball")
	require.NoError(t, err)
	assert.Equal(t, echo, h.stderr)

	// Reads stay quiet.
	_, err = h.run(t, "petstore", "pets", "list")
	require.NoError(t, err)
	assert.Empty(t, h.stderr)
	_, err = h.run(t, "petstore", "pets", "describe", "p-1")
	require.NoError(t, err)
	assert.Empty(t, h.stderr)

	// A flag or a full name is the caller's own scope.
	_, err = h.run(t, "petstore", "pets", "update", "p-1", "--store-id", "s-1", "--display-name", "Rex")
	require.NoError(t, err)
	assert.Empty(t, h.stderr)
	_, err = h.run(t, "petstore", "pets", "delete", "stores/s-1/pets/p-2")
	require.NoError(t, err)
	assert.Empty(t, h.stderr)

	_, err = h.run(t, "petstore", "pets", "add-toys", "p-1", "ball")
	require.NoError(t, err)
	assert.Equal(t, echo, h.stderr)
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

	// A target resolves as the pet does: a bare id composes under the same
	// scope, and a full name passes after its own check. The server sees
	// names either way.
	out, err := h.run(t, "petstore", "pets", "add-toys", "p-1", "--store-id", "s-1", "ball", "stores/s-1/toys/rope")
	require.NoError(t, err)
	assert.Equal(t, "Updated stores/s-1/pets/p-1\n", out)
	assert.Equal(t, []string{"stores/s-1/toys/ball", "stores/s-1/toys/rope"}, h.fake.toys["stores/s-1/pets/p-1"])

	out, err = h.run(t, "petstore", "pets", "list-toys", "p-1", "--store-id", "s-1")
	require.NoError(t, err)
	assert.Contains(t, out, "stores/s-1/toys/ball")
	assert.Contains(t, out, "Toy stores/s-1/toys/rope")

	_, err = h.run(t, "petstore", "pets", "remove-toys", "stores/s-1/pets/p-1", "ball")
	require.NoError(t, err)
	assert.Equal(t, []string{"stores/s-1/toys/rope"}, h.fake.toys["stores/s-1/pets/p-1"])

	// A full name in another collection is refused before the call,
	// naming the target, rather than sent for the server to reject.
	_, err = h.run(t, "petstore", "pets", "add-toys", "p-1", "--store-id", "s-1", "stores/s-1/pets/p-2")
	require.ErrorContains(t, err, `target "stores/s-1/pets/p-2"`)
	assert.Equal(t, []string{"stores/s-1/toys/rope"}, h.fake.toys["stores/s-1/pets/p-1"])
}

// TestCLI_CallerNamedCreate covers the caller-named lane: `create` takes the
// id as its positional argument, the API receives it in the entity's name
// field, and a full resource name in that position is refused rather than
// composed into a nonsense name.
func TestCLI_CallerNamedCreate(t *testing.T) {

	h := newHarness(t)

	out, err := h.run(t, "petstore", "toys", "create", "squeaky-bone",
		"--store-id", "s-1", "--display-name", "Squeaky bone")
	require.NoError(t, err)
	assert.Equal(t, "Created stores/s-1/toys/squeaky-bone\n", out)

	server := h.fake.toyEntities["stores/s-1/toys/squeaky-bone"]
	require.NotNil(t, server)
	assert.Equal(t, "Squeaky bone", server.GetDisplayName())

	// The verb's usage line advertises the id.
	out, err = h.run(t, "petstore", "toys", "create", "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "create <id>")

	// A full resource name is not an id.
	_, err = h.run(t, "petstore", "toys", "create", "stores/s-1/toys/other",
		"--store-id", "s-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is a resource name, not an id")

	// The id is required: cobra rejects the verb without it.
	_, err = h.run(t, "petstore", "toys", "create", "--store-id", "s-1")
	require.Error(t, err)
}
