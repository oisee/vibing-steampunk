package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Request is schema 1: the iterator's closure request (iteration, requested,
// seen) plus what an extract adds. A field this binary does not know is an
// error naming it, never ignored.
type Request struct {
	Schema    int                 `json:"schema"`
	Iteration int                 `json:"iteration,omitempty"`
	Requested map[string][]string `json:"requested,omitempty"`
	Seen      map[string][]string `json:"seen,omitempty"`

	Connection *Connection `json:"connection,omitempty"`

	// Names are SQL LIKE patterns on object and program names.
	Names []string `json:"names,omitempty"`
	// Packages are SQL LIKE patterns on TADIR DEVCLASS.
	Packages []string `json:"packages,omitempty"`
	// Local selects local objects: every $ package and local (LOCAL delivery
	// unit) package, or only those objects whose TADIR AUTHOR is listed.
	Local *Local `json:"local,omitempty"`

	Inactive         bool `json:"inactive,omitempty"`
	Catalog          bool `json:"catalog,omitempty"`
	DDIC             bool `json:"ddic,omitempty"`
	SkipTestIncludes bool `json:"skip_test_includes,omitempty"`

	// Languages (translations) and Tables (customer table contents) are
	// reserved: accepted by the schema, refused until implemented.
	Languages []string   `json:"languages,omitempty"`
	Tables    []TableReq `json:"tables,omitempty"`

	Provenance string `json:"provenance,omitempty"`
}

type Connection struct {
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	Database   string `json:"database,omitempty"`
	User       string `json:"user,omitempty"`
	ABAPSchema string `json:"abap_schema,omitempty"`
	// TLS: Encrypt verifies the server against the system's roots (or
	// CAFile) under TLSServerName (default: Host); TrustServerCertificate
	// encrypts without verifying (a self-signed bed, never production).
	Encrypt                bool   `json:"encrypt,omitempty"`
	TrustServerCertificate bool   `json:"trust_server_certificate,omitempty"`
	CAFile                 string `json:"ca_file,omitempty"`
	TLSServerName          string `json:"tls_server_name,omitempty"`
}

// tlsMode: "verified", "unverified" (encrypted, certificate not checked), or "off".
func (c Connection) tlsMode() string {
	switch {
	case c.TrustServerCertificate:
		return "unverified"
	case c.Encrypt || c.CAFile != "" || c.TLSServerName != "":
		return "verified"
	}
	return "off"
}

type Local struct {
	All     bool     `json:"all,omitempty"`
	Authors []string `json:"authors,omitempty"`
}

type TableReq struct {
	Name    string `json:"name"`
	RowsMax int    `json:"rows_max,omitempty"`
}

func parseRequest(r io.Reader) (*Request, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var req Request
	if err := dec.Decode(&req); err != nil {
		// encoding/json says: json: unknown field "x"
		return nil, fmt.Errorf("request: %w", err)
	}
	if dec.More() {
		return nil, errors.New("request: trailing data after the JSON object")
	}
	if req.Schema != 1 {
		return nil, fmt.Errorf("request: schema %d, this binary reads schema 1", req.Schema)
	}
	if len(req.Languages) > 0 {
		return nil, errors.New("request: languages: not implemented in this version (see --version --json capabilities)")
	}
	if len(req.Tables) > 0 {
		return nil, errors.New("request: tables: not implemented in this version (see --version --json capabilities)")
	}
	if req.Local != nil && req.Local.All && len(req.Local.Authors) > 0 {
		return nil, errors.New("request: local: give all or authors, not both")
	}
	for _, a := range localAuthors(&req) {
		if a == "" || strings.ContainsAny(a, "%_") {
			return nil, fmt.Errorf("request: local.authors: %q is not a user name", a)
		}
	}
	return &req, nil
}

func localAuthors(r *Request) []string {
	if r.Local == nil {
		return nil
	}
	return r.Local.Authors
}

// hasScope says whether the request selects anything.
func (r *Request) hasScope() bool {
	return len(r.Names)+len(r.Packages)+len(r.Requested) > 0 || r.Local != nil && (r.Local.All || len(r.Local.Authors) > 0)
}
