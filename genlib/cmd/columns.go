package cmd

import (
	"fmt"

	gentf "github.com/activatedio/tfinfra/genlib/tf"
)

// IDColumn is the virtual column holding a resource's bare id — the last
// segment of its name, derived at render time (pkg/cmd.IDField). It is a
// valid column for any message with a name field.
const IDColumn = "id"

// ColumnsFor resolves the entry's default output columns for list and
// describe tables. A Columns marker is validated against the entity's
// fields (an unknown name panics); IDColumn is accepted wherever the
// message has a name. Absent a marker, the default is the bare id plus
// "display_name" when the message has it — describe renders the id column
// as the full name.
func ColumnsFor(e gentf.Entry, cols Columns) []string {

	byName := map[string]bool{}
	for _, f := range normalizedFields(e) {
		byName[f.ProtoName] = true
	}
	if byName[gentf.NameField] {
		byName[IDColumn] = true
	}

	if len(cols.Default) > 0 {
		out := make([]string, len(cols.Default))
		for i, n := range cols.Default {
			if !byName[n] {
				panic(fmt.Sprintf("%s: Columns.Default references unknown field %q", entityType(e).Name(), n))
			}
			out[i] = n
		}
		return out
	}

	out := []string{IDColumn}
	if byName["display_name"] {
		out = append(out, "display_name")
	}
	return out
}
