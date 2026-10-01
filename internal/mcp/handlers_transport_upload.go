// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_transport_upload.go puts a released request's files into the
// connected system's transport directory and import queue -- and stops there.
package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/config"
)

// uploadTargetOverrides are parameters that would point an upload somewhere
// other than this server's own system and client. None is accepted: the
// files go to the system the server is connected to, and the request into
// that system's own import buffer.
var uploadTargetOverrides = []string{
	"system", "sid", "target", "target_system", "client", "mandt", "host", "sysnr",
	"port", "url", "destination", "dir", "directory", "dir_trans", "transdir",
}

// transportService is ZADT_VSP's transport domain on this server's own
// connection. Tests replace it.
func (s *Server) transportService(ctx context.Context) (adt.TransportService, error) {
	if s.transportWS != nil {
		return s.transportWS(ctx)
	}
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return nil, fmt.Errorf("the transport domain needs ZADT_VSP with ZCL_VSP_TRANSPORT_SERVICE (vsp install zadt-vsp): %w", err)
	}
	return s.debugWSClient, nil
}

// checkOwnTarget refuses a call that names a target of its own, and a server
// whose .vsp.json entry, named by -s/SAP_SYSTEM, is another system than the
// one it is connected to.
func (s *Server) checkOwnTarget(args map[string]any) error {
	for _, k := range uploadTargetOverrides {
		if _, ok := args[k]; ok {
			return fmt.Errorf("%q is not accepted: this call works on this server's own system and client (%s client %s) and nowhere else; "+
				"for another system, use a server connected to it", k, s.config.BaseURL, orDefaultClient(s.config.Client))
		}
	}
	// A .vsp.json that cannot be read is not "no .vsp.json": the entry that
	// would have said which system this server is may be in it. Refuse.
	cfg, path, err := config.LoadSystems()
	if err != nil {
		return fmt.Errorf("%s cannot be read, so this server's own system cannot be confirmed: %w", orDefault(path, ".vsp.json"), err)
	}
	if cfg != nil {
		if _, _, _, oerr := s.ownSystem(cfg); oerr != nil {
			return oerr
		}
	}
	return nil
}

func orDefaultClient(c string) string {
	if c = strings.TrimSpace(c); c == "" {
		return defaultSAPClient
	}
	return c
}

// handleUploadTransport writes a released request's cofile and data file into
// DIR_TRANS of this server's system and adds the request to that system's
// import buffer. It never imports.
//
//	SAP(action="system", params={"type": "upload_transport",
//	    "cofile_path": "/path/K900123.DEV", "datafile_path": "/path/R900123.DEV"})
//
// It answers as soon as the files are written and the background job that
// adds the request is released: status "pending" and the job's number.
// transport_status tells the outcome.
//
//	SAP(action="system", params={"type": "upload_transport",
//	    "cofile_name": "K900123.DEV", "cofile_base64": "...",
//	    "datafile_name": "R900123.DEV", "datafile_base64": "..."})
//
// Every gate runs before any file is read and before ZADT_VSP is dialled:
// --read-only and --transport-read-only refuse, --enable-transports is
// required, --allowed-transports applies to the request, the server must be
// in expert mode, and no target other than the server's own is accepted.
func (s *Server) handleUploadTransport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	const op = "UploadTransport"
	args := request.GetArguments()

	// Policy first, before the arguments are even looked at.
	if err := s.adtClient.CheckTransportUpload(""); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if s.config.Mode != "expert" {
		return newToolResultError(fmt.Sprintf("%s is available in expert mode only (--mode expert), with --enable-transports; this server runs in %s mode",
			op, orDefault(s.config.Mode, "focused"))), nil
	}
	if err := s.checkOwnTarget(args); err != nil {
		return newToolResultError(err.Error()), nil
	}

	cofilePath, dataPath := getStringParam(args, "cofile_path"), getStringParam(args, "datafile_path")
	cofileName, dataName := getStringParam(args, "cofile_name"), getStringParam(args, "datafile_name")
	cofileB64, dataB64 := getStringParam(args, "cofile_base64"), getStringParam(args, "datafile_base64")
	byPath := cofilePath != "" || dataPath != ""
	byContent := cofileB64 != "" || dataB64 != ""
	switch {
	case byPath && byContent:
		return newToolResultError("give the files either as cofile_path + datafile_path or as cofile_name/cofile_base64 + datafile_name/datafile_base64, not both"), nil
	case byPath:
		cofileName, dataName = filepath.Base(cofilePath), filepath.Base(dataPath)
		if cofilePath == "" || dataPath == "" {
			return newToolResultError("both files are required: cofile_path (K<nr>.<SID>) and datafile_path (R<nr>.<SID>)"), nil
		}
	case byContent:
		if cofileB64 == "" || dataB64 == "" {
			return newToolResultError("both files are required: cofile_base64 and datafile_base64, with cofile_name and datafile_name"), nil
		}
	default:
		return newToolResultError("both files are required: cofile_path + datafile_path (K<nr>.<SID>, R<nr>.<SID>), or their names and base64 contents"), nil
	}

	// The request comes from the names alone; the whitelist applies to it
	// before either file is read.
	req, _, _, err := adt.TransportRequestFromFileNames(cofileName, dataName)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err = s.adtClient.CheckTransportUpload(req); err != nil {
		return newToolResultError(err.Error()), nil
	}

	var files *adt.TransportFiles
	if byPath {
		files, err = adt.ReadTransportFiles(cofilePath, dataPath)
	} else {
		var cofile, data []byte
		if cofile, err = decodeTransportFile(cofileB64, adt.TransportCofileMaxBytes); err != nil {
			return newToolResultError("cofile_base64: " + err.Error()), nil
		}
		if data, err = decodeTransportFile(dataB64, adt.TransportUploadMaxBytes); err != nil {
			return newToolResultError("datafile_base64: " + err.Error()), nil
		}
		files, err = adt.NewTransportFiles(cofileName, cofile, dataName, data)
	}
	if err != nil {
		return newToolResultError(err.Error()), nil
	}

	ws, err := s.transportService(ctx)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	res, err := s.adtClient.UploadTransport(ctx, ws, files)
	if err != nil {
		if res == nil {
			return newToolResultError(err.Error()), nil
		}
		out := newToolResultJSON(map[string]any{"error": err.Error(), "result": res})
		out.IsError = true
		return out, nil
	}
	return newToolResultJSON(res), nil
}

// decodeTransportFile decodes base64 content, refusing more than limit bytes
// before decoding.
func decodeTransportFile(b64 string, limit int) ([]byte, error) {
	if base64.StdEncoding.DecodedLen(len(b64)) > limit+3 {
		return nil, fmt.Errorf("over the %d-byte limit", limit)
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("not base64: %w", err)
	}
	return b, nil
}

// handleTransportBuffer shows this system's import buffer, or one request's
// entries in it. It changes nothing.
//
//	SAP(action="system", params={"type": "transport_buffer"})
//	SAP(action="system", params={"type": "transport_buffer", "transport": "TR-EXAMPLE"})
func (s *Server) handleTransportBuffer(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	req := strings.ToUpper(strings.TrimSpace(transportParam(args)))
	if err := s.adtClient.CheckTransportBufferRead(req, "TransportBuffer"); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err := s.checkOwnTarget(args); err != nil {
		return newToolResultError(err.Error()), nil
	}
	ws, err := s.transportService(ctx)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	res, err := s.adtClient.TransportBuffer(ctx, ws, req)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	return newToolResultJSON(res), nil
}

// handleTransportStatus reports the outcome of an upload's add to the import
// queue: queued, pending, job_failed or unknown, from the job's status and log
// and the buffer file. Read-only.
//
//	SAP(action="system", params={"type": "transport_status", "transport": "TR-EXAMPLE", "job": "12345678"})
func (s *Server) handleTransportStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	req := strings.ToUpper(strings.TrimSpace(transportParam(args)))
	if req == "" {
		return newToolResultError("transport (the request) is required, with job (the number upload_transport reported)"), nil
	}
	if err := s.adtClient.CheckTransportBufferRead(req, "TransportAddStatus"); err != nil {
		return newToolResultError(err.Error()), nil
	}
	if err := s.checkOwnTarget(args); err != nil {
		return newToolResultError(err.Error()), nil
	}
	ws, err := s.transportService(ctx)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	st, err := s.adtClient.TransportAddStatus(ctx, ws, req, getStringParam(args, "job"))
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	return newToolResultJSON(st), nil
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
