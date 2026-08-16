package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"sigs.k8s.io/yaml"
)

// Renderer renders records for output.
type Renderer interface {
	RenderList(records []Record, fields FieldList, w io.Writer) error
	RenderSingle(record Record, fields FieldList, w io.Writer) error
}

// RendererParams configures output rendering.
type RendererParams struct {
	// Format selects the renderer: "table" (the default when empty),
	// "yaml", or "json".
	Format string
	// Masked lists field paths whose values never print in tables —
	// sensitive fields render as ********. The machine formats (yaml,
	// json) print everything: they exist for piping, and masking them
	// would corrupt round-trips.
	Masked []string
}

// NewRenderer returns the renderer for the format.
func NewRenderer(params RendererParams) (Renderer, error) {

	switch params.Format {
	case "", "table":
		masked := map[string]bool{}
		for _, m := range params.Masked {
			masked[m] = true
		}
		return &tableRenderer{masked: masked}, nil
	case "yaml":
		return yamlRenderer{}, nil
	case "json":
		return jsonRenderer{}, nil
	default:
		return nil, fmt.Errorf("unknown format %q (table, yaml, json)", params.Format)
	}
}

// maxCellWidth caps table cell values; longer values trim with an
// ellipsis.
const maxCellWidth = 40

const maskedValue = "********"

type tableRenderer struct {
	masked map[string]bool
}

func (t *tableRenderer) RenderSingle(record Record, fields FieldList, w io.Writer) error {
	return t.RenderList([]Record{record}, fields, w)
}

func (t *tableRenderer) RenderList(records []Record, fields FieldList, w io.Writer) error {

	rows := make([][]string, len(records))
	for i, rec := range records {
		row := make([]string, len(fields))
		for j, f := range fields {
			row[j] = t.cell(rec, f)
		}
		rows[i] = row
	}

	widths := columnWidths(fields, rows)

	var b strings.Builder
	writeRow := func(cells []string) {
		for j, c := range cells {
			if j > 0 {
				b.WriteString("  ")
			}
			b.WriteString(c)
			if j < len(cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[j]-len(c)))
			}
		}
		b.WriteString("\n")
	}

	headers := make([]string, len(fields))
	for j, f := range fields {
		headers[j] = strings.ToUpper(f)
	}
	writeRow(headers)
	for _, row := range rows {
		writeRow(row)
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// columnWidths sizes each column to its widest cell, headers included.
func columnWidths(fields FieldList, rows [][]string) []int {

	widths := make([]int, len(fields))
	for j, f := range fields {
		widths[j] = len(f)
		for _, row := range rows {
			if l := len(row[j]); l > widths[j] {
				widths[j] = l
			}
		}
	}
	return widths
}

func (t *tableRenderer) cell(rec Record, field string) string {

	v := pathValue(rec, field)
	if v == nil {
		return ""
	}
	if t.masked[field] {
		return maskedValue
	}

	s := fmt.Sprintf("%v", v)
	if len(s) > maxCellWidth {
		return s[:maxCellWidth-3] + "..."
	}
	return s
}

// pathValue walks a dotted field path ("buckle.material") into the
// record's nested maps.
func pathValue(rec Record, path string) any {

	var v any = map[string]any(rec)
	for _, part := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v, ok = m[part]
		if !ok {
			return nil
		}
	}
	return v
}

type yamlRenderer struct{}

func (yamlRenderer) RenderList(records []Record, _ FieldList, w io.Writer) error {
	return renderYAML(records, w)
}

func (yamlRenderer) RenderSingle(record Record, _ FieldList, w io.Writer) error {
	return renderYAML(record, w)
}

func renderYAML(v any, w io.Writer) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

type jsonRenderer struct{}

func (jsonRenderer) RenderList(records []Record, _ FieldList, w io.Writer) error {
	return renderJSON(records, w)
}

func (jsonRenderer) RenderSingle(record Record, _ FieldList, w io.Writer) error {
	return renderJSON(record, w)
}

func renderJSON(v any, w io.Writer) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}
