package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
)

// Probe is what `probe` prints: enough to say whether an extract can run,
// with this login, against this database. It names no repository object.
type Probe struct {
	OK bool `json:"ok"`
	// DB is "tenant" or "system".
	DB          string        `json:"db"`
	ABAPSchemas []SchemaProbe `json:"abap_schemas"`
	// LoginClass: schema_owner (owns an ABAP schema), admin (DATABASE, USER
	// or ROLE ADMIN), or other.
	LoginClass string `json:"login_class"`
	// TLS: "verified", "unverified" or "off", as connected.
	TLS        string   `json:"tls"`
	AdminPrivs []string `json:"admin_privileges,omitempty"`
	Notes      []string `json:"notes,omitempty"`
}

type SchemaProbe struct {
	Schema string `json:"schema"`
	// OwnedByLogin: the login is the schema owner (and so holds every
	// privilege on it).
	OwnedByLogin bool `json:"owned_by_login"`
	// NonSelect: the login holds a privilege on the schema other than
	// SELECT / SELECT METADATA; NonSelectKinds lists their kinds.
	NonSelect      bool     `json:"non_select_on_schema"`
	NonSelectKinds []string `json:"non_select_kinds,omitempty"`
	Release        string   `json:"basis_release,omitempty"`
	Clients        []string `json:"clients,omitempty"`
	Programs       int      `json:"reposrc_rows"`
	Objects        int      `json:"tadir_rows"`
	// ABAPAttached: "yes" when the schema owner has other open connections
	// (an ABAP server's work processes), "no", or "unknown" when this login
	// may not see them. Informational.
	ABAPAttached string `json:"abap_attached"`
}

func runProbe(ctx context.Context, db *sql.DB, c Connection, w io.Writer) error {
	p := Probe{DB: "tenant", LoginClass: "other", TLS: c.tlsMode()}
	var dbName string
	if err := db.QueryRowContext(ctx, `SELECT DATABASE_NAME FROM SYS.M_DATABASE`).Scan(&dbName); err != nil {
		return fail(exitUnsupported, "M_DATABASE: %v", err)
	}
	if dbName == "SYSTEMDB" {
		p.DB = "system"
	}
	var me string
	if err := db.QueryRowContext(ctx, `SELECT CURRENT_USER FROM DUMMY`).Scan(&me); err != nil {
		return fail(exitUnsupported, "CURRENT_USER: %v", err)
	}
	admin, err := strings1(ctx, db, `SELECT DISTINCT PRIVILEGE FROM SYS.EFFECTIVE_PRIVILEGES WHERE USER_NAME = CURRENT_USER AND OBJECT_TYPE = 'SYSTEMPRIVILEGE' AND PRIVILEGE IN ('DATABASE ADMIN', 'USER ADMIN', 'ROLE ADMIN') ORDER BY 1`)
	if err != nil {
		p.Notes = append(p.Notes, "system privileges not readable")
	}
	if len(admin) > 0 {
		p.LoginClass, p.AdminPrivs = "admin", admin
	}

	schemas, err := abapSchemas(ctx, db)
	if err != nil {
		return fail(exitUnsupported, "looking for ABAP schemas: %v", err)
	}
	for _, s := range schemas {
		if c.ABAPSchema != "" && s != c.ABAPSchema {
			continue
		}
		sp := SchemaProbe{Schema: s, ABAPAttached: "unknown"}
		var owner string
		if err := db.QueryRowContext(ctx, `SELECT SCHEMA_OWNER FROM SYS.SCHEMAS WHERE SCHEMA_NAME = ?`, s).Scan(&owner); err == nil {
			sp.OwnedByLogin = owner == me
		}
		if sp.OwnedByLogin {
			p.LoginClass = "schema_owner"
			sp.NonSelect = true
			sp.NonSelectKinds = []string{"OWNER"}
		} else if kinds, err := strings1(ctx, db, `SELECT DISTINCT PRIVILEGE FROM SYS.EFFECTIVE_PRIVILEGES WHERE USER_NAME = CURRENT_USER AND (SCHEMA_NAME = ? OR OBJECT_TYPE = 'SYSTEMPRIVILEGE' AND PRIVILEGE IN ('DATA ADMIN')) AND PRIVILEGE NOT IN ('SELECT', 'SELECT METADATA') ORDER BY 1`, s); err == nil {
			sp.NonSelect, sp.NonSelectKinds = len(kinds) > 0, kinds
		} else {
			p.Notes = append(p.Notes, s+": privileges not readable")
		}
		q := func(dst any, query string, args ...any) error {
			return db.QueryRowContext(ctx, fmt.Sprintf(query, s), args...).Scan(dst)
		}
		if err := q(&sp.Programs, `SELECT COUNT(*) FROM %q."REPOSRC"`); err != nil {
			p.Notes = append(p.Notes, s+": REPOSRC not readable")
		}
		if err := q(&sp.Objects, `SELECT COUNT(*) FROM %q."TADIR"`); err != nil {
			p.Notes = append(p.Notes, s+": TADIR not readable")
		}
		var rel string
		if err := q(&rel, `SELECT RELEASE || '/' || EXTRELEASE FROM %q."CVERS" WHERE COMPONENT = 'SAP_BASIS'`); err == nil {
			sp.Release = rel
		}
		if cl, err := strings1(ctx, db, fmt.Sprintf(`SELECT MANDT FROM %q."T000" ORDER BY 1`, s)); err == nil {
			sp.Clients = cl
		}
		// Sessions of the schema owner that an ABAP work process opened: the
		// ABAP database interface names its application "ABAP:<SID>". Other
		// clients (this one, hdbsql, tools) do not count. A login that is
		// neither the owner nor an administrator sees only its own.
		if sp.OwnedByLogin || p.LoginClass == "admin" {
			var n int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM SYS.M_CONNECTIONS c WHERE c.USER_NAME = ? AND c.CONNECTION_ID > 0 AND c.CONNECTION_ID <> CURRENT_CONNECTION AND EXISTS (SELECT 1 FROM SYS.M_SESSION_CONTEXT s WHERE s.CONNECTION_ID = c.CONNECTION_ID AND s.KEY = 'APPLICATION' AND s.VALUE LIKE 'ABAP:%')`, owner).Scan(&n); err == nil {
				sp.ABAPAttached = map[bool]string{true: "yes", false: "no"}[n > 0]
			}
		}
		p.ABAPSchemas = append(p.ABAPSchemas, sp)
	}
	p.OK = len(p.ABAPSchemas) > 0
	if p.DB == "system" {
		p.Notes = append(p.Notes, "connected to SYSTEMDB: connect to the tenant (its own port, or --database)")
	}
	writeJSONLine(w, p)
	if !p.OK {
		return failPrinted(exitUnsupported, "no ABAP schema readable by this login")
	}
	return nil
}

// strings1 returns the first column of every row.
func strings1(ctx context.Context, db *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
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
		out = append(out, strings.TrimSpace(s))
	}
	return out, rows.Err()
}
