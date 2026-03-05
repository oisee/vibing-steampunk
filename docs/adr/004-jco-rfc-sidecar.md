# ADR-004: JCo RFC Sidecar for SAP Connectivity

## Status

Accepted

## Context

vsp connects to SAP systems via HTTP/HTTPS using the ADT REST API. This requires SAP's ICM HTTP ports (typically 50000 for HTTP, 44300 for HTTPS) to be open.

In many enterprise environments, these HTTP ports are blocked by firewalls or network segmentation policies. However, RFC ports (33xx) are almost always open because SAP GUI -- the standard tool used by thousands of SAP users daily -- relies on RFC for communication.

Eclipse ADT already solves this problem: it ships with SAP JCo (Java Connector) libraries and can connect to SAP systems via RFC, tunneling ADT REST API requests through the `SADT_REST_RFC_ENDPOINT` function module. This function module is a standard part of SAP NetWeaver and acts as an internal HTTP dispatcher -- it receives an HTTP-like request via RFC and routes it to the ICM internally, returning the response.

We wanted to bring this same capability to vsp without rewriting the Go binary in Java or adding a CGo dependency on JCo's native libraries.

## Decision

We introduce a **Java sidecar process** that acts as an RFC-to-HTTP bridge, alongside a **`Requester` interface** that abstracts the transport layer in the Go codebase.

### Architecture

```
MCP Client → vsp (Go) → [Requester interface]
                              ├── Transport (HTTP mode) → SAP HTTP/HTTPS
                              └── RfcTransport (RFC mode) → Java Sidecar → JCo → RFC → SAP
```

### Key Design Decisions

1. **Requester interface abstraction** (`pkg/adt/http.go`): Both `Transport` (HTTP) and `RfcTransport` (RFC) implement the same `Requester` interface. The ADT client layer is completely unaware of the transport mechanism. This means all 122 tools work identically in both modes with zero tool-level changes.

2. **Sidecar process, not embedded Java**: We run JCo in a separate Java process rather than embedding it via CGo/JNI. This keeps the Go binary dependency-free and the sidecar optional -- you only need Java when using RFC mode.

3. **Automatic lifecycle management** (`pkg/adt/sidecar.go`): `SidecarManager` handles starting, health-checking, and stopping the Java process. The sidecar auto-starts on first request and communicates its port via stdout (`SIDECAR_PORT=<port>`).

4. **JCo library discovery** (`pkg/adt/jco_discovery.go`): An interactive `vsp jco setup` wizard discovers JCo libraries from Eclipse ADT plugin directories, copies them locally, extracts native libraries, and validates Java/JCo architecture compatibility.

5. **Reuse existing Eclipse-MCP proxy code**: The Java sidecar extends the existing `sidecar/jco-proxy/` codebase (originally built for Eclipse-MCP integration) with a new `/rfc-proxy` endpoint, minimizing new Java code.

6. **Two RFC endpoints**:
   - `/rfc-proxy` -- Tunnels ADT REST requests through `SADT_REST_RFC_ENDPOINT` (used by RfcTransport for all standard tools)
   - `/rfc-call` -- Calls function modules directly via JCo (used by the `CallRFC` MCP tool)

7. **Concurrency control**: `RfcTransport` uses a semaphore (default: 5 concurrent requests) to prevent overwhelming the SAP RFC connection pool.

## Consequences

### Positive

- **Works behind firewalls**: Any SAP system reachable by SAP GUI is now reachable by vsp
- **Clean abstraction**: The `Requester` interface allows future transport mechanisms (e.g., SAP Cloud Connector) without touching tool handlers
- **Zero impact on HTTP mode**: RFC support is purely additive; HTTP mode has no new dependencies or behavioral changes
- **Leverages standard SAP infrastructure**: Uses `SADT_REST_RFC_ENDPOINT` which is shipped with every SAP NetWeaver system
- **JCo discovery from Eclipse**: Organizations with Eclipse ADT already have JCo libraries available -- no additional SAP download required

### Negative

- **Java dependency for RFC mode**: Users who need RFC must have Java 11+ installed
- **Platform-specific JCo native libraries**: JCo requires a native library matching the OS and CPU architecture, which can cause setup friction (architecture mismatches, missing libraries)
- **Additional process management**: The sidecar adds process lifecycle complexity (startup, health checks, graceful shutdown, crash recovery)
- **Slight latency overhead**: Each request goes through an extra HTTP hop (vsp → sidecar) and JSON serialization/deserialization
- **JCo licensing**: SAP JCo is proprietary; users must ensure appropriate licensing

### Alternatives Considered

1. **CGo/JNI binding to JCo**: Would eliminate the sidecar process but would make the Go binary platform-dependent and complicate the build process significantly.

2. **Pure Go RFC implementation**: No production-quality open-source Go RFC library exists. Building one would be a massive undertaking for a proprietary protocol.

3. **SAP Cloud Connector**: Would work for BTP scenarios but doesn't help with on-premise-only environments.

4. **SSH tunneling to HTTP ports**: Works but requires SSH access and manual tunnel management, which is fragile and not suitable for automated AI agent workflows.
