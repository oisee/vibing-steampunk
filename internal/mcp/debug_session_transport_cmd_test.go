package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// A minimal transport-command helper: this test binary re-run with a marker,
// answering 200 with an empty body to every framed request.
const mcpStdioHelperMarker = "vsp-mcp-stdio-helper"

func TestMCPStdioHelperProcess(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-1] != mcpStdioHelperMarker {
		return
	}
	r := bufio.NewReader(os.Stdin)
	for {
		var hdr [4]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			os.Exit(0)
		}
		buf := make([]byte, binary.BigEndian.Uint32(hdr[:]))
		if _, err := io.ReadFull(r, buf); err != nil {
			os.Exit(7)
		}
		if _, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(buf))); err != nil {
			os.Exit(8)
		}
		msg := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"
		binary.BigEndian.PutUint32(hdr[:], uint32(len(msg)))
		os.Stdout.Write(hdr[:])
		os.Stdout.Write([]byte(msg))
	}
}

// The HTTPS debug session owns a helper of its own behind a transport
// command; ending the session must end that helper too.
func TestDebugSessionClosesItsTransportCmd(t *testing.T) {
	argv := []string{os.Args[0], "-test.run=^TestMCPStdioHelperProcess$", "--", mcpStdioHelperMarker}
	s := NewServer(&Config{BaseURL: "https://sidecar.invalid", Client: "001", Mode: "expert", TransportCmd: argv})
	defer func() { _ = s.adtClient.CloseTransport() }()
	ctx := context.Background()

	sess, err := s.debugger(ctx)
	if err != nil {
		t.Fatalf("debugger: %v", err)
	}
	if sess.route != "https" || sess.http == nil {
		t.Fatalf("want the HTTPS route with its transport kept, got route %q", sess.route)
	}
	if _, err := sess.http.Request(ctx, "/sap/bc/adt/vsp/ping", nil); err != nil {
		t.Fatalf("request through the session's helper: %v", err)
	}
	s.closeDebugSession(ctx)
	_, err = sess.http.Request(ctx, "/sap/bc/adt/vsp/ping", nil)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("after the session ended: want its transport closed, got %v", err)
	}
}
