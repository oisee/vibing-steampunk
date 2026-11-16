// osd-hana-dump reads ABAP sources and repository rows straight out of the
// HANA database of an AS ABAP, without the ABAP server. It is the dumper
// sidecar of the OSD extension: read-only by construction, one binary, no
// configuration files.
//
//	osd-hana-dump --version [--json]
//	osd-hana-dump probe [connection flags]
//	osd-hana-dump list  [connection flags] [--request f.json|-] [--out dir]
//	osd-hana-dump dump  [connection flags] --request f.json|- --out dir [--allow-in-repo]
//
// The password comes from $OSD_HANA_PASSWORD or, with --password-stdin, from
// the first line of standard input. Never from the command line.
//
// Exit codes: 0 done; 1 usage or bad request; 2 connection or logon failed;
// 3 partial (written, but the manifest lists errors); 4 refused (output
// directory not empty, or inside a git working tree); 5 unsupported system
// (no or several ABAP schemas, unknown layout).
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	hdb "github.com/SAP/go-hdb/driver"

	"github.com/oisee/vibing-steampunk/pkg/hanadump"
)

// Set by -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = ""
)

const (
	exitOK          = 0
	exitUsage       = 1
	exitConnect     = 2
	exitPartial     = 3
	exitRefused     = 4
	exitUnsupported = 5
)

// exitError carries the exit code with the message.
type exitError struct {
	code int
	err  error
	// printed: the command already wrote its JSON line to stdout
	printed bool
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func fail(code int, format string, args ...any) error {
	return &exitError{code: code, err: fmt.Errorf(format, args...)}
}

// failPrinted is fail for a command that has written its own JSON line.
func failPrinted(code int, format string, args ...any) error {
	return &exitError{code: code, err: fmt.Errorf(format, args...), printed: true}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	err := dispatch(args, stdin, stdout, stderr)
	if err == nil {
		return exitOK
	}
	code := exitUsage
	var ee *exitError
	if errors.As(err, &ee) {
		code = ee.code
	}
	fmt.Fprintln(stderr, "osd-hana-dump:", err)
	if ee == nil || !ee.printed {
		writeJSONLine(stdout, map[string]any{"ok": false, "exit": code, "error": err.Error()})
	}
	return code
}

func usage(w io.Writer) {
	fmt.Fprint(w, `usage:
  osd-hana-dump --version [--json]
  osd-hana-dump probe [connection flags]
  osd-hana-dump list  [connection flags] [--request f.json|-] [--out dir]
  osd-hana-dump dump  [connection flags] --request f.json|- --out dir [--allow-in-repo]

connection flags: --host --port --database --user --abap-schema --password-stdin
                  --encrypt --trust-server-certificate --ca-file --tls-server-name
password: $OSD_HANA_PASSWORD, or --password-stdin (first line of stdin)
read-only by construction: only SELECT statements leave this program.
`)
}

func dispatch(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return fail(exitUsage, "no command")
	}
	switch args[0] {
	case "--version", "-version", "version":
		fs := flag.NewFlagSet("version", flag.ContinueOnError)
		fs.SetOutput(stderr)
		asJSON := fs.Bool("json", false, "machine-readable")
		if err := fs.Parse(args[1:]); err != nil {
			return fail(exitUsage, "%v", err)
		}
		return printVersion(stdout, *asJSON)
	case "-h", "--help", "help":
		usage(stdout)
		return nil
	case "probe", "list", "dump":
	default:
		usage(stderr)
		return fail(exitUsage, "unknown command %q", args[0])
	}
	verb := args[0]
	fs := flag.NewFlagSet(verb, flag.ContinueOnError)
	fs.SetOutput(stderr)
	host := fs.String("host", "", "HANA host")
	port := fs.Int("port", 0, "SQL port (tenant's own, or SYSTEMDB's with --database)")
	database := fs.String("database", "", "tenant name, when connecting through SYSTEMDB")
	user := fs.String("user", "", "database user (default $OSD_HANA_USER)")
	abapSchema := fs.String("abap-schema", "", "ABAP schema (default: the one schema holding REPOSRC)")
	pwStdin := fs.Bool("password-stdin", false, "read the password from the first line of stdin")
	encrypt := fs.Bool("encrypt", false, "TLS, verifying the server certificate (system roots, or --ca-file)")
	trustCert := fs.Bool("trust-server-certificate", false, "TLS without verifying the certificate (self-signed test systems only)")
	caFile := fs.String("ca-file", "", "PEM file with the root certificate(s) to verify the server against (implies --encrypt)")
	tlsName := fs.String("tls-server-name", "", "host name to verify the certificate for (default: --host; implies --encrypt)")
	reqPath := fs.String("request", "", "request JSON file, or - for stdin")
	out := fs.String("out", "", "output directory")
	allowInRepo := fs.Bool("allow-in-repo", false, "permit an output directory inside a git working tree")
	if err := fs.Parse(args[1:]); err != nil {
		return fail(exitUsage, "%v", err)
	}
	if fs.NArg() > 0 {
		return fail(exitUsage, "unexpected argument %q", fs.Arg(0))
	}
	if *pwStdin && *reqPath == "-" {
		return fail(exitUsage, "--password-stdin and --request - both want stdin")
	}

	in := bufio.NewReader(stdin)
	password := os.Getenv("OSD_HANA_PASSWORD")
	if *pwStdin {
		line, err := in.ReadString('\n')
		if err != nil && line == "" {
			return fail(exitUsage, "--password-stdin: no password on stdin")
		}
		password = strings.TrimRight(line, "\r\n")
	}

	var req *Request
	if *reqPath != "" {
		var r io.Reader = in
		if *reqPath != "-" {
			f, err := os.Open(*reqPath)
			if err != nil {
				return fail(exitUsage, "%v", err)
			}
			defer f.Close()
			r = f
		}
		var err error
		if req, err = parseRequest(r); err != nil {
			return fail(exitUsage, "%v", err)
		}
	}
	if verb == "dump" && (req == nil || !req.hasScope()) {
		return fail(exitUsage, "dump needs --request selecting names, packages, local objects or a closure")
	}
	if verb == "dump" && *out == "" {
		return fail(exitUsage, "dump needs --out")
	}

	c := Connection{}
	if req != nil && req.Connection != nil {
		c = *req.Connection
	}
	if *host != "" {
		c.Host = *host
	}
	if *port != 0 {
		c.Port = *port
	}
	if *database != "" {
		c.Database = *database
	}
	if *user != "" {
		c.User = *user
	}
	if c.User == "" {
		c.User = os.Getenv("OSD_HANA_USER")
	}
	if *abapSchema != "" {
		c.ABAPSchema = *abapSchema
	}
	c.Encrypt = c.Encrypt || *encrypt
	c.TrustServerCertificate = c.TrustServerCertificate || *trustCert
	if *caFile != "" {
		c.CAFile = *caFile
	}
	if *tlsName != "" {
		c.TLSServerName = *tlsName
	}
	if c.TrustServerCertificate && c.CAFile != "" {
		return fail(exitUsage, "--trust-server-certificate and --ca-file contradict each other")
	}
	if c.Host == "" || c.Port == 0 || c.User == "" {
		return fail(exitUsage, "connection: host, port and user are required (flags, request.connection, or $OSD_HANA_USER)")
	}
	if password == "" {
		return fail(exitUsage, "no password: set $OSD_HANA_PASSWORD or use --password-stdin")
	}

	ctx := context.Background()
	db, err := open(ctx, c, password)
	if err != nil {
		return err
	}
	defer db.Close()

	switch verb {
	case "probe":
		return runProbe(ctx, db, c, stdout)
	case "list":
		schema, err := abapSchemaOf(ctx, db, c.ABAPSchema)
		if err != nil {
			return err
		}
		if req == nil {
			req = &Request{Schema: 1}
		}
		return runList(ctx, db, schema, req, *out, *allowInRepo, stdout)
	default:
		schema, err := abapSchemaOf(ctx, db, c.ABAPSchema)
		if err != nil {
			return err
		}
		return runDump(ctx, db, c, schema, req, *out, *allowInRepo, stdout, stderr)
	}
}

// open connects read-only: the session is switched to READ ONLY and every
// statement passes hanadump.CheckReadOnly before it reaches the driver.
func open(ctx context.Context, c Connection, password string) (*sql.DB, error) {
	dsn := url.URL{Scheme: "hdb", User: url.UserPassword(c.User, password), Host: net.JoinHostPort(c.Host, strconv.Itoa(c.Port))}
	q := url.Values{}
	if c.Database != "" {
		q.Set("databaseName", c.Database)
	}
	switch c.tlsMode() {
	case "unverified":
		q.Set(hdb.DSNTLSInsecureSkipVerify, "true")
	case "verified":
		name := c.TLSServerName
		if name == "" {
			name = c.Host
		}
		q.Set(hdb.DSNTLSServerName, name)
		if c.CAFile != "" {
			q.Set(hdb.DSNTLSRootCAFile, c.CAFile)
		}
	}
	dsn.RawQuery = q.Encode()
	base, err := hdb.NewDSNConnector(dsn.String())
	if err != nil {
		// the DSN carries the password: never echo it
		return nil, fail(exitUsage, "connection settings are not valid")
	}
	db := sql.OpenDB(hanadump.ReadOnlyConnector(base))
	if c.tlsMode() == "off" {
		fmt.Fprintln(os.Stderr, "osd-hana-dump: warning: unencrypted connection, the password and the data travel in clear (--encrypt)")
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fail(exitConnect, "connect %s:%d as %s: %v", c.Host, c.Port, c.User, scrub(err, password))
	}
	return db, nil
}

// scrub keeps a password out of an error text, whatever the driver put in.
func scrub(err error, password string) string {
	s := err.Error()
	if password != "" {
		s = strings.ReplaceAll(s, password, "***")
		s = strings.ReplaceAll(s, url.QueryEscape(password), "***")
		s = strings.ReplaceAll(s, url.PathEscape(password), "***")
	}
	return s
}

// abapSchemaOf returns the given schema, or the one schema with a REPOSRC.
func abapSchemaOf(ctx context.Context, db *sql.DB, given string) (string, error) {
	schemas, err := abapSchemas(ctx, db)
	if err != nil {
		return "", fail(exitUnsupported, "looking for ABAP schemas: %v", err)
	}
	if given != "" {
		for _, s := range schemas {
			if s == given {
				return s, nil
			}
		}
		return "", fail(exitUnsupported, "schema %s holds no REPOSRC readable by this user", given)
	}
	switch len(schemas) {
	case 0:
		return "", fail(exitUnsupported, "no ABAP schema (no readable REPOSRC) in this database")
	case 1:
		return schemas[0], nil
	default:
		return "", fail(exitUnsupported, "%d ABAP schemas (%s): name one with --abap-schema", len(schemas), strings.Join(schemas, ", "))
	}
}

func abapSchemas(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT SCHEMA_NAME FROM SYS.TABLES WHERE TABLE_NAME = 'REPOSRC' ORDER BY SCHEMA_NAME`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func buildCommit() string {
	if commit != "" {
		return commit
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				return s.Value
			}
		}
	}
	return "unknown"
}

// capabilities are what a request may ask of this binary; OSG refuses an
// old pinned binary by checking them.
var capabilities = []string{"probe", "list", "dump", "names", "packages", "local", "closure", "inactive", "catalog", "ddic", "skip_test_includes", "object_report", "tls", "msag"}

func printVersion(w io.Writer, asJSON bool) error {
	if !asJSON {
		fmt.Fprintf(w, "osd-hana-dump %s %s %s %s/%s (read-only)\n", version, buildCommit(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return nil
	}
	writeJSONLine(w, map[string]any{
		"name": "osd-hana-dump", "version": version, "commit": buildCommit(), "go": runtime.Version(),
		"os_arch": runtime.GOOS + "/" + runtime.GOARCH, "schema": 1, "read_only": true,
		"capabilities": capabilities,
	})
	return nil
}

func writeJSONLine(w io.Writer, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintln(w, string(b))
}
