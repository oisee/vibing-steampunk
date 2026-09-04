package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/datacluster"
)

// Cluster tables — BALDAT, INDX, STXL — hold EXPORT data clusters that ADT's
// data preview returns as hex and nothing on the SAP side will decode for a
// remote caller. pkg/datacluster decodes them here, so an agent can read what
// a program stashed in INDX, the SAPscript text behind an STXL key, or the
// messages of an application log, over the same free SQL as any other table.

const (
	noteClusterNoNames = "Field names are not in a cluster: the kernel writes types, not DDIC. Fields are numbered in order; lay the exporting program's structure over them."
	noteClusterCut     = "The row limit was reached; the last cluster was dropped rather than shown incomplete. Narrow where, or raise max_results."
)

type clusterReadResult struct {
	Table     string          `json:"table"`
	Keys      []string        `json:"keyColumns"`
	Records   []clusterRecord `json:"records"`
	Count     int             `json:"count"`
	Fragments int             `json:"fragments"`
	Notes     []string        `json:"notes,omitempty"`
}

type clusterRecord struct {
	Key        []datacluster.KeyValue `json:"key"`
	Fragments  int                    `json:"fragments"`
	Bytes      int                    `json:"bytes"`
	Compressed bool                   `json:"compressed"`
	Algorithm  string                 `json:"algorithm,omitempty"`
	Codepage   string                 `json:"codepage,omitempty"`
	Objects    []clusterObject        `json:"objects,omitempty"`
	Messages   []adt.AppLogMessage    `json:"messages,omitempty"`
	Lines      []datacluster.TextLine `json:"lines,omitempty"`
	Text       string                 `json:"text,omitempty"`
	Notes      []string               `json:"notes,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

type clusterObject struct {
	Name      string              `json:"name"`
	Kind      string              `json:"kind"`
	Layout    string              `json:"layout,omitempty"`
	RowLength int                 `json:"rowLength"`
	RowCount  int                 `json:"rowCount"`
	Fields    []datacluster.Field `json:"fields"`
	Rows      [][]any             `json:"rows,omitempty"`
	Records   []map[string]any    `json:"records,omitempty"`
}

// handleClusterRead answers analyze type=cluster_read: table, where,
// max_results (fragments), layout (a DDIC structure for every object,
// OBJECT=STRUCTURE pairs, "applog" for BALDAT messages, "stxl" for SAPscript
// text — the default on STXL), schema_only (types and counts without rows).
func (s *Server) handleClusterRead(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	table := strings.ToUpper(strings.TrimSpace(firstString(args, "table", "name", "object_name")))
	if table == "" {
		return newToolResultError("cluster_read needs table: the cluster table to read (BALDAT, INDX, STXL, ...)"), nil
	}
	where := firstString(args, "where", "filter")
	spec, err := adt.ParseLayoutSpec(firstString(args, "layout", "structure"))
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	if spec.Empty() && table == "STXL" {
		spec.Default = adt.LayoutSTXL
	}
	mode := spec.Mode()
	resolver := adt.NewLayoutResolver(s.adtClient)
	schemaOnly, _ := getBoolParam(args, "schema_only")
	maxRows := 200
	if n, ok := firstNumber(args, "max_results", "top", "limit"); ok && n > 0 {
		maxRows = int(n)
	}

	res, err := s.adtClient.ReadClusterRecords(ctx, table, where, maxRows)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to read %s: %v", table, err)), nil
	}
	out := clusterReadResult{Table: res.Table.Name, Keys: res.Table.Keys, Fragments: res.Fragments, Records: []clusterRecord{}}
	if res.Truncated {
		out.Notes = append(out.Notes, noteClusterCut)
	}
	if spec.Empty() && !schemaOnly {
		out.Notes = append(out.Notes, noteClusterNoNames)
	}
	for _, rec := range res.Records {
		r := clusterRecord{Key: rec.Key, Fragments: rec.Parts, Bytes: len(rec.Blob)}
		c, err := datacluster.Parse(rec.Blob)
		if err != nil {
			r.Error = err.Error()
			out.Records = append(out.Records, r)
			continue
		}
		r.Compressed, r.Algorithm, r.Codepage = c.Compressed, c.Algorithm, c.Codepage
		switch mode {
		case adt.LayoutAppLog:
			if r.Messages, err = adt.DecodeAppLogMessages(rec.Blob); err != nil {
				r.Error = err.Error()
			} else if len(r.Messages) > 0 {
				if terr := s.adtClient.AppLogTexts(ctx, s.adtClient.Language(), r.Messages); terr != nil {
					out.Notes = appendUnique(out.Notes, "Message texts could not be read from T100: "+terr.Error())
				}
			}
		case adt.LayoutSTXL:
			if r.Lines, r.Text, err = c.SAPscriptText(); err != nil {
				r.Error = err.Error()
			}
		default:
			r.Notes = resolver.Apply(ctx, c, spec)
			for _, obj := range c.Objects {
				o := clusterObject{Name: obj.Name, Kind: obj.Kind.String(), RowLength: obj.RowLength, RowCount: len(obj.Rows), Fields: obj.Fields}
				if len(obj.Fields) > 0 && obj.Fields[0].Name != "" {
					o.Layout = spec.For(obj.Name)
					if !schemaOnly {
						o.Records = obj.Records()
					}
				} else if !schemaOnly {
					o.Rows = obj.Rows
				}
				r.Objects = append(r.Objects, o)
			}
		}
		out.Records = append(out.Records, r)
	}
	out.Count = len(out.Records)
	return newToolResultJSON(out), nil
}

func appendUnique(notes []string, note string) []string {
	for _, n := range notes {
		if n == note {
			return notes
		}
	}
	return append(notes, note)
}
