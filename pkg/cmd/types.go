package cmd

import (
	"context"
	"sort"

	"google.golang.org/protobuf/proto"
)

// FieldList is an ordered list of output fields (proto field names).
type FieldList []string

// Record is one entity rendered for output: its protojson form (proto
// field names, defaults emitted) decoded into a generic map.
type Record map[string]any

// StringsRecord is free-form string input for an entity, keyed by proto
// field name — typically collected from command-line flags.
type StringsRecord map[string]string

// Keys returns the record's keys sorted, so derived update masks and
// error messages are deterministic.
func (s StringsRecord) Keys() []string {
	ks := make([]string, 0, len(s))
	for k := range s {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// RetrievalParams is the common read-side input: the output field list and
// the AIP parent.
type RetrievalParams struct {
	Fields FieldList
	Parent string
}

// ListParams is the input to ListService.List.
type ListParams struct {
	RetrievalParams
}

// GetParams is the input to GetService.Get.
type GetParams struct {
	RetrievalParams
	Name string
}

// SearchPredicateType classifies a search predicate.
type SearchPredicateType string

// The search predicate types.
const (
	SearchPredicateTypeQuery     SearchPredicateType = "query"
	SearchPredicateTypeKeywords  SearchPredicateType = "keywords"
	SearchPredicateTypeEqual     SearchPredicateType = "equal"
	SearchPredicateTypeNotEqual  SearchPredicateType = "notEqual"
	SearchPredicateTypeTypeMatch SearchPredicateType = "typeMatch"
)

// SearchPredicate is one search criterion.
type SearchPredicate struct {
	Type  SearchPredicateType
	Field string
	Value string
}

// SearchParams is the input to SearchService.Search.
type SearchParams struct {
	RetrievalParams
	Predicates []*SearchPredicate
}

// CreateParams is the input to CreateService.Create: a record (flag input),
// an entity (--file input), or both — the record applies over the entity.
type CreateParams[E proto.Message] struct {
	Record StringsRecord
	Entity E
	Parent string
}

// PatchParams is the input to UpdateService.Patch: only the record's fields
// change, and they become the update mask.
type PatchParams struct {
	Record StringsRecord
	Name   string
}

// UpdateParams is the input to UpdateService.Update: a full replace from an
// entity (--file input).
type UpdateParams[E proto.Message] struct {
	Entity E
	Name   string
}

// DeleteParams is the input to DeleteService.Delete.
type DeleteParams struct {
	Name string
}

// BaseService names a service for messages and errors.
type BaseService interface {
	Name() string
}

// BaseRetrievalService is the read-side base: the default output columns.
type BaseRetrievalService interface {
	BaseService
	DefaultFieldList() FieldList
}

// GetService reads one entity by full AIP name.
type GetService interface {
	BaseRetrievalService
	Get(ctx context.Context, params GetParams) (Record, error)
}

// ListService lists a parent's entities, walking pagination to the end.
type ListService interface {
	BaseRetrievalService
	List(ctx context.Context, params ListParams) ([]Record, error)
}

// SearchService searches entities by predicates.
type SearchService interface {
	BaseRetrievalService
	Search(ctx context.Context, params SearchParams) ([]Record, error)
}

// CreateService creates an entity, returning the created name.
type CreateService[E proto.Message] interface {
	BaseService
	Zero() E
	Create(ctx context.Context, params CreateParams[E]) (string, error)
}

// UpdateService mutates an entity: Patch changes only the record's fields
// (field-mask patch); Update replaces the entity. Both return the name.
type UpdateService[E proto.Message] interface {
	BaseService
	Zero() E
	Patch(ctx context.Context, params PatchParams) (string, error)
	Update(ctx context.Context, params UpdateParams[E]) (string, error)
}

// DeleteService deletes an entity by full AIP name, returning the name.
type DeleteService interface {
	BaseService
	Delete(ctx context.Context, params DeleteParams) (string, error)
}

// CrudService is the full lifecycle surface of one resource.
type CrudService[E proto.Message] interface {
	GetService
	ListService
	CreateService[E]
	UpdateService[E]
	DeleteService
}

// AssociateService manages one association edge (the AIP
// Associate*/List*By* RPC family) as authoritative add/remove operations.
type AssociateService interface {
	BaseRetrievalService
	Add(ctx context.Context, name string, targets []string) error
	Remove(ctx context.Context, name string, targets []string) error
	ListAssociated(ctx context.Context, name string) ([]Record, error)
}
