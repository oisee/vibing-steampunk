package adt

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/datacluster"
)

// A report variant is three things: VARID says it exists and who made it,
// VARIT says what it is called, and VARI — a cluster table — holds the
// values under RELID VA, one object per parameter named after it, and the
// selection screen's shape under RELID VB: %_VARI40C, a row per field with
// its kind (P parameter, S select-option), type, length and DDIC reference.
// The labels a person sees are the program's selection texts, the S entries
// of its text pool.

// Variant is one report variant.
type Variant struct {
	Report    string    `json:"report"`
	Name      string    `json:"name"`
	Text      string    `json:"text,omitempty"`
	CreatedBy string    `json:"createdBy,omitempty"`
	Created   time.Time `json:"created,omitempty"`
	ChangedBy string    `json:"changedBy,omitempty"`
	Changed   time.Time `json:"changed,omitempty"`
	Protected bool      `json:"protected,omitempty"`
	// Fields are the selection screen fields with their values, in screen
	// order when the screen description is stored, else by name.
	Fields []VariantField `json:"fields,omitempty"`
}

// VariantField is one parameter or select-option with its value.
type VariantField struct {
	Name string `json:"name"`
	// Label is the selection text, when the program has one.
	Label string `json:"label,omitempty"`
	// Kind is P for a parameter, S for a select-option.
	Kind     string  `json:"kind"`
	Type     string  `json:"type,omitempty"`
	Length   int     `json:"length,omitempty"`
	DDIC     string  `json:"ddic,omitempty"`
	Protect  string  `json:"protect,omitempty"`
	Value    any     `json:"value,omitempty"`
	Ranges   []Range `json:"ranges,omitempty"`
	Screen   int     `json:"screen,omitempty"`
	Position int     `json:"position,omitempty"`
}

// Range is one row of a select-option.
type Range struct {
	Sign   string `json:"sign"`
	Option string `json:"option"`
	Low    string `json:"low"`
	High   string `json:"high,omitempty"`
}

// Variants lists the variants of a report.
func (c *Client) Variants(ctx context.Context, report, lang string) ([]Variant, error) {
	report = strings.ToUpper(strings.TrimSpace(report))
	res, err := c.RunQuery(ctx, fmt.Sprintf("SELECT report, variant, ename, edat, etime, aename, aedat, aetime, protected FROM varid WHERE report = '%s' ORDER BY variant", sqlQuote(report)), 500)
	if err != nil {
		return nil, fmt.Errorf("reading the variants of %s: %w", report, err)
	}
	var out []Variant
	if res == nil {
		return out, nil
	}
	for _, row := range res.Rows {
		v := Variant{Report: report, Name: cell(row, "VARIANT"), CreatedBy: cell(row, "ENAME"), ChangedBy: cell(row, "AENAME"), Protected: cell(row, "PROTECTED") == "X"}
		v.Created = parseSAPStamp(cell(row, "EDAT"), cell(row, "ETIME"))
		v.Changed = parseSAPStamp(cell(row, "AEDAT"), cell(row, "AETIME"))
		out = append(out, v)
	}
	texts, err := c.RunQuery(ctx, fmt.Sprintf("SELECT variant, vtext FROM varit WHERE report = '%s' AND langu = '%s'", sqlQuote(report), sqlQuote(spras(lang))), 500)
	if err == nil && texts != nil {
		byName := map[string]string{}
		for _, row := range texts.Rows {
			byName[cell(row, "VARIANT")] = cell(row, "VTEXT")
		}
		for i := range out {
			out[i].Text = byName[out[i].Name]
		}
	}
	return out, nil
}

// Variant reads one variant with its values and the screen's labels.
func (c *Client) Variant(ctx context.Context, report, name, lang string) (*Variant, error) {
	report, name = strings.ToUpper(strings.TrimSpace(report)), strings.ToUpper(strings.TrimSpace(name))
	all, err := c.Variants(ctx, report, lang)
	if err != nil {
		return nil, err
	}
	var v *Variant
	for i := range all {
		if all[i].Name == name {
			v = &all[i]
		}
	}
	if v == nil {
		return nil, fmt.Errorf("report %s has no variant %s", report, name)
	}
	recs, err := c.ReadClusterRecords(ctx, "VARI", fmt.Sprintf("report = '%s' AND variant = '%s'", sqlQuote(report), sqlQuote(name)), 2000)
	if err != nil {
		return nil, err
	}
	fields := map[string]*VariantField{}
	var order []string
	for _, rec := range recs.Records {
		relid := ""
		for _, kv := range rec.Key {
			if kv.Column == "RELID" {
				relid = kv.Value
			}
		}
		cl, err := datacluster.Parse(rec.Blob)
		if err != nil {
			return nil, fmt.Errorf("variant %s %s (%s): %w", report, name, relid, err)
		}
		switch relid {
		case "VA":
			for _, obj := range cl.Objects {
				f := fields[obj.Name]
				if f == nil {
					f = &VariantField{Name: obj.Name, Kind: "P"}
					fields[obj.Name] = f
					order = append(order, obj.Name)
				}
				if obj.Kind == datacluster.Table {
					f.Kind = "S"
					for _, row := range obj.Rows {
						if len(row) >= 3 {
							r := Range{Sign: str(row[0]), Option: str(row[1]), Low: str(row[2])}
							if len(row) >= 4 {
								r.High = str(row[3])
							}
							f.Ranges = append(f.Ranges, r)
						}
					}
				} else if len(obj.Rows) > 0 && len(obj.Rows[0]) > 0 {
					if len(obj.Rows[0]) == 1 {
						f.Value = obj.Rows[0][0]
					} else {
						f.Value = obj.Rows[0]
					}
				}
			}
		case "VB":
			if meta := cl.Object("%_VARI40C"); meta != nil {
				for i, row := range meta.Rows {
					if len(row) < 12 {
						continue
					}
					n := str(row[0])
					f := fields[n]
					if f == nil {
						f = &VariantField{Name: n}
						fields[n] = f
						order = append(order, n)
					}
					f.Kind = str(row[2])
					if v, ok := row[3].(int64); ok {
						f.Length = int(v)
					}
					f.Type = str(row[4])
					f.DDIC = str(row[5])
					f.Protect = str(row[11])
					f.Position = i + 1
					if v, ok := row[1].(int64); ok {
						f.Screen = int(v / 1000000)
					}
				}
			}
		}
	}
	// Selection texts: "?..." is what SAP stores for "none maintained", and
	// a text beginning with D and a dot says "use the DDIC field label".
	labels := map[string]string{}
	if pool, perr := c.GetTextPoolInLanguage(ctx, report, lang); perr == nil {
		for _, e := range pool {
			if e.ID == "S" {
				labels[strings.ToUpper(strings.TrimSpace(e.Key))] = strings.TrimSpace(e.Text)
			}
		}
	}
	for _, n := range order {
		f := fields[n]
		label := labels[n]
		switch {
		case label == "?...":
			label = ""
		case strings.HasPrefix(label, "D") && strings.Contains(label, "."):
			label = strings.TrimSpace(label[strings.Index(label, ".")+1:])
		}
		if label == "" && f.DDIC != "" {
			label = c.ddicLabel(ctx, f.DDIC, lang)
		}
		f.Label = label
		v.Fields = append(v.Fields, *f)
	}
	sort.SliceStable(v.Fields, func(i, j int) bool {
		if v.Fields[i].Position != v.Fields[j].Position && v.Fields[i].Position > 0 && v.Fields[j].Position > 0 {
			return v.Fields[i].Position < v.Fields[j].Position
		}
		return v.Fields[i].Name < v.Fields[j].Name
	})
	return v, nil
}

// VariantMarkdown renders a variant for a person: one table of fields.
func VariantMarkdown(v *Variant) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s / %s\n\n", v.Report, v.Name)
	if v.Text != "" {
		fmt.Fprintf(&b, "%s\n\n", v.Text)
	}
	if v.CreatedBy != "" {
		fmt.Fprintf(&b, "Created by %s on %s", v.CreatedBy, v.Created.Format("2006-01-02"))
		if v.ChangedBy != "" && !v.Changed.IsZero() {
			fmt.Fprintf(&b, ", changed by %s on %s", v.ChangedBy, v.Changed.Format("2006-01-02"))
		}
		b.WriteString(".\n\n")
	}
	b.WriteString("| Field | Label | Kind | Type | Value |\n|---|---|---|---|---|\n")
	for _, f := range v.Fields {
		value := ""
		switch {
		case f.Kind == "S":
			var parts []string
			for _, r := range f.Ranges {
				p := r.Sign + " " + r.Option + " " + r.Low
				if r.High != "" {
					p += " … " + r.High
				}
				parts = append(parts, strings.TrimSpace(p))
			}
			value = strings.Join(parts, "<br>")
		case f.Value != nil:
			value = fmt.Sprint(f.Value)
		}
		typ := f.Type
		if f.DDIC != "" {
			typ += " " + f.DDIC
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", f.Name, f.Label, f.Kind, strings.TrimSpace(typ), strings.ReplaceAll(value, "|", "\\|"))
	}
	return b.String()
}

// ddicLabel finds the medium field label of a DDIC reference: a data element
// by name, or TABLE-FIELD through DD03L. Empty when there is none.
func (c *Client) ddicLabel(ctx context.Context, ref, lang string) string {
	element := strings.ToUpper(strings.TrimSpace(ref))
	if table, field, ok := strings.Cut(element, "-"); ok {
		res, err := c.RunQuery(ctx, fmt.Sprintf("SELECT rollname FROM dd03l WHERE tabname = '%s' AND fieldname = '%s' AND as4local = 'A'", sqlQuote(table), sqlQuote(field)), 1)
		if err != nil || res == nil || len(res.Rows) == 0 {
			return ""
		}
		element = cell(res.Rows[0], "ROLLNAME")
	}
	if element == "" {
		return ""
	}
	res, err := c.RunQuery(ctx, fmt.Sprintf("SELECT scrtext_m, scrtext_l, ddtext FROM dd04t WHERE rollname = '%s' AND ddlanguage = '%s' AND as4local = 'A'", sqlQuote(element), sqlQuote(spras(lang))), 1)
	if err != nil || res == nil || len(res.Rows) == 0 {
		return ""
	}
	for _, col := range []string{"SCRTEXT_M", "SCRTEXT_L", "DDTEXT"} {
		if t := cell(res.Rows[0], col); t != "" {
			return t
		}
	}
	return ""
}
