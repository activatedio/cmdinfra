package cmd

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"
)

// CrudClient is the closure seam over one resource's AIP operations.
// Generated adapters bind these to the service's Create*/Get*/List*/
// Patch*/Update*/Delete* methods; tests bind fakes. A nil operation means
// the surface does not expose it — calling its verb is an error, never a
// panic.
type CrudClient[E proto.Message] struct {
	Create func(ctx context.Context, parent string, entity E) (E, error)
	Get    func(ctx context.Context, name string) (E, error)
	List   func(ctx context.Context, parent string, pageToken string) (items []E, nextPageToken string, err error)
	Patch  func(ctx context.Context, name string, entity E, updateMask []string) (E, error)
	Update func(ctx context.Context, name string, entity E) (E, error)
	Delete func(ctx context.Context, name string) error
}

// CrudParams configures one resource's Crud service.
type CrudParams[E proto.Message] struct {
	// Name is the service name used in messages and errors, e.g. "realm".
	Name string
	// Columns is the default output field list.
	Columns FieldList
	// Client carries the AIP operations.
	Client CrudClient[E]
}

// Crud is the generic CrudService implementation over AIP client
// operations: one runtime for every resource, replacing per-resource
// hand-written service files.
type Crud[E proto.Message] struct {
	params CrudParams[E]
}

// NewCrud returns the CrudService for one resource.
func NewCrud[E proto.Message](params CrudParams[E]) *Crud[E] {
	return &Crud[E]{params: params}
}

// Name returns the service name.
func (c *Crud[E]) Name() string {
	return c.params.Name
}

// DefaultFieldList returns the default output columns.
func (c *Crud[E]) DefaultFieldList() FieldList {
	return c.params.Columns
}

// Zero returns a fresh empty entity.
func (c *Crud[E]) Zero() E {
	return zero[E]()
}

// Get reads one entity by full AIP name.
func (c *Crud[E]) Get(ctx context.Context, params GetParams) (Record, error) {

	if c.params.Client.Get == nil {
		return nil, c.unsupported("get")
	}

	e, err := c.params.Client.Get(ctx, params.Name)
	if err != nil {
		return nil, err
	}
	return ToRecord(e)
}

// List lists the parent's entities, walking pagination to the end.
func (c *Crud[E]) List(ctx context.Context, params ListParams) ([]Record, error) {

	if c.params.Client.List == nil {
		return nil, c.unsupported("list")
	}

	var out []Record
	token := ""

	for {
		items, next, err := c.params.Client.List(ctx, params.Parent, token)
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

// Create creates an entity under the parent: the record (flag input)
// applies over the entity (--file input, or a fresh one), and the created
// name returns.
func (c *Crud[E]) Create(ctx context.Context, params CreateParams[E]) (string, error) {

	if c.params.Client.Create == nil {
		return "", c.unsupported("create")
	}

	entity := params.Entity
	if !entity.ProtoReflect().IsValid() {
		entity = c.Zero()
	}

	if err := ApplyRecord(entity, params.Record); err != nil {
		return "", err
	}

	created, err := c.params.Client.Create(ctx, params.Parent, entity)
	if err != nil {
		return "", err
	}
	return NameOf(created), nil
}

// Patch updates exactly the record's fields: the record becomes both the
// patch entity and the update mask (its keys, sorted).
func (c *Crud[E]) Patch(ctx context.Context, params PatchParams) (string, error) {

	if c.params.Client.Patch == nil {
		return "", c.unsupported("patch")
	}
	if len(params.Record) == 0 {
		return "", fmt.Errorf("%s: nothing to patch — no fields given", c.params.Name)
	}

	entity := c.Zero()
	if err := ApplyRecord(entity, params.Record); err != nil {
		return "", err
	}

	patched, err := c.params.Client.Patch(ctx, params.Name, entity, params.Record.Keys())
	if err != nil {
		return "", err
	}
	if name := NameOf(patched); name != "" {
		return name, nil
	}
	return params.Name, nil
}

// Update replaces the entity (--file input).
func (c *Crud[E]) Update(ctx context.Context, params UpdateParams[E]) (string, error) {

	if c.params.Client.Update == nil {
		return "", c.unsupported("update")
	}

	updated, err := c.params.Client.Update(ctx, params.Name, params.Entity)
	if err != nil {
		return "", err
	}
	if name := NameOf(updated); name != "" {
		return name, nil
	}
	return params.Name, nil
}

// Delete deletes the entity by full AIP name.
func (c *Crud[E]) Delete(ctx context.Context, params DeleteParams) (string, error) {

	if c.params.Client.Delete == nil {
		return "", c.unsupported("delete")
	}

	if err := c.params.Client.Delete(ctx, params.Name); err != nil {
		return "", err
	}
	return params.Name, nil
}

// GetEntity reads one entity as its proto type — the seam the editor flow
// needs (records lose the typed identity).
func (c *Crud[E]) GetEntity(ctx context.Context, name string) (E, error) {

	var none E
	if c.params.Client.Get == nil {
		return none, c.unsupported("get")
	}
	return c.params.Client.Get(ctx, name)
}

// PatchEntity patches with an explicit entity and mask — the editor flow's
// write side.
func (c *Crud[E]) PatchEntity(ctx context.Context, name string, entity E, mask []string) (string, error) {

	if c.params.Client.Patch == nil {
		return "", c.unsupported("patch")
	}

	patched, err := c.params.Client.Patch(ctx, name, entity, mask)
	if err != nil {
		return "", err
	}
	if key := NameOf(patched); key != "" {
		return key, nil
	}
	return name, nil
}

func (c *Crud[E]) unsupported(op string) error {
	return fmt.Errorf("%s does not support %s", c.params.Name, op)
}
