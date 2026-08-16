package cmd

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// SearchClient is the closure seam over a service's Search operation.
type SearchClient[E proto.Message] func(ctx context.Context, parent string, predicates []*SearchPredicate, pageToken string) (items []E, nextPageToken string, err error)

// SearcherParams configures one resource's Search service.
type SearcherParams[E proto.Message] struct {
	// Name is the service name used in messages and errors.
	Name string
	// Columns is the default output field list.
	Columns FieldList
	// Client carries the Search operation.
	Client SearchClient[E]
}

// Searcher is the generic SearchService implementation, walking pagination
// to the end.
type Searcher[E proto.Message] struct {
	params SearcherParams[E]
}

// NewSearcher returns the SearchService for one resource.
func NewSearcher[E proto.Message](params SearcherParams[E]) *Searcher[E] {
	return &Searcher[E]{params: params}
}

// Name returns the service name.
func (s *Searcher[E]) Name() string {
	return s.params.Name
}

// DefaultFieldList returns the default output columns.
func (s *Searcher[E]) DefaultFieldList() FieldList {
	return s.params.Columns
}

// Search runs the predicates and returns all matching entities.
func (s *Searcher[E]) Search(ctx context.Context, params SearchParams) ([]Record, error) {

	var out []Record
	token := ""

	for {
		items, next, err := s.params.Client(ctx, params.Parent, params.Predicates, token)
		if err != nil {
			return nil, err
		}
		records, err := ToRecordList(items)
		if err != nil {
			return nil, err
		}
		out = append(out, records...)

		if next == "" {
			return out, nil
		}
		token = next
	}
}
