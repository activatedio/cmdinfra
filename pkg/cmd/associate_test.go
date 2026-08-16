package cmd_test

import (
	"context"
	"testing"

	petstorev1 "github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/activatedio/cmdinfra/pkg/cmd"
)

func TestAssociator(t *testing.T) {

	type call struct {
		name        string
		set, remove []string
	}

	type s struct {
		act    func(a *cmd.Associator[*petstorev1.Pet]) ([]cmd.Record, error)
		assert func(t *testing.T, calls []call, records []cmd.Record, err error)
	}

	cases := map[string]s{
		"add sends the targets as set": {
			act: func(a *cmd.Associator[*petstorev1.Pet]) ([]cmd.Record, error) {
				return nil, a.Add(context.Background(), "users/u-1", []string{"roles/r-1", "roles/r-2"})
			},
			assert: func(t *testing.T, calls []call, _ []cmd.Record, err error) {
				require.NoError(t, err)
				require.Len(t, calls, 1)
				assert.Equal(t, call{name: "users/u-1", set: []string{"roles/r-1", "roles/r-2"}}, calls[0])
			},
		},
		"remove sends the targets as remove": {
			act: func(a *cmd.Associator[*petstorev1.Pet]) ([]cmd.Record, error) {
				return nil, a.Remove(context.Background(), "users/u-1", []string{"roles/r-1"})
			},
			assert: func(t *testing.T, calls []call, _ []cmd.Record, err error) {
				require.NoError(t, err)
				require.Len(t, calls, 1)
				assert.Equal(t, call{name: "users/u-1", remove: []string{"roles/r-1"}}, calls[0])
			},
		},
		"list associated walks pagination": {
			act: func(a *cmd.Associator[*petstorev1.Pet]) ([]cmd.Record, error) {
				return a.ListAssociated(context.Background(), "users/u-1")
			},
			assert: func(t *testing.T, _ []call, records []cmd.Record, err error) {
				require.NoError(t, err)
				require.Len(t, records, 3)
				assert.Equal(t, "pets/p-3", records[2]["name"])
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			var calls []call
			pages := map[string][]*petstorev1.Pet{
				"":       {{Name: "pets/p-1"}, {Name: "pets/p-2"}},
				"page-2": {{Name: "pets/p-3"}},
			}

			a := cmd.NewAssociator(cmd.AssociateParams[*petstorev1.Pet]{
				Name: "user pets",
				Client: cmd.AssociateClient[*petstorev1.Pet]{
					Associate: func(_ context.Context, name string, set, remove []string) error {
						calls = append(calls, call{name: name, set: set, remove: remove})
						return nil
					},
					ListBy: func(_ context.Context, _ string, pageToken string) ([]*petstorev1.Pet, string, error) {
						next := ""
						if pageToken == "" {
							next = "page-2"
						}
						return pages[pageToken], next, nil
					},
				},
			})

			records, err := tc.act(a)

			tc.assert(t, calls, records, err)
		})
	}
}

func TestSearcher_WalksPagination(t *testing.T) {

	var seen []*cmd.SearchPredicate

	s := cmd.NewSearcher(cmd.SearcherParams[*petstorev1.Pet]{
		Name: "pet",
		Client: func(_ context.Context, _ string, predicates []*cmd.SearchPredicate, pageToken string) ([]*petstorev1.Pet, string, error) {
			seen = predicates
			if pageToken == "" {
				return []*petstorev1.Pet{{Name: "pets/p-1"}}, "more", nil
			}
			return []*petstorev1.Pet{{Name: "pets/p-2"}}, "", nil
		},
	})

	records, err := s.Search(context.Background(), cmd.SearchParams{
		Predicates: []*cmd.SearchPredicate{{Type: cmd.SearchPredicateTypeEqual, Field: "type", Value: "PET_TYPE_DOG"}},
	})

	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "pets/p-2", records[1]["name"])
	require.Len(t, seen, 1)
	assert.Equal(t, cmd.SearchPredicateTypeEqual, seen[0].Type)
}
