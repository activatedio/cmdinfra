package cmd_test

import (
	"testing"

	petstorev1 "github.com/activatedio/tfinfra/examples/petstore/gen/petstore/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/activatedio/cmdinfra/pkg/cmd"
)

func TestApplyRecord(t *testing.T) {

	type s struct {
		arrange func() cmd.StringsRecord
		assert  func(t *testing.T, pet *petstorev1.Pet, err error)
	}

	cases := map[string]s{
		"scalars, enum, list, map, timestamp": {
			arrange: func() cmd.StringsRecord {
				return cmd.StringsRecord{
					"display_name": "Rex",
					"type":         "PET_TYPE_DOG",
					"age":          "3",
					"vaccinated":   "true",
					"weight":       "12.5",
					"tags":         "loud,friendly",
					"labels":       "team=platform,env=prod",
					"create_time":  "2026-08-16T01:02:03Z",
				}
			},
			assert: func(t *testing.T, pet *petstorev1.Pet, err error) {
				require.NoError(t, err)
				assert.Equal(t, "Rex", pet.GetDisplayName())
				assert.Equal(t, petstorev1.PetType_PET_TYPE_DOG, pet.GetType())
				assert.Equal(t, int32(3), pet.GetAge())
				assert.True(t, pet.GetVaccinated())
				assert.InDelta(t, 12.5, pet.GetWeight(), 0.0001)
				assert.Equal(t, []string{"loud", "friendly"}, pet.GetTags())
				assert.Equal(t, map[string]string{"team": "platform", "env": "prod"}, pet.GetLabels())
				assert.Equal(t, "2026-08-16T01:02:03Z", pet.GetCreateTime().AsTime().Format("2006-01-02T15:04:05Z07:00"))
			},
		},
		"any from protojson with @type": {
			arrange: func() cmd.StringsRecord {
				return cmd.StringsRecord{
					"config": `{"@type": "type.googleapis.com/petstore.v1.CollarConfig", "color": "red"}`,
				}
			},
			assert: func(t *testing.T, pet *petstorev1.Pet, err error) {
				require.NoError(t, err)
				var collar petstorev1.CollarConfig
				require.NoError(t, pet.GetConfig().UnmarshalTo(&collar))
				assert.Equal(t, "red", collar.GetColor())
			},
		},
		"struct from a json object": {
			arrange: func() cmd.StringsRecord {
				return cmd.StringsRecord{"metadata": `{"chip": "abc-123"}`}
			},
			assert: func(t *testing.T, pet *petstorev1.Pet, err error) {
				require.NoError(t, err)
				assert.Equal(t, "abc-123", pet.GetMetadata().GetFields()["chip"].GetStringValue())
			},
		},
		"unknown field errors": {
			arrange: func() cmd.StringsRecord {
				return cmd.StringsRecord{"nope": "x"}
			},
			assert: func(t *testing.T, _ *petstorev1.Pet, err error) {
				require.ErrorContains(t, err, `unknown field "nope"`)
			},
		},
		"bad bool errors with the field name": {
			arrange: func() cmd.StringsRecord {
				return cmd.StringsRecord{"vaccinated": "maybe"}
			},
			assert: func(t *testing.T, _ *petstorev1.Pet, err error) {
				require.ErrorContains(t, err, `field "vaccinated"`)
				require.ErrorContains(t, err, "not a bool")
			},
		},
		"bad enum lists the valid names": {
			arrange: func() cmd.StringsRecord {
				return cmd.StringsRecord{"type": "PET_TYPE_FISH"}
			},
			assert: func(t *testing.T, _ *petstorev1.Pet, err error) {
				require.ErrorContains(t, err, "PET_TYPE_UNSPECIFIED, PET_TYPE_DOG, PET_TYPE_CAT")
			},
		},
		"bad map pair errors": {
			arrange: func() cmd.StringsRecord {
				return cmd.StringsRecord{"labels": "team"}
			},
			assert: func(t *testing.T, _ *petstorev1.Pet, err error) {
				require.ErrorContains(t, err, `map entry "team" is not key=value`)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			rec := tc.arrange()
			pet := &petstorev1.Pet{}

			err := cmd.ApplyRecord(pet, rec)

			tc.assert(t, pet, err)
		})
	}
}

// TestApplyRecord_NestedMessage covers a concrete (non-WKT) message field
// taking protojson.
func TestApplyRecord_NestedMessage(t *testing.T) {

	collar := &petstorev1.CollarConfig{}

	err := cmd.ApplyRecord(collar, cmd.StringsRecord{
		"color":  "blue",
		"size":   "3",
		"buckle": `{"material": "brass"}`,
	})

	require.NoError(t, err)
	assert.Equal(t, "blue", collar.GetColor())
	assert.Equal(t, int32(3), collar.GetSize())
	assert.Equal(t, "brass", collar.GetBuckle().GetMaterial())
}

func TestToRecord(t *testing.T) {

	pet := &petstorev1.Pet{Name: "stores/s-1/pets/p-1", DisplayName: "Rex"}

	got, err := cmd.ToRecord(pet)

	require.NoError(t, err)
	assert.Equal(t, "stores/s-1/pets/p-1", got["name"])
	assert.Equal(t, "Rex", got["display_name"])
	// EmitDefaultValues: zero fields are present, so column selection always
	// has something to select.
	assert.Contains(t, got, "vaccinated")
}

func TestDecodeEntity(t *testing.T) {

	type s struct {
		arrange func() []byte
		assert  func(t *testing.T, pet *petstorev1.Pet, err error)
	}

	cases := map[string]s{
		"yaml document": {
			arrange: func() []byte {
				return []byte("display_name: Rex\nvaccinated: true\ntags:\n  - loud\n")
			},
			assert: func(t *testing.T, pet *petstorev1.Pet, err error) {
				require.NoError(t, err)
				assert.Equal(t, "Rex", pet.GetDisplayName())
				assert.True(t, pet.GetVaccinated())
				assert.Equal(t, []string{"loud"}, pet.GetTags())
			},
		},
		"json document": {
			arrange: func() []byte {
				return []byte(`{"display_name": "Lady", "age": 2}`)
			},
			assert: func(t *testing.T, pet *petstorev1.Pet, err error) {
				require.NoError(t, err)
				assert.Equal(t, "Lady", pet.GetDisplayName())
				assert.Equal(t, int32(2), pet.GetAge())
			},
		},
		"unknown field errors": {
			arrange: func() []byte {
				return []byte(`{"nope": true}`)
			},
			assert: func(t *testing.T, _ *petstorev1.Pet, err error) {
				require.Error(t, err)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {

			data := tc.arrange()

			pet, err := cmd.DecodeEntity[*petstorev1.Pet](data)

			tc.assert(t, pet, err)
		})
	}
}
