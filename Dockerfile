# =============================================================================
# vsp — ABAP Development Tools MCP Server
# Multi-stage build: builder (CGO + gcc) → minimal Alpine runtime
# =============================================================================

# --- Build Stage -------------------------------------------------------------
FROM golang:1.23-alpine AS builder

# CGO is required for go-sqlite3
RUN apk add --no-cache gcc musl-dev

WORKDIR /src

# Cache dependencies separately from source
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .

# Build args for version injection (pass via --build-arg or CI)
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w \
      -X main.Version=${VERSION} \
      -X main.Commit=${COMMIT} \
      -X main.BuildDate=${BUILD_DATE}" \
    -o /vsp ./cmd/vsp

# --- Runtime Stage -----------------------------------------------------------
FROM alpine:3.21

# ca-certificates: needed for HTTPS connections to SAP systems with valid TLS
# libgcc / libstdc++: required by CGO-compiled go-sqlite3
RUN apk add --no-cache ca-certificates libgcc libstdc++

# Run as non-root user
RUN addgroup -S vsp && adduser -S vsp -G vsp

WORKDIR /home/vsp

COPY --from=builder /vsp /usr/local/bin/vsp

USER vsp

# ─── Connection ──────────────────────────────────────────────────────────────
# SAP system URL (required)
ENV SAP_URL=""
# Basic auth
ENV SAP_USER=""
ENV SAP_PASSWORD=""
# SAP client / language
ENV SAP_CLIENT="001"
ENV SAP_LANGUAGE="EN"
# Skip TLS verification (set to "true" for self-signed certs)
ENV SAP_INSECURE="false"

# ─── Tool Mode ───────────────────────────────────────────────────────────────
# focused (81 tools, default) or expert (122 tools)
ENV SAP_MODE="focused"
# Disable tool groups: 5/U=UI5, T=Tests, H=HANA, D=Debug  (e.g. "TH")
ENV SAP_DISABLED_GROUPS=""

# ─── Safety ──────────────────────────────────────────────────────────────────
# Block ALL write operations (read-only access to the SAP system)
ENV SAP_READ_ONLY="false"
# Block arbitrary SQL via RunQuery
ENV SAP_BLOCK_FREE_SQL="false"
# Whitelist operation types: R=Read, S=Search, Q=Query, C=Create, D=Delete,
#   U=Update, A=Activate  (e.g. "RSQ" = read-only equivalent via allowlist)
ENV SAP_ALLOWED_OPS=""
# Blacklist operation types (e.g. "CDUA" blocks create/delete/update/activate)
ENV SAP_DISALLOWED_OPS=""
# Restrict to packages, comma-separated, wildcards OK  (e.g. "Z*,$TMP")
ENV SAP_ALLOWED_PACKAGES=""
# Allow editing objects that need a transport request
ENV SAP_ALLOW_TRANSPORTABLE_EDITS="false"

# ─── MCP Transport ───────────────────────────────────────────────────────────
# stdio  = default, for local MCP client spawning docker run -i
# http-streamable = recommended for containerised / remote deployments
ENV SAP_TRANSPORT="http-streamable"
# Listen on all interfaces so the container port is reachable from the host.
# Override with SAP_HTTP_ADDR=127.0.0.1:8080 to restrict to localhost.
ENV SAP_HTTP_ADDR="0.0.0.0:8080"

# ─── Transport Management ────────────────────────────────────────────────────
# Enable CTS transport tools (disabled by default)
ENV SAP_ENABLE_TRANSPORTS="false"
# Allow only read operations on transports
ENV SAP_TRANSPORT_READ_ONLY="false"
# Restrict to specific transports, comma-separated, wildcards OK
ENV SAP_ALLOWED_TRANSPORTS=""

# ─── Feature Flags ───────────────────────────────────────────────────────────
# Each accepts: auto (default) | on | off
ENV SAP_FEATURE_ABAPGIT="auto"
ENV SAP_FEATURE_RAP="auto"
ENV SAP_FEATURE_AMDP="auto"
ENV SAP_FEATURE_UI5="auto"
ENV SAP_FEATURE_TRANSPORT="auto"
ENV SAP_FEATURE_HANA="auto"

# ─── Debugger ────────────────────────────────────────────────────────────────
# Match SAP GUI terminal ID for shared breakpoints
ENV SAP_TERMINAL_ID=""

# ─── Output ──────────────────────────────────────────────────────────────────
ENV SAP_VERBOSE="false"

# Expose the MCP streamable HTTP port (default transport in this image).
# Map to any host port: docker run -p 8080:8080 ...
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/vsp"]
