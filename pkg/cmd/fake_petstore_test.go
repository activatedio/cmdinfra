package cmd_test

import (
	"context"
	"fmt"
	"net"
	"sort"
	"sync"
	"testing"

	petstorev1 "github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/activatedio/cmdinfra/pkg/cmd"
)

// fakePetStore is the in-process AIP gRPC service the adapter tests run
// against: real wire, real client, only the backend faked. Lists paginate
// with a fixed page size to exercise token walking.
type fakePetStore struct {
	petstorev1.UnimplementedPetStoreServiceServer

	mu       sync.Mutex
	pets     map[string]*petstorev1.Pet
	seq      int
	pageSize int

	lastPatchMask []string
}

func newFakePetStore() *fakePetStore {
	return &fakePetStore{pets: map[string]*petstorev1.Pet{}, pageSize: 2}
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
			return nil, status.Errorf(codes.InvalidArgument, "bad page token %q", in.GetPageToken())
		}
	}

	res := &petstorev1.ListPetsResponse{}
	end := min(start+f.pageSize, len(names))
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
		case "tags":
			existing.Tags = in.GetPet().GetTags()
		default:
			return nil, status.Errorf(codes.InvalidArgument, "unsupported update_mask path %q", path)
		}
	}
	return proto.Clone(existing).(*petstorev1.Pet), nil
}

func (f *fakePetStore) UpdatePet(_ context.Context, in *petstorev1.UpdatePetRequest) (*petstorev1.Pet, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.pets[in.GetName()]; !ok {
		return nil, status.Errorf(codes.NotFound, "pet %q not found", in.GetName())
	}
	p := proto.Clone(in.GetPet()).(*petstorev1.Pet)
	p.Name = in.GetName()
	f.pets[p.GetName()] = p
	return proto.Clone(p).(*petstorev1.Pet), nil
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

// newPetCrud boots the fake service and returns the generic Crud bound to
// a real gRPC client against it.
func newPetCrud(t *testing.T) (*cmd.Crud[*petstorev1.Pet], *fakePetStore) {

	t.Helper()

	fake := newFakePetStore()

	lis, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	s := grpc.NewServer()
	petstorev1.RegisterPetStoreServiceServer(s, fake)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := petstorev1.NewPetStoreServiceClient(conn)

	crud := cmd.NewCrud(cmd.CrudParams[*petstorev1.Pet]{
		Name:    "pet",
		Columns: cmd.FieldList{"name", "display_name"},
		Client: cmd.CrudClient[*petstorev1.Pet]{
			Create: func(ctx context.Context, parent string, entity *petstorev1.Pet) (*petstorev1.Pet, error) {
				return client.CreatePet(ctx, &petstorev1.CreatePetRequest{Parent: parent, Pet: entity})
			},
			Get: func(ctx context.Context, name string) (*petstorev1.Pet, error) {
				return client.GetPet(ctx, &petstorev1.GetPetRequest{Name: name})
			},
			List: func(ctx context.Context, parent, pageToken string) ([]*petstorev1.Pet, string, error) {
				res, err := client.ListPets(ctx, &petstorev1.ListPetsRequest{Parent: parent, PageToken: pageToken})
				if err != nil {
					return nil, "", err
				}
				return res.GetPets(), res.GetNextPageToken(), nil
			},
			Patch: func(ctx context.Context, name string, entity *petstorev1.Pet, updateMask []string) (*petstorev1.Pet, error) {
				return client.PatchPet(ctx, &petstorev1.PatchPetRequest{
					Name:       name,
					Pet:        entity,
					UpdateMask: maskOf(updateMask),
				})
			},
			Update: func(ctx context.Context, name string, entity *petstorev1.Pet) (*petstorev1.Pet, error) {
				return client.UpdatePet(ctx, &petstorev1.UpdatePetRequest{Name: name, Pet: entity})
			},
			Delete: func(ctx context.Context, name string) error {
				_, err := client.DeletePet(ctx, &petstorev1.DeletePetRequest{Name: name})
				return err
			},
		},
	})

	return crud, fake
}
