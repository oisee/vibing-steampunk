package adt

import (
	"context"
	"fmt"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/datacluster"
)

// The Function Builder's test data live in EUFUNC, a cluster table keyed by
// function group, module and test data number. Number 999 is the directory:
// TE_DATADIR lists the saved sets, FDESC_COPY is the interface at the time.
// Each set is one cluster with one object per parameter, named %_I<param>
// for an import, %_V<param> for a value that came back, plus TIME1 (the
// runtime in microseconds), V_RC (the return code) and VEXCEPTION.

// FunctionTestSet is one saved test run.
type FunctionTestSet struct {
	Number string `json:"number"`
	Title  string `json:"title,omitempty"`
	Date   string `json:"date,omitempty"`
	Time   string `json:"time,omitempty"`
	// Inputs are the %_I objects, Outputs the %_V ones, by parameter name.
	Inputs  map[string]any `json:"inputs,omitempty"`
	Outputs map[string]any `json:"outputs,omitempty"`
	// Others are the remaining objects — TIME1, V_RC, VEXCEPTION and
	// anything a newer release added — by object name.
	Others    map[string]any `json:"others,omitempty"`
	Runtime   string         `json:"runtime,omitempty"`
	RC        string         `json:"rc,omitempty"`
	Exception string         `json:"exception,omitempty"`
}

// FunctionTestData is everything EUFUNC holds for one module.
type FunctionTestData struct {
	Function  string              `json:"function"`
	Group     string              `json:"group,omitempty"`
	Interface []FunctionTestParam `json:"interface,omitempty"`
	Sets      []FunctionTestSet   `json:"sets"`
	Notes     []string            `json:"notes,omitempty"`
}

// FunctionTestParam is one row of FDESC_COPY: the interface as saved.
type FunctionTestParam struct {
	Name   string `json:"name"`
	DDIC   string `json:"ddic,omitempty"`
	Type   string `json:"type,omitempty"`
	Length string `json:"length,omitempty"`
	Kind   string `json:"kind,omitempty"`
}

// FunctionTestData reads the saved test data of a function module.
func (c *Client) FunctionTestData(ctx context.Context, function string) (*FunctionTestData, error) {
	function = strings.ToUpper(strings.TrimSpace(function))
	recs, err := c.ReadClusterRecords(ctx, "EUFUNC", fmt.Sprintf("relid = 'FL' AND name = '%s'", sqlQuote(function)), 5000)
	if err != nil {
		return nil, err
	}
	out := &FunctionTestData{Function: function, Sets: []FunctionTestSet{}}
	if len(recs.Records) == 0 {
		return out, fmt.Errorf("no test data saved for %s", function)
	}
	titles := map[string]FunctionTestSet{}
	var sets []FunctionTestSet
	for _, rec := range recs.Records {
		number := ""
		for _, kv := range rec.Key {
			switch kv.Column {
			case "NUMMER":
				number = strings.TrimSpace(kv.Value)
			case "GRUPPE":
				out.Group = strings.TrimSpace(kv.Value)
			}
		}
		cl, err := datacluster.Parse(rec.Blob)
		if err != nil {
			out.Notes = append(out.Notes, fmt.Sprintf("set %s: %v", number, err))
			continue
		}
		if number == "999" {
			if dir := cl.Object("TE_DATADIR"); dir != nil {
				for _, row := range dir.Rows {
					if len(row) >= 6 {
						titles[strings.TrimSpace(str(row[0]))] = FunctionTestSet{Date: str(row[3]), Time: str(row[4]), Title: str(row[5])}
					}
				}
			}
			if iface := cl.Object("FDESC_COPY"); iface != nil {
				for _, row := range iface.Rows {
					if len(row) >= 3 {
						p := FunctionTestParam{Name: str(row[0]), DDIC: str(row[1]), Type: str(row[2])}
						if len(row) >= 4 {
							p.Length = strings.TrimSpace(str(row[3]))
						}
						if len(row) >= 7 {
							p.Kind = str(row[6])
						}
						if p.Name != "" && p.Name != "*" {
							out.Interface = append(out.Interface, p)
						}
					}
				}
			}
			continue
		}
		set := FunctionTestSet{Number: number, Inputs: map[string]any{}, Outputs: map[string]any{}, Others: map[string]any{}}
		for _, obj := range cl.Objects {
			var value any
			switch {
			case len(obj.Rows) == 0:
				value = nil
			case obj.Kind == datacluster.Table:
				value = obj.Rows
			case len(obj.Rows[0]) == 1:
				value = obj.Rows[0][0]
			default:
				value = obj.Rows[0]
			}
			switch {
			case strings.HasPrefix(obj.Name, "%_I"):
				set.Inputs[obj.Name[3:]] = value
			case strings.HasPrefix(obj.Name, "%_V"):
				set.Outputs[obj.Name[3:]] = value
			case obj.Name == "TIME1":
				set.Runtime = fmt.Sprint(value) + " µs"
			case obj.Name == "V_RC":
				set.RC = fmt.Sprint(value)
			case obj.Name == "VEXCEPTION":
				set.Exception = strings.TrimSpace(fmt.Sprint(value))
			default:
				set.Others[obj.Name] = value
			}
		}
		sets = append(sets, set)
	}
	for i := range sets {
		if t, ok := titles[strings.TrimLeft(sets[i].Number, "0")]; ok || len(titles) > 0 {
			if !ok {
				t, ok = titles[sets[i].Number]
			}
			if ok {
				sets[i].Title, sets[i].Date, sets[i].Time = t.Title, t.Date, t.Time
			}
		}
	}
	out.Sets = sets
	return out, nil
}

// FunctionTestMarkdown renders the test data for a person.
func FunctionTestMarkdown(d *FunctionTestData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Test data of %s", d.Function)
	if d.Group != "" {
		fmt.Fprintf(&b, " (%s)", d.Group)
	}
	b.WriteString("\n\n")
	if len(d.Interface) > 0 {
		b.WriteString("## Interface as saved\n\n| Parameter | Type | Length | DDIC |\n|---|---|---|---|\n")
		for _, p := range d.Interface {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", p.Name, p.Type, p.Length, p.DDIC)
		}
		b.WriteString("\n")
	}
	for _, s := range d.Sets {
		fmt.Fprintf(&b, "## Set %s", s.Number)
		if s.Title != "" {
			fmt.Fprintf(&b, " — %s", s.Title)
		}
		if s.Date != "" {
			fmt.Fprintf(&b, " (%s %s)", s.Date, s.Time)
		}
		b.WriteString("\n\n")
		writeKV := func(title string, m map[string]any) {
			if len(m) == 0 {
				return
			}
			fmt.Fprintf(&b, "**%s**\n\n| Parameter | Value |\n|---|---|\n", title)
			for _, k := range sortedKeys(m) {
				fmt.Fprintf(&b, "| %s | %s |\n", k, strings.ReplaceAll(fmt.Sprint(m[k]), "|", "\\|"))
			}
			b.WriteString("\n")
		}
		writeKV("Inputs", s.Inputs)
		writeKV("Outputs", s.Outputs)
		writeKV("Other", s.Others)
		var facts []string
		if s.Runtime != "" {
			facts = append(facts, "runtime "+s.Runtime)
		}
		if s.RC != "" {
			facts = append(facts, "rc "+s.RC)
		}
		if s.Exception != "" {
			facts = append(facts, "exception "+s.Exception)
		}
		if len(facts) > 0 {
			b.WriteString(strings.Join(facts, ", ") + "\n\n")
		}
	}
	for _, n := range d.Notes {
		b.WriteString("_" + n + "_\n")
	}
	return b.String()
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
