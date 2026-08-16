package cmd

import (
	"context"

	"google.golang.org/protobuf/proto"
)

// AssociateClient is the closure seam over one association edge: the AIP
// Associate{X}To{Y} operation (AssociationRequest{set, remove}) plus the
// List{X}By{Y} read. T is the associated (target) entity type.
type AssociateClient[T proto.Message] struct {
	Associate func(ctx context.Context, name string, set, remove []string) error
	ListBy    func(ctx context.Context, name string, pageToken string) (items []T, nextPageToken string, err error)
}

// AssociateParams configures one association edge.
type AssociateParams[T proto.Message] struct {
	// Name is the service name used in messages and errors, e.g.
	// "user roles".
	Name string
	// Columns is the default output field list for ListAssociated.
	Columns FieldList
	// Client carries the association operations.
	Client AssociateClient[T]
}

// Associator is the generic AssociateService implementation over the AIP
// association RPC family.
type Associator[T proto.Message] struct {
	params AssociateParams[T]
}

// NewAssociator returns the AssociateService for one association edge.
func NewAssociator[T proto.Message](params AssociateParams[T]) *Associator[T] {
	return &Associator[T]{params: params}
}

// Name returns the service name.
func (a *Associator[T]) Name() string {
	return a.params.Name
}

// DefaultFieldList returns the default output columns.
func (a *Associator[T]) DefaultFieldList() FieldList {
	return a.params.Columns
}

// Add associates the targets with the named entity.
func (a *Associator[T]) Add(ctx context.Context, name string, targets []string) error {
	return a.params.Client.Associate(ctx, name, targets, nil)
}

// Remove dissociates the targets from the named entity.
func (a *Associator[T]) Remove(ctx context.Context, name string, targets []string) error {
	return a.params.Client.Associate(ctx, name, nil, targets)
}

// ListAssociated returns the entities associated with the named entity,
// walking pagination to the end.
func (a *Associator[T]) ListAssociated(ctx context.Context, name string) ([]Record, error) {

	var out []Record
	token := ""

	for {
		items, next, err := a.params.Client.ListBy(ctx, name, token)
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
