package cmd

import (
	"fmt"

	gentf "github.com/activatedio/tfinfra/genlib/tf"
)

// ColumnsFor resolves the entry's default output columns for list and
// describe tables. A Columns marker is validated against the entity's
// fields (an unknown name panics); absent one, the default is "name" plus
// "display_name" when the message has it.
func ColumnsFor(e gentf.Entry, cols Columns) []string {

	byName := map[string]bool{}
	for _, f := range normalizedFields(e) {
		byName[f.ProtoName] = true
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

	out := []string{gentf.NameField}
	if byName["display_name"] {
		out = append(out, "display_name")
	}
	return out
}
