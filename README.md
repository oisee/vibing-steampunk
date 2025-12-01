# mcp-abap-adt-go

A Go-native MCP (Model Context Protocol) server for SAP ABAP Development Tools (ADT).

This provides a single-binary distribution of ABAP ADT tools for use with Claude and other MCP-compatible LLMs.

## Features

- **Single Binary**: Zero runtime dependencies, trivial distribution
- **36 ADT Tools**: Complete ABAP development lifecycle support
- **Read Operations**: Retrieve source code for programs, classes, functions, tables
- **CRUD Operations**: Create, update, and delete ABAP objects
- **Development Tools**: Syntax check, activation, unit test execution
- **Code Intelligence**: Find definition, find references, code completion, pretty printer
- **Workflow Tools**: High-level operations that combine multiple steps
- **Cross-Platform**: Builds for Linux, macOS, Windows (amd64, arm64)

## Available Tools

### Read Operations (13 tools)

| Tool | Description |
|------|-------------|
| `GetProgram` | Retrieve ABAP program source code |
| `GetClass` | Retrieve ABAP class source code |
| `GetInterface` | Retrieve ABAP interface source code |
| `GetFunction` | Retrieve function module source code |
| `GetFunctionGroup` | Retrieve function group structure |
| `GetInclude` | Retrieve ABAP include source code |
| `GetTable` | Retrieve ABAP table structure |
| `GetTableContents` | Retrieve data from ABAP table (supports SQL filtering) |
| `GetStructure` | Retrieve ABAP structure definition |
| `GetPackage` | Retrieve package contents |
| `GetTransaction` | Retrieve transaction details |
| `GetTypeInfo` | Retrieve data type information |
| `SearchObject` | Search for ABAP objects |
| `RunQuery` | Execute freestyle SQL query |

### Development Tools (3 tools)

| Tool | Description |
|------|-------------|
| `SyntaxCheck` | Check ABAP source code for syntax errors |
| `Activate` | Activate an ABAP object |
| `RunUnitTests` | Execute ABAP Unit tests |

### CRUD Operations (5 tools)

| Tool | Description |
|------|-------------|
| `LockObject` | Acquire edit lock on an ABAP object |
| `UnlockObject` | Release edit lock |
| `CreateObject` | Create new ABAP object (program, class, interface, include, function group, function module) |
| `UpdateSource` | Write source code to an object |
| `DeleteObject` | Delete an ABAP object |

### Class Include Operations (3 tools)

| Tool | Description |
|------|-------------|
| `GetClassInclude` | Retrieve class include source (definitions, implementations, macros, testclasses) |
| `CreateTestInclude` | Create test classes include for a class |
| `UpdateClassInclude` | Update class include source |

### Workflow Tools (4 tools)

| Tool | Description |
|------|-------------|
| `WriteProgram` | Update existing program with syntax check and activation |
| `WriteClass` | Update existing class with syntax check and activation |
| `CreateAndActivateProgram` | Create new program with source and activate it |
| `CreateClassWithTests` | Create class with unit tests and run them |

### Code Intelligence Tools (7 tools)

| Tool | Description |
|------|-------------|
| `FindDefinition` | Navigate to symbol definition |
| `FindReferences` | Find all references to an object or symbol |
| `CodeCompletion` | Get code completion suggestions |
| `PrettyPrint` | Format ABAP source code |
| `GetPrettyPrinterSettings` | Get formatter settings |
| `SetPrettyPrinterSettings` | Update formatter settings |
| `GetTypeHierarchy` | Get type hierarchy (supertypes/subtypes) |

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/vibingsteamer/mcp-abap-adt-go.git
cd mcp-abap-adt-go

# Build
go build -o mcp-abap-adt-go ./cmd/mcp-abap-adt-go

# Or with make
make build
```

### Pre-built Binaries

Download from the [releases page](https://github.com/vibingsteamer/mcp-abap-adt-go/releases).

## Configuration

The server is configured via environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SAP_URL` | Yes | - | SAP system URL (e.g., `https://host:44300`) |
| `SAP_USER` | Yes | - | SAP username |
| `SAP_PASSWORD` | Yes | - | SAP password |
| `SAP_CLIENT` | No | `001` | SAP client number |
| `SAP_LANGUAGE` | No | `EN` | SAP language |
| `SAP_INSECURE` | No | `false` | Skip TLS certificate verification |

## Usage with Claude Desktop

Add to your Claude Desktop configuration (`~/.config/claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "abap-adt": {
      "command": "/path/to/mcp-abap-adt-go",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "your-username",
        "SAP_PASSWORD": "your-password",
        "SAP_CLIENT": "001",
        "SAP_LANGUAGE": "EN"
      }
    }
  }
}
```

## Usage with Claude Code

Add `.mcp.json` to your project root:

```json
{
  "mcpServers": {
    "abap-adt": {
      "command": "/path/to/mcp-abap-adt-go",
      "env": {
        "SAP_URL": "https://your-sap-host:44300",
        "SAP_USER": "your-username",
        "SAP_PASSWORD": "your-password",
        "SAP_CLIENT": "001",
        "SAP_LANGUAGE": "EN"
      }
    }
  }
}
```

## Development

```bash
# Run unit tests
go test ./...

# Run integration tests (requires SAP system)
SAP_URL=http://host:port SAP_USER=user SAP_PASSWORD=pass go test -tags=integration -v ./pkg/adt/

# Build
go build -o mcp-abap-adt-go ./cmd/mcp-abap-adt-go
```

## Architecture

```
mcp-abap-adt-go/
├── cmd/mcp-abap-adt-go/    # Main entry point
├── pkg/adt/                 # ADT client library
│   ├── client.go           # Main client facade
│   ├── config.go           # Configuration
│   ├── http.go             # HTTP transport with CSRF & session handling
│   ├── sources.go          # Source retrieval operations
│   ├── crud.go             # CRUD operations (create, update, delete, lock)
│   ├── devtools.go         # Development tools (syntax check, activate, unit tests)
│   ├── codeintel.go        # Code intelligence (definition, references, completion)
│   ├── workflows.go        # High-level workflow operations
│   └── xml.go              # XML parsing utilities
├── internal/mcp/           # MCP server implementation
│   └── server.go           # Tool registration and handlers
└── testdata/               # Test fixtures
```

## Comparison with TypeScript MCP

| Aspect | mcp-abap-adt (TS) | mcp-abap-adt-go |
|--------|-------------------|-----------------|
| Tools | 13 | 36 |
| SQL Query | No | Yes |
| Syntax Check | No | Yes |
| Unit Tests | No | Yes |
| Activation | No | Yes |
| CRUD | No | Yes |
| Class Includes | No | Yes |
| Code Intelligence | No | Yes |
| Workflow Tools | No | Yes |
| Distribution | npm + Node.js | Single binary |
| Startup | ~500ms | ~10ms |

## License

MIT
