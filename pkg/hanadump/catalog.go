package hanadump

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CatalogRecord is one line of catalog/objects.jsonl: a database object whose
// definition was written under catalog/.
type CatalogRecord struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Type   string `json:"type"` // PROCEDURE, FUNCTION, VIEW, TABLE
	// Why it is in the dump: "selected" (matched a name pattern) or the name
	// of the object that depends on it.
	Reason string `json:"reason"`
	File   string `json:"file,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Dependency is one line of catalog/dependencies.jsonl, as
// SYS.OBJECT_DEPENDENCIES states it.
type Dependency struct {
	Schema     string `json:"schema"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	BaseSchema string `json:"base_schema"`
	BaseName   string `json:"base_name"`
	BaseType   string `json:"base_type"`
}

type catalogObject struct{ schema, name, typ string }

// dumpCatalog writes the definitions of the database procedures and functions
// whose names match the patterns — AMDP methods are created as
// "<CLASS>=><METHOD>" — and, transitively, of the views and tables they read:
// the material to compare a native AMDP with one deployed elsewhere.
func dumpCatalog(ctx context.Context, db *sql.DB, opts Options, m *Manifest, logf func(string, ...any)) error {
	dir := filepath.Join(opts.Out, "catalog")
	for _, d := range []string{"procedures", "functions", "views", "tables"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return err
		}
	}
	objs, err := os.Create(filepath.Join(dir, "objects.jsonl"))
	if err != nil {
		return err
	}
	defer objs.Close()
	deps, err := os.Create(filepath.Join(dir, "dependencies.jsonl"))
	if err != nil {
		return err
	}
	defer deps.Close()
	ow, dw := bufio.NewWriter(objs), bufio.NewWriter(deps)
	oenc, denc := json.NewEncoder(ow), json.NewEncoder(dw)

	seen := map[catalogObject]bool{}
	var queue []catalogObject
	reason := map[catalogObject]string{}

	for _, kind := range []struct{ view, col, typ string }{
		{"PROCEDURES", "PROCEDURE_NAME", "PROCEDURE"},
		{"FUNCTIONS", "FUNCTION_NAME", "FUNCTION"},
	} {
		where, args := likeAny(kind.col, opts.Names)
		q := fmt.Sprintf(`SELECT %s FROM SYS.%s WHERE SCHEMA_NAME = ? AND %s ORDER BY 1`, kind.col, kind.view, where)
		rows, err := db.QueryContext(ctx, q, append([]any{opts.Schema}, args...)...)
		if err != nil {
			return fmt.Errorf("hanadump: catalog %s: %w", kind.view, err)
		}
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				rows.Close()
				return err
			}
			o := catalogObject{opts.Schema, n, kind.typ}
			seen[o], reason[o] = true, "selected"
			queue = append(queue, o)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}

	// Breadth first over the dependencies: a procedure reads a view, the
	// view reads tables. Only objects that carry a definition are followed.
	for i := 0; i < len(queue); i++ {
		o := queue[i]
		rec := CatalogRecord{Schema: o.schema, Name: o.name, Type: o.typ, Reason: reason[o]}
		text, err := definition(ctx, db, opts, o)
		if err != nil {
			rec.Error = err.Error()
			m.Counts["catalog_errors"]++
		} else {
			rec.File = "catalog/" + strings.ToLower(o.typ) + "s/" + FileName(o.schema+"."+o.name) + ".sql"
			sum := sha256.Sum256([]byte(text))
			rec.SHA256 = hex.EncodeToString(sum[:])
			if err := os.WriteFile(filepath.Join(opts.Out, rec.File), []byte(text), 0o644); err != nil {
				return err
			}
			m.Counts["catalog_"+strings.ToLower(o.typ)+"s"]++
		}
		if err := oenc.Encode(rec); err != nil {
			return err
		}
		if o.typ == "TABLE" {
			continue
		}
		bases, err := dependencies(ctx, db, o)
		if err != nil {
			return err
		}
		for _, d := range bases {
			if err := denc.Encode(d); err != nil {
				return err
			}
			b := catalogObject{d.BaseSchema, d.BaseName, d.BaseType}
			if b.schema != opts.Schema {
				continue // SYS views and the like: listed, not dumped
			}
			switch b.typ {
			case "TABLE", "VIEW", "PROCEDURE", "FUNCTION":
			default:
				continue // synonyms, sequences, types: listed, not followed
			}
			if !seen[b] {
				seen[b], reason[b] = true, o.name
				queue = append(queue, b)
			}
		}
	}
	logf("catalog: %d procedures, %d functions, %d views, %d tables, %d errors",
		m.Counts["catalog_procedures"], m.Counts["catalog_functions"], m.Counts["catalog_views"],
		m.Counts["catalog_tables"], m.Counts["catalog_errors"])
	if err := ow.Flush(); err != nil {
		return err
	}
	return dw.Flush()
}

func dependencies(ctx context.Context, db *sql.DB, o catalogObject) ([]Dependency, error) {
	rows, err := db.QueryContext(ctx, `SELECT BASE_SCHEMA_NAME, BASE_OBJECT_NAME, BASE_OBJECT_TYPE
		FROM SYS.OBJECT_DEPENDENCIES
		WHERE DEPENDENT_SCHEMA_NAME = ? AND DEPENDENT_OBJECT_NAME = ? AND DEPENDENCY_TYPE = 1
		ORDER BY 1, 2`, o.schema, o.name)
	if err != nil {
		return nil, fmt.Errorf("hanadump: dependencies of %s: %w", o.name, err)
	}
	defer rows.Close()
	var out []Dependency
	for rows.Next() {
		d := Dependency{Schema: o.schema, Name: o.name, Type: o.typ}
		if err := rows.Scan(&d.BaseSchema, &d.BaseName, &d.BaseType); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// definition returns an object's DDL as the catalog holds it; for a table,
// which the catalog keeps no text for, a CREATE statement assembled from its
// columns and primary key.
func definition(ctx context.Context, db *sql.DB, opts Options, o catalogObject) (string, error) {
	var view, col string
	switch o.typ {
	case "PROCEDURE":
		view, col = "PROCEDURES", "PROCEDURE_NAME"
	case "FUNCTION":
		view, col = "FUNCTIONS", "FUNCTION_NAME"
	case "VIEW":
		view, col = "VIEWS", "VIEW_NAME"
	case "TABLE":
		return tableDDL(ctx, db, o)
	default:
		return "", fmt.Errorf("no definition for type %s", o.typ)
	}
	var text string
	var dst any = &text
	if opts.TextInto != nil {
		dst = opts.TextInto(&text)
	}
	err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT DEFINITION FROM SYS.%s WHERE SCHEMA_NAME = ? AND %s = ?`, view, col),
		o.schema, o.name).Scan(dst)
	if err != nil {
		return "", err
	}
	if o.typ == "VIEW" {
		// SYS.VIEWS keeps the query only.
		text = fmt.Sprintf("CREATE VIEW %q.%q AS %s", o.schema, o.name, text)
	}
	return strings.TrimRight(text, " \n") + "\n", nil
}

func tableDDL(ctx context.Context, db *sql.DB, o catalogObject) (string, error) {
	rows, err := db.QueryContext(ctx, `SELECT COLUMN_NAME, DATA_TYPE_NAME, LENGTH, SCALE, IS_NULLABLE, DEFAULT_VALUE
		FROM SYS.TABLE_COLUMNS WHERE SCHEMA_NAME = ? AND TABLE_NAME = ? ORDER BY POSITION`, o.schema, o.name)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var (
			name, typ, nullable string
			length, scale       sql.NullInt64
			def                 sql.NullString
		)
		if err := rows.Scan(&name, &typ, &length, &scale, &nullable, &def); err != nil {
			return "", err
		}
		c := fmt.Sprintf("  %q %s", name, sqlType(typ, length, scale))
		if def.Valid {
			c += " DEFAULT '" + strings.ReplaceAll(def.String, "'", "''") + "'"
		}
		if nullable == "FALSE" {
			c += " NOT NULL"
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(cols) == 0 {
		return "", fmt.Errorf("table %s.%s has no columns in the catalog", o.schema, o.name)
	}
	key, err := primaryKey(ctx, db, o)
	if err != nil {
		return "", err
	}
	if len(key) > 0 {
		q := make([]string, len(key))
		for i, k := range key {
			q[i] = fmt.Sprintf("%q", k)
		}
		cols = append(cols, "  PRIMARY KEY ("+strings.Join(q, ", ")+")")
	}
	return fmt.Sprintf("CREATE COLUMN TABLE %q.%q (\n%s\n);\n", o.schema, o.name, strings.Join(cols, ",\n")), nil
}

func primaryKey(ctx context.Context, db *sql.DB, o catalogObject) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT COLUMN_NAME, POSITION FROM SYS.CONSTRAINTS
		WHERE SCHEMA_NAME = ? AND TABLE_NAME = ? AND IS_PRIMARY_KEY = 'TRUE'`, o.schema, o.name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type kc struct {
		name string
		pos  int
	}
	var ks []kc
	for rows.Next() {
		var k kc
		if err := rows.Scan(&k.name, &k.pos); err != nil {
			return nil, err
		}
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i].pos < ks[j].pos })
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = k.name
	}
	return out, rows.Err()
}

// sqlType spells a catalog column type the way CREATE TABLE takes it.
func sqlType(typ string, length, scale sql.NullInt64) string {
	switch typ {
	case "NVARCHAR", "VARCHAR", "NCHAR", "CHAR", "VARBINARY", "BINARY", "ALPHANUM", "SHORTTEXT":
		if length.Valid {
			return fmt.Sprintf("%s(%d)", typ, length.Int64)
		}
	case "DECIMAL":
		if length.Valid && scale.Valid {
			return fmt.Sprintf("DECIMAL(%d,%d)", length.Int64, scale.Int64)
		}
		if length.Valid {
			return fmt.Sprintf("DECIMAL(%d)", length.Int64)
		}
	}
	return typ
}
