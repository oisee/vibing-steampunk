package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"

	hdb "github.com/SAP/go-hdb/driver"
	"github.com/spf13/cobra"

	"github.com/oisee/vibing-steampunk/pkg/hanadump"
)

var hanaCmd = &cobra.Command{
	Use:   "hana",
	Short: "Work with the HANA database behind an AS ABAP directly, without the ABAP stack",
}

var hanaDumpCmd = &cobra.Command{
	Use:          "dump",
	SilenceUsage: true,
	Short:        "Dump ABAP sources and repository rows from HANA to a flat directory",
	Long: `Read REPOSRC and a few repository tables straight from the HANA database of
an AS ABAP, over SQL — no logon to ABAP, so it works on a system whose ABAP
side will not start, or on a backup restored into any HANA.

Each selected source is decompressed and written as plain text under src/,
listed in reposrc.jsonl; the matching rows of TADIR, TRDIR, TMDIR, TFDIR,
SEOCLASS and TLIBG go to tables/; manifest.json records what was read, from
where, and the provenance you state.

What to dump is yours to say: --name takes SQL LIKE patterns ('%', '_'),
repeated as needed, matched against program names and the tables' key columns.
Nothing else filters the selection.

The output directory must be new or empty, and outside any git working tree:
a dump of a repository is private data, not something to commit.

  VSP_HANA_PASSWORD=... vsp hana dump --host h --port 30215 --user SAPA4H \
      --schema SAPA4H --name 'Z%' --name 'LZ%' --name 'SAPLZ%' \
      --provenance "A4H trial, backup of 2026-10-15" --out /private/dumps/a4h-110`,
	RunE: runHanaDump,
}

func init() {
	f := hanaDumpCmd.Flags()
	f.String("host", "", "HANA host")
	f.Int("port", 30015, "SQL port of the database to read (the tenant's own port, or SYSTEMDB's with --database)")
	f.String("user", "", "database user; the schema owner reads the repository tables")
	f.String("password", "", "password (default: $VSP_HANA_PASSWORD)")
	f.String("database", "", "tenant name, when connecting through the SYSTEMDB port")
	f.String("schema", "", "ABAP schema, e.g. SAPA4H or SAPABAP1")
	f.StringArray("name", nil, "LIKE pattern for program and object names (repeatable)")
	f.Bool("inactive", false, "also dump inactive versions")
	f.Bool("catalog", false, "also dump definitions of matching database procedures/functions (AMDP) and the views and tables they read")
	f.String("provenance", "", "where this content comes from and under what rights; written to the manifest")
	f.String("out", "", "output directory (new or empty, outside git)")
	f.Bool("allow-in-repo", false, "permit an output directory inside a git working tree")
	for _, n := range []string{"host", "user", "schema", "name", "provenance", "out"} {
		_ = hanaDumpCmd.MarkFlagRequired(n)
	}
	hanaCmd.AddCommand(hanaDumpCmd)
	rootCmd.AddCommand(hanaCmd)
}

func runHanaDump(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	host, _ := f.GetString("host")
	port, _ := f.GetInt("port")
	user, _ := f.GetString("user")
	password, _ := f.GetString("password")
	database, _ := f.GetString("database")
	schema, _ := f.GetString("schema")
	names, _ := f.GetStringArray("name")
	inactive, _ := f.GetBool("inactive")
	catalog, _ := f.GetBool("catalog")
	provenance, _ := f.GetString("provenance")
	out, _ := f.GetString("out")
	allowInRepo, _ := f.GetBool("allow-in-repo")

	if password == "" {
		password = os.Getenv("VSP_HANA_PASSWORD")
	}
	if password == "" {
		return errors.New("no password: pass --password or set VSP_HANA_PASSWORD")
	}

	dsn := url.URL{Scheme: "hdb", User: url.UserPassword(user, password), Host: net.JoinHostPort(host, strconv.Itoa(port))}
	if database != "" {
		dsn.RawQuery = url.Values{"databaseName": {database}}.Encode()
	}
	db, err := sql.Open("hdb", dsn.String())
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect %s:%d as %s: %w", host, port, user, err)
	}

	m, err := hanadump.Run(ctx, db, hanadump.Options{
		Schema: schema, Names: names, Inactive: inactive, Catalog: catalog, Out: out,
		Provenance:  provenance,
		Connection:  hanadump.Connection{Host: host, Port: port, User: user, Database: database},
		AllowInRepo: allowInRepo,
		BlobInto:    func(dst *[]byte) any { return (*hdbBlob)(dst) },
		TextInto:    func(dst *string) any { return (*hdbText)(dst) },
		Log: func(format string, args ...any) {
			fmt.Fprintf(cmd.ErrOrStderr(), format+"\n", args...)
		},
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "dumped %d sources (%d failed) to %s in %s\n",
		m.Counts["sources"], m.Counts["source_errors"], out, m.Finished.Sub(m.Started).Round(1e8))
	return nil
}

// hdbBlob receives a BLOB from go-hdb, which hands out LOB locators that only
// the driver can read.
type hdbBlob []byte

func (b *hdbBlob) Scan(src any) error {
	if src == nil { // SQL NULL: a program row without source
		*b = nil
		return nil
	}
	return hdb.ScanLobBytes(src, (*[]byte)(b))
}

// hdbText receives a character LOB from go-hdb.
type hdbText string

func (t *hdbText) Scan(src any) error {
	if src == nil {
		*t = ""
		return nil
	}
	return hdb.ScanLobString(src, (*string)(t))
}
