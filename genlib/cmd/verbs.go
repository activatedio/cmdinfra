package cmd

import (
	gentf "github.com/activatedio/tfinfra/genlib/tf"
)

// Verb is one derived command verb for a resource group, gcloud-shaped.
type Verb struct {
	// Name is the command name: create, delete, describe, edit, list, or
	// update.
	Name string
	// Op is the API operation driving the verb. For update and edit it
	// distinguishes the mechanics: OpPatch means a field-mask patch (update
	// diffs changed flags; edit diffs the $EDITOR result), OpUpdate means a
	// full replace (update takes --file).
	Op gentf.Ops
}

// VerbsFor derives the verb set from the entry's declared operations, in
// canonical order: create, delete, describe, edit, list, update.
//
//	OpCreate → create
//	OpDelete → delete
//	OpGet    → describe
//	OpList   → list
//	OpPatch  → update (changed-flag diff) and edit ($EDITOR patch)
//	OpUpdate → update (--file full replace) and edit
//
// When both OpPatch and OpUpdate are present, Patch wins for update and
// edit, mirroring tfinfra's Patch-first update. edit additionally requires
// OpGet — there is nothing to edit without a read.
func VerbsFor(ops gentf.Ops) []Verb {

	var verbs []Verb

	mutateOp, canMutate := mutateOpFor(ops)

	if ops.Has(gentf.OpCreate) {
		verbs = append(verbs, Verb{Name: "create", Op: gentf.OpCreate})
	}
	if ops.Has(gentf.OpDelete) {
		verbs = append(verbs, Verb{Name: "delete", Op: gentf.OpDelete})
	}
	if ops.Has(gentf.OpGet) {
		verbs = append(verbs, Verb{Name: "describe", Op: gentf.OpGet})
	}
	if canMutate && ops.Has(gentf.OpGet) {
		verbs = append(verbs, Verb{Name: "edit", Op: mutateOp})
	}
	if ops.Has(gentf.OpList) {
		verbs = append(verbs, Verb{Name: "list", Op: gentf.OpList})
	}
	if canMutate {
		verbs = append(verbs, Verb{Name: "update", Op: mutateOp})
	}

	return verbs
}

// mutateOpFor picks the operation behind update/edit: Patch first, full
// replace otherwise.
func mutateOpFor(ops gentf.Ops) (gentf.Ops, bool) {
	if ops.Has(gentf.OpPatch) {
		return gentf.OpPatch, true
	}
	if ops.Has(gentf.OpUpdate) {
		return gentf.OpUpdate, true
	}
	return 0, false
}
