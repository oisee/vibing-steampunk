package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/datacluster"
)

var clusterCmd = &cobra.Command{
	Use:   "cluster",
	Short: "Read and decode cluster tables (BALDAT, INDX, STXL, ...) — what only IMPORT could read",
	Long: `Read data clusters — the byte streams EXPORT ... TO DATABASE writes into
INDX-like tables — and decode them here, without IMPORT, without RFC, without
anything installed on the system.

A cluster table is an ordinary table: a key, a sequence SRTF2, a byte count
CLUSTR and a RAW column CLUSTD. ADT's data preview reads all of it; what it
does not do is understand CLUSTD. This does: fragments joined per key, the
SAP LZH/LZC compression undone, and every exported object laid out by its own
type descriptor — every field with type, length and decimals, values decoded.
Field names are not in the cluster; the kernel writes types, not DDIC.

  vsp cluster read BALDAT --where "relid = 'AL' AND log_handle = '...'"
  vsp cluster read INDX --where "relid = 'ZV'" --schema
  vsp cluster read INDX --where "relid = 'ZD'" --layout ZDEMO_S_HEADER
  vsp cluster read INDX --where "relid = 'ZD'" --layout "HDR=ZDEMO_S_HEADER,ITEMS=ZDEMO_S_ITEM"
  vsp cluster read STXL --where "tdobject = 'TEXT' AND tdname = 'Z...'"   # SAPscript text
  vsp cluster decode baldat.txt --layout applog     # an SE16H export, offline

--layout names what to lay over the fields. A DDIC structure name is read from
DD03L (includes resolved) and applied to every object, or per object as
OBJECT=STRUCTURE; it is refused when it does not fit, field by field, rather
than guessed. Two layouts are built in: applog (BAL_S_MSG over BALDAT buckets,
printing the messages — 'vsp applog --messages' joins them to the headers) and
stxl (TLINE over STXL, rendered as the text; the default when the table is STXL).`,
}

var clusterReadCmd = &cobra.Command{
	Use:   "read <TABLE>",
	Short: "Read clusters from a table over ADT and decode them",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		where, _ := cmd.Flags().GetString("where")
		top, _ := cmd.Flags().GetInt("top")
		res, err := client.ReadClusterRecords(context.Background(), args[0], where, top)
		if err != nil {
			return err
		}
		if res.Truncated {
			fmt.Fprintf(os.Stderr, "row limit %d reached: the last cluster was dropped rather than shown incomplete; narrow --where or raise --top\n", top)
		}
		return emitClusters(cmd, res.Table.Name, res.Records, client)
	},
}

var clusterDecodeCmd = &cobra.Command{
	Use:   "decode <FILE>",
	Short: "Decode clusters from a table export (SE16, SE16H, SE16N) or a hex dump, offline",
	Long: `Decode a cluster table exported as text, with no system at hand.

The file is a delimited export with a header line — the way SE16H's "download"
writes it — with one fragment per line: the key columns, SRTF2, CLUSTR and
CLUSTD as hex. Fragments are grouped by every column that is not the client,
not one of those three, and not named in --ignore. A file holding only hex is
taken as one whole cluster.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		ignore, _ := cmd.Flags().GetStringSlice("ignore")
		var records []datacluster.Record
		if blob, herr := datacluster.DecodeHex(string(raw)); herr == nil && len(blob) > datacluster.HeaderSize {
			records = []datacluster.Record{{Key: []datacluster.KeyValue{{Column: "FILE", Value: filepath.Base(args[0])}}, Blob: blob, Parts: 1}}
		} else if records, err = datacluster.ReadExport(strings.NewReader(string(raw)), ignore...); err != nil {
			return err
		}
		// A DDIC layout needs a system; the built-in ones do not, so the
		// client is only made when a name asks for it.
		var client *adt.Client
		if layout, _ := cmd.Flags().GetString("layout"); layout != "" {
			spec, err := adt.ParseLayoutSpec(layout)
			if err != nil {
				return err
			}
			if spec.Mode() == "" {
				params, perr := resolveSystemParams(cmd)
				if perr != nil {
					return fmt.Errorf("--layout %s needs a system to read DD03L from: %w", layout, perr)
				}
				if client, err = getClient(params); err != nil {
					return err
				}
			}
		}
		return emitClusters(cmd, strings.ToUpper(strings.TrimSuffix(filepath.Base(args[0]), filepath.Ext(args[0]))), records, client)
	},
}

// clusterOut is the JSON shape of one decoded cluster.
type clusterOut struct {
	Table      string                 `json:"table"`
	Key        []datacluster.KeyValue `json:"key"`
	Fragments  int                    `json:"fragments"`
	Bytes      int                    `json:"bytes"`
	Compressed bool                   `json:"compressed"`
	Algorithm  string                 `json:"algorithm,omitempty"`
	Codepage   string                 `json:"codepage"`
	Objects    []clusterObjectOut     `json:"objects,omitempty"`
	Messages   []adt.AppLogMessage    `json:"messages,omitempty"`
	Lines      []datacluster.TextLine `json:"lines,omitempty"`
	Text       string                 `json:"text,omitempty"`
	Notes      []string               `json:"notes,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

type clusterObjectOut struct {
	Name      string              `json:"name"`
	Kind      string              `json:"kind"`
	Layout    string              `json:"layout,omitempty"`
	RowLength int                 `json:"rowLength"`
	Fields    []datacluster.Field `json:"fields"`
	Rows      [][]any             `json:"rows"`
	Records   []map[string]any    `json:"records,omitempty"`
}

func emitClusters(cmd *cobra.Command, table string, records []datacluster.Record, client *adt.Client) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	schema, _ := cmd.Flags().GetBool("schema")
	layoutArg, _ := cmd.Flags().GetString("layout")
	rawDir, _ := cmd.Flags().GetString("raw-dir")
	spec, err := adt.ParseLayoutSpec(layoutArg)
	if err != nil {
		return err
	}
	if spec.Empty() && strings.EqualFold(table, "STXL") {
		spec.Default = adt.LayoutSTXL
	}
	mode := spec.Mode()
	resolver := adt.NewLayoutResolver(client)
	ctx := context.Background()
	if rawDir != "" {
		if err := os.MkdirAll(rawDir, 0o755); err != nil {
			return err
		}
	}

	var outs []clusterOut
	failed := 0
	for i, rec := range records {
		out := clusterOut{Table: table, Key: rec.Key, Fragments: rec.Parts, Bytes: len(rec.Blob)}
		if rawDir != "" {
			name := filepath.Join(rawDir, fmt.Sprintf("%s-%03d.bin", strings.ToLower(table), i+1))
			if err := os.WriteFile(name, rec.Blob, 0o644); err != nil {
				return err
			}
		}
		c, err := datacluster.Parse(rec.Blob)
		if err != nil {
			out.Error = err.Error()
			failed++
		} else {
			out.Compressed, out.Algorithm, out.Codepage = c.Compressed, c.Algorithm, c.Codepage
			switch mode {
			case adt.LayoutAppLog:
				if out.Messages, err = adt.DecodeAppLogMessages(rec.Blob); err != nil {
					out.Error = err.Error()
					failed++
				}
			case adt.LayoutSTXL:
				if out.Lines, out.Text, err = c.SAPscriptText(); err != nil {
					out.Error = err.Error()
					failed++
				}
			default:
				out.Notes = resolver.Apply(ctx, c, spec)
				for _, obj := range c.Objects {
					o := clusterObjectOut{Name: obj.Name, Kind: obj.Kind.String(), RowLength: obj.RowLength, Fields: obj.Fields, Rows: obj.Rows}
					if o.Rows == nil {
						o.Rows = [][]any{}
					}
					if len(obj.Fields) > 0 && obj.Fields[0].Name != "" {
						o.Layout = spec.For(obj.Name)
						o.Records = obj.Records()
					}
					out.Objects = append(out.Objects, o)
				}
			}
		}
		outs = append(outs, out)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(outs)
	}
	if len(outs) == 0 {
		fmt.Fprintln(os.Stderr, "no clusters match")
		return nil
	}
	for _, out := range outs {
		fmt.Printf("== %s %s  (%d fragments, %d bytes", out.Table, datacluster.Record{Key: out.Key}.KeyString(), out.Fragments, out.Bytes)
		if out.Compressed {
			fmt.Printf(", %s", out.Algorithm)
		}
		if out.Codepage != "" {
			fmt.Printf(", codepage %s", out.Codepage)
		}
		fmt.Println(")")
		if out.Error != "" {
			fmt.Printf("   cannot decode: %s\n", out.Error)
			continue
		}
		switch mode {
		case adt.LayoutAppLog:
			if len(out.Messages) == 0 {
				fmt.Println("   no messages")
			}
			for _, m := range out.Messages {
				printAppLogMessage(m)
			}
			continue
		case adt.LayoutSTXL:
			for _, l := range out.Lines {
				fmt.Printf("   %-3s %s\n", l.Format, l.Line)
			}
			continue
		}
		for _, note := range out.Notes {
			fmt.Printf("   layout: %s\n", note)
		}
		for _, obj := range out.Objects {
			fmt.Printf("-- %s  %s, %d row(s), %d field(s), %d bytes/row", obj.Name, obj.Kind, len(obj.Rows), len(obj.Fields), obj.RowLength)
			if obj.Layout != "" {
				fmt.Printf(", laid out as %s", obj.Layout)
			}
			fmt.Println()
			if schema {
				for _, f := range obj.Fields {
					desc := fmt.Sprintf("%s(%d)", f.Type, f.Length)
					if f.Decimals > 0 {
						desc = fmt.Sprintf("%s(%d,%d)", f.Type, f.Length, f.Decimals)
					}
					fmt.Printf("     %-8s %-24s %s\n", f.Path, f.Name, desc)
				}
			}
			for n, row := range obj.Rows {
				parts := make([]string, len(row))
				for i, v := range row {
					if obj.Layout != "" {
						parts[i] = obj.Fields[i].Name + "=" + fmt.Sprint(v)
					} else {
						parts[i] = fmt.Sprint(v)
					}
				}
				fmt.Printf("   [%d] %s\n", n+1, strings.Join(parts, " | "))
			}
		}
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "\n%d of %d clusters could not be decoded\n", failed, len(outs))
	}
	return nil
}

func init() {
	for _, c := range []*cobra.Command{clusterReadCmd, clusterDecodeCmd} {
		c.Flags().Bool("json", false, "Emit JSON")
		c.Flags().Bool("schema", false, "List every field's type, length and decimals before the rows")
		c.Flags().String("layout", "", "What to lay over the fields: a DDIC structure, OBJECT=STRUCTURE pairs, applog, or stxl")
		c.Flags().String("raw-dir", "", "Also write each joined cluster, still compressed, as a .bin file into this directory")
	}
	clusterReadCmd.Flags().String("where", "", "WHERE clause on the table's own columns, e.g. \"relid = 'AL' AND log_handle = '...'\"")
	clusterReadCmd.Flags().Int("top", 500, "Maximum fragments (database rows) to read")
	clusterDecodeCmd.Flags().StringSlice("ignore", nil, "Export columns that are not part of the cluster key (e.g. AEDAT,USERA)")
	clusterCmd.AddCommand(clusterReadCmd, clusterDecodeCmd)
	rootCmd.AddCommand(clusterCmd)
}
