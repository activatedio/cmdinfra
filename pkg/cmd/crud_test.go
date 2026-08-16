package cmd_test

import (
	"context"
	"fmt"
	"testing"

	petstorev1 "github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/activatedio/cmdinfra/pkg/cmd"
)

// The generic adapters implement the service interfaces the verb factories
// consume.
var (
	_ cmd.CrudService[*petstorev1.Pet] = (*cmd.Crud[*petstorev1.Pet])(nil)
	_ cmd.SearchService                = (*cmd.Searcher[*petstorev1.Pet])(nil)
	_ cmd.AssociateService             = (*cmd.Associator[*petstorev1.Pet])(nil)
)

func maskOf(paths []string) *fieldmaskpb.FieldMask {
	return &fieldmaskpb.FieldMask{Paths: paths}
}

// TestCrud_Lifecycle drives the full lifecycle through the real gRPC wire
// against the in-process fake: record-based create, get, record-based
// patch (only the record's fields change), file-based update (full
// replace), delete.
func TestCrud_Lifecycle(t *testing.T) {

	crud, fake := newPetCrud(t)
	ctx := context.Background()

	created, err := crud.Create(ctx, cmd.CreateParams[*petstorev1.Pet]{
		Parent: "stores/s-1",
		Record: cmd.StringsRecord{
			"display_name": "Rex",
			"type":         "PET_TYPE_DOG",
			"tags":         "loud,friendly",
			"labels":       "team=platform",
			"config":       `{"@type": "type.googleapis.com/petstore.v1.CollarConfig", "color": "red"}`,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "stores/s-1/pets/p-1", created)

	got, err := crud.Get(ctx, cmd.GetParams{Name: created})
	require.NoError(t, err)
	assert.Equal(t, "Rex", got["display_name"])
	assert.Equal(t, "PET_TYPE_DOG", got["type"])

	patched, err := crud.Patch(ctx, cmd.PatchParams{
		Name:   created,
		Record: cmd.StringsRecord{"display_name": "Lord Rex"},
	})
	require.NoError(t, err)
	assert.Equal(t, created, patched)
	assert.Equal(t, []string{"display_name"}, fake.lastPatchMask)

	server := fake.pets[created]
	assert.Equal(t, "Lord Rex", server.GetDisplayName())
	// Untouched fields survive a patch.
	assert.Equal(t, petstorev1.PetType_PET_TYPE_DOG, server.GetType())
	assert.Equal(t, map[string]string{"team": "platform"}, server.GetLabels())

	entity, err := cmd.DecodeEntity[*petstorev1.Pet]([]byte("display_name: Replaced\nvaccinated: true\n"))
	require.NoError(t, err)
	updated, err := crud.Update(ctx, cmd.UpdateParams[*petstorev1.Pet]{Name: created, Entity: entity})
	require.NoError(t, err)
	assert.Equal(t, created, updated)
	// Full replace: fields absent from the document reset.
	assert.Equal(t, "Replaced", fake.pets[created].GetDisplayName())
	assert.Empty(t, fake.pets[created].GetLabels())

	deleted, err := crud.Delete(ctx, cmd.DeleteParams{Name: created})
	require.NoError(t, err)
	assert.Equal(t, created, deleted)

	_, err = crud.Get(ctx, cmd.GetParams{Name: created})
	require.Error(t, err)
}

// TestCrud_ListPagination proves List walks next_page_token to the end
// (the fake pages two at a time).
func TestCrud_ListPagination(t *testing.T) {

	crud, _ := newPetCrud(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := crud.Create(ctx, cmd.CreateParams[*petstorev1.Pet]{
			Parent: "stores/s-1",
			Record: cmd.StringsRecord{"display_name": fmt.Sprintf("Pet %d", i)},
		})
		require.NoError(t, err)
	}

	records, err := crud.List(ctx, cmd.ListParams{RetrievalParams: cmd.RetrievalParams{Parent: "stores/s-1"}})

	require.NoError(t, err)
	require.Len(t, records, 5)
	names := map[any]bool{}
	for _, r := range records {
		names[r["name"]] = true
	}
	assert.Len(t, names, 5)
}

func TestCrud_PatchMask(t *testing.T) {

	type s struct {
		arrange func() cmd.StringsRecord
		assert  func(t *testing.T, fake *fakePetStore, key string, err error)
	}

	cases := map[string]s{
		"mask is the record's keys, sorted": {
			arrange: func() cmd.StringsRecord {
				return cmd.StringsRecord{"tags": "a,b", "display_name": "X", "age": "4"}
			},
			assert: func(t *testing.T, fake *fakePetStore, _ string, err error) {
				require.NoError(t, err)
				assert.Equal(t, []string{"age", "display_name", "tags"}, fake.lastPatchMask)
			},
		},
		"empty record refuses to patch": {
			arrange: func() cmd.StringsRecord { return cmd.StringsRecord{} },
			assert: func(t *testing.T, _ *fakePetStore, _ string, err error) {
				require.ErrorContains(t, err, "nothing to patch")
			},
		},
		"bad record value surfaces before the wire": {
			arrange: func() cmd.StringsRecord {
				return cmd.StringsRecord{"age": "old"}
			},
			assert: func(t *testing.T, fake *fakePetStore, _ string, err error) {
				require.ErrorContains(t, err, `field "age"`)
				assert.Nil(t, fake.lastPatchMask)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			crud, fake := newPetCrud(t)
			ctx := context.Background()

			created, err := crud.Create(ctx, cmd.CreateParams[*petstorev1.Pet]{
				Parent: "stores/s-1",
				Record: cmd.StringsRecord{"display_name": "Rex"},
			})
			require.NoError(t, err)

			key, err := crud.Patch(ctx, cmd.PatchParams{Name: created, Record: tc.arrange()})

			tc.assert(t, fake, key, err)
		})
	}
}

func TestCrud_UnsupportedOps(t *testing.T) {

	crud := cmd.NewCrud(cmd.CrudParams[*petstorev1.Pet]{Name: "pet"})
	ctx := context.Background()

	_, err := crud.Get(ctx, cmd.GetParams{Name: "x"})
	require.ErrorContains(t, err, "pet does not support get")

	_, err = crud.Create(ctx, cmd.CreateParams[*petstorev1.Pet]{})
	require.ErrorContains(t, err, "pet does not support create")

	_, err = crud.Delete(ctx, cmd.DeleteParams{Name: "x"})
	require.ErrorContains(t, err, "pet does not support delete")
}
