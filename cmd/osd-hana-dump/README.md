# osd-hana-dump

Reads ABAP sources and repository rows straight out of the HANA database of an
AS ABAP, without the ABAP server. It is the dumper sidecar of the OSD
extension: one static binary (CGO off), no configuration files, JSON in and
out. The same library backs `vsp hana dump`.

**Read-only by construction.** Every connection is switched to
`SET TRANSACTION READ ONLY` before use, so HANA itself refuses any write, and
every statement passes a gate that lets through a single `SELECT` or
`WITH ... SELECT` and nothing else: no DML, DDL, `CALL`, `GRANT`, second
statement or `FOR UPDATE`. A prepared statement can be queried but never
executed, and only read-only transactions can begin
(`pkg/hanadump/readonly.go`; `TestReadOnlyConnectorNothingButSelectLeaves`
proves nothing else reaches the driver). Creating a read-only database user is
not this program's job.

## Commands

```
osd-hana-dump --version [--json]
osd-hana-dump probe [connection flags]
osd-hana-dump list  [connection flags] [--request f.json|-] [--out dir]
osd-hana-dump dump  [connection flags] --request f.json|- --out dir [--allow-in-repo]
```

Connection flags: `--host`, `--port` (the tenant's SQL port, or SYSTEMDB's with
`--database <tenant>`), `--user` (default `$OSD_HANA_USER`), `--abap-schema`
(default: the one schema holding `REPOSRC`), and the TLS flags below. The same fields can come from the
request's `connection` object; flags win.

**TLS**: `--encrypt` verifies the server certificate against the system's roots
or `--ca-file`, for `--tls-server-name` (default: `--host`; a certificate without
the host as a SAN fails, as it should). `--trust-server-certificate` encrypts
without verifying, for self-signed test systems only. The request's `connection`
takes `encrypt`, `trust_server_certificate`, `ca_file`, `tls_server_name`.
Without TLS the password and the data travel in clear; stderr says so, and
`probe` reports `tls`: `verified`, `unverified` or `off`.

**Password**: `$OSD_HANA_PASSWORD`, or `--password-stdin` (first line of
standard input). There is no password flag: argv is visible to every user of
the machine. `--password-stdin` and `--request -` cannot share stdin.

Every command prints exactly one JSON line on stdout: its result, or
`{"ok":false,"exit":N,"error":"..."}`. Progress and log go to stderr.

### `--version --json`

`{name, version, commit, go, os_arch, schema: 1, read_only: true, capabilities: [...]}`.
A caller pins a version and checks `capabilities` before sending a request
that needs one.

### `probe`

Without reading any repository object: `db` (`tenant` or `system`), and per
ABAP schema `owned_by_login`, `non_select_on_schema` with the privilege kinds
(no object names), `basis_release`, `clients`, `reposrc_rows`, `tadir_rows`,
and `abap_attached` (`yes` when the schema owner has sessions an ABAP work
process opened, `no`, or `unknown` when this login may not see them;
informational). `login_class` is `schema_owner`, `admin` (DATABASE, USER or
ROLE ADMIN) or `other`.

### `list`

TADIR objects in scope, without sources: a summary with counts and the stored
(compressed) size of their active sources per type × package × class, and with
`--out`, `objects.jsonl` (one object per line). An empty request lists the
whole repository.

**Class rule.** An object is `customer` when its name is in the customer name
range, or its package is local (`$...`, or delivery unit `LOCAL`), or its
package's name is in the customer name range. The customer name range is `Z*`,
`Y*`, and `/NS/...` for every namespace this system produces (TRNSPACE role
`P`). Everything else is `sap`. TADIR `SRCSYSTEM` is reported
(`srcsystem_sap`) but does not decide: copies between systems change it.
Objects with TADIR `GENFLAG` set are marked `generated`.

### `dump`

Writes the dump into `--out` (new or empty, and outside any git working tree
unless `--allow-in-repo`): `src/*.abap`, `undecoded/*.bin`, `reposrc.jsonl`,
`tables/<NAME>.jsonl`, `manifest.json`, and `objects.jsonl`, the per-object
report. Every object in scope, and every name a closure asked for, gets one
line:

| field | |
|---|---|
| `type`, `name`, `package`, `author`, `class` | from TADIR |
| `state` | `A`, `I` or `A+I`: the versions written |
| `status` | `written` (sources under `src/`), `rows` (content in `tables/`: MSAG always; TABL, DTEL, DOMA, TTYP, SHLP, VIEW, DDLS, DDLX, SRVD, DCLS with `ddic`), `skipped` |
| `reason` | for `skipped`: `not-found`, `no-source`, `encoding`, `excluded` (needs `ddic`), `unsupported`, `too-large` (reserved) |
| `files` | `path`, `state`, `sha256` of the bytes as written (UTF-8, LF), `bytes`, `error` |

`manifest.json` gains a `sidecar` block: version, commit, the request as
received, counts by status and reason, `errors`, and `unmatched_patterns`
(name and package patterns that selected no object).

## Request (schema 1)

```json
{
  "schema": 1,
  "connection": {"host": "...", "port": 30215, "user": "...", "abap_schema": "..."},
  "names": ["Z%"],
  "packages": ["ZMYAPP%"],
  "local": {"all": true},
  "requested": {"CLAS": ["..."], "TABL": ["..."]},
  "iteration": 1,
  "seen": {},
  "inactive": false,
  "catalog": false,
  "ddic": true,
  "skip_test_includes": false,
  "provenance": "where this content comes from and under what rights"
}
```

`names` and `packages` are SQL LIKE patterns (`%`, `_`, `\` escapes). A name
pattern selects objects: an object it matches brings its includes, and with
`catalog` a class brings its AMDP methods (HANA names them `<CLASS>=><METHOD>`).

`catalog` needs more than SELECT on the ABAP schema: HANA shows a procedure's or
view's definition only to its owner or to a login with the `CATALOG READ`
system privilege (read-only). Without it the definition comes back empty; that
is reported as an error (exit 3), never written as an empty file.
`local` takes `all` or `authors` (TADIR AUTHOR). `requested`/`iteration`/`seen`
are an exact object list, as (type, name) pairs, and an iterator's
dependency-closure request: source kinds bring their own sources (a function
group its main program and its `L<group>` + three-character includes), message
classes and CDS kinds their rows, dictionary kinds are closed over transitively
(the names reached are listed in `manifest.json` `closure.resolved`), and
`ddic` is implied. Nothing outside the list and its closure is selected. No
statement carries more than 500 parameters, so the list may be tens of
thousands of names long.
`languages` and `tables` are reserved and refused until implemented. Any other
field is an error naming it.

## Exit codes

| | |
|---|---|
| 0 | done: everything asked for was written or reported |
| 1 | usage error or bad request |
| 2 | connection or logon failed |
| 3 | partial: the dump is written, `manifest.json` `sidecar.errors` is not empty |
| 4 | refused: `--out` not empty, or inside a git working tree |
| 5 | unsupported system: no ABAP schema, several without `--abap-schema`, unexpected layout |

## Build

```
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=X.Y.Z -X main.commit=$(git rev-parse HEAD)" ./cmd/osd-hana-dump
```
