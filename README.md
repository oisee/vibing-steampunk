
![Vibing ABAP Developer](./media/vibing-steamer.png)

# mcp-adt-go

A Go-native MCP (Model Context Protocol) server for SAP ABAP Development Tools (ADT).

Single-binary distribution of 43 ADT tools for use with Claude and other MCP-compatible AI assistants.

## Why This Project?

This project brings **AI-assisted ABAP development** to Claude Code and Claude Desktop by exposing SAP ADT capabilities through the Model Context Protocol. With this MCP server, Claude can:

- Read and understand your ABAP codebase
- Create, modify, and delete ABAP objects
- Run syntax checks and unit tests
- Navigate code with find definition/references
- Format code with pretty printer
- Execute SQL queries against SAP tables

## Inspirations & Credits

This project stands on the shoulders of giants:

| Project | Author | Description |
|---------|--------|-------------|
| [abap-adt-api](https://github.com/marcellourbani/abap-adt-api) | Marcello Urbani | TypeScript library that implements the ADT REST API. Powers the [ABAP Remote FS](https://github.com/marcellourbani/vscode_abap_remote_fs) VS Code extension. The definitive reference for ADT API implementation. |
| [mcp-abap-adt](https://github.com/mario-andreschak/mcp-abap-adt) | Mario Andreschak | First MCP server for ABAP ADT in TypeScript/Node.js. Pioneered the concept of AI-assisted ABAP development via MCP. |

**mcp-adt-go** is a complete rewrite in Go, providing:
- Single binary with zero runtime dependencies
- Extended toolset (39 vs 13 tools)
- Full CRUD operations and code intelligence
- ~50x faster startup time

## Capability Matrix

Comparison of ADT capabilities across implementations:

| Capability | ADT (Eclipse) | abap-adt-api (TS) | **mcp-adt-go** |
|------------|:-------------:|:-----------------:|:--------------:|
| **Source Read** |
| Programs, Classes, Interfaces | Y | Y | **Y** |
| Functions, Function Groups | Y | Y | **Y** |
| Tables, Structures | Y | Y | **Y** |
| Includes | Y | Y | **Y** |
| Package Contents | Y | Y | **Y** |
| Type Info | Y | Y | **Y** |
| CDS Views | Y | Y | **Y** (dependencies) |
| RAP/BDEF | Y | Y | N |
| **Data Query** |
| Table Contents | Y | Y | **Y** |
| SQL Filtering | Y | Y | **Y** |
| Freestyle SQL | Y | Y | **Y** |
| **Development Tools** |
| Syntax Check | Y | Y | **Y** |
| Activation | Y | Y | **Y** |
| Unit Tests | Y | Y | **Y** |
| **CRUD Operations** |
| Lock/Unlock | Y | Y | **Y** |
| Create Objects | Y | Y | **Y** |
| Update Source | Y | Y | **Y** |
| Delete Objects | Y | Y | **Y** |
| Class Includes | Y | Y | **Y** |
| **Code Intelligence** |
| Find Definition | Y | Y | **Y** |
| Find References | Y | Y | **Y** |
| Code Completion | Y | Y | **Y** |
| Type Hierarchy | Y | Y | **Y** |
| Pretty Printer | Y | Y | **Y** |
| **Workflow Tools** |
| Surgical Edit (Edit tool pattern) | - | - | **Y** |
| Write & Activate | - | - | **Y** |
| Create & Activate | - | - | **Y** |
| Create with Tests | - | - | **Y** |
| **File-Based Deployment** |
| Deploy from File | - | - | **Y** |
| Save to File | - | - | **Y** |
| Rename Objects | - | - | **Y** |
| **Grep/Search** |
| Regex Search (single object) | - | - | **Y** |
| Regex Search (package-wide) | - | - | **Y** |
| **Transports** |
| Transport Management | Y | Y | N |
| **ATC** |
| Code Quality Checks | Y | Y | N |
| **Debugging** |
| Remote Debugging | Y | Y | N |

**Legend:** Y = Full support, P = Partial, N = Not implemented, - = Not applicable

## Documentation for AI Agents

**[MCP Usage Guide](MCP_USAGE.md)** - Machine-friendly reference for AI assistants using this MCP server. Includes:
- Tool selection decision trees
- Workflow patterns (search → edit, package-wide refactoring)
- Performance optimization (token usage, when to use which tool)
- Regex pattern library for ABAP
- Error handling patterns
- Integration examples (CI/CD, code review)

This guide follows emerging best practices for MCP documentation aimed at AI agents rather than human developers.

## Available Tools (43)

### Read Operations (15 tools)

| Tool | Description |
|------|-------------|
| `SearchObject` | Search for ABAP objects |
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
| `GetCDSDependencies` | Retrieve CDS view dependency tree (forward dependencies: what this view depends on) |
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
| `CreateObject` | Create new ABAP object (program, class, interface, include, function group, function module, package) |
| `UpdateSource` | Write source code to an object |
| `DeleteObject` | Delete an ABAP object |

### Class Include Operations (3 tools)

| Tool | Description |
|------|-------------|
| `GetClassInclude` | Retrieve class include source (definitions, implementations, macros, testclasses) |
| `CreateTestInclude` | Create test classes include for a class |
| `UpdateClassInclude` | Update class include source |

### Workflow Tools (5 tools)

Workflow tools are **composite/multi-step operations** that combine multiple ADT API calls into a single tool. They reduce round-trips and handle the complex orchestration required for common development tasks:

| Tool | Description | Steps Performed |
|------|-------------|-----------------|
| `EditSource` | ✅ **RECOMMENDED** - Surgical string replacement on ABAP source. Matches Edit tool pattern for local files. | GetSource → FindReplace → SyntaxCheck → Lock → Update → Unlock → Activate |
| `WriteProgram` | Update existing program with syntax check and activation | Lock → SyntaxCheck → UpdateSource → Unlock → Activate |
| `WriteClass` | Update existing class with syntax check and activation | Lock → SyntaxCheck → UpdateSource → Unlock → Activate |
| `CreateAndActivateProgram` | Create new program with source and activate it | Create → UpdateSource → Activate |
| `CreateClassWithTests` | Create class with unit tests and run them | Create → Lock → UpdateSource → CreateTestInclude → WriteTests → Unlock → Activate → RunUnitTests |

These tools significantly simplify AI-assisted development by handling locking, error checking, and activation automatically.

### File-Based Deployment Tools (3 tools)

**Solves token limit problem** for large generated files (like ML models, complex classes). These tools read/write ABAP source files directly from the filesystem, bypassing Claude's token limits:

| Tool | Description | Use Case |
|------|-------------|----------|
| `DeployFromFile` | ✅ **RECOMMENDED** - Smart deploy: auto-detects create vs update | Deploy any ABAP file (class, program, interface, function group/module) |
| `SaveToFile` | Save ABAP object source to local file (SAP → File) | Export objects for version control, bidirectional sync |
| `RenameObject` | Rename object by creating copy with new name | Fix naming conventions, refactor |

**Workflow executed:** Parse file → Detect type/name → Lock → Syntax check → Write → Unlock → Activate

**Supported file extensions:**
- `.clas.abap` - Classes
- `.prog.abap` - Programs
- `.intf.abap` - Interfaces
- `.fugr.abap` - Function Groups
- `.func.abap` - Function Modules

**Example:** Deploy a 3,948-line generated ML class without token limits:
```bash
# Claude calls:
DeployFromFile(file_path="/path/to/zcl_ml_iris.clas.abap", package_name="$ZAML_IRIS")
```

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

### Grep/Search Tools (2 tools)

| Tool | Description |
|------|-------------|
| `GrepObject` | Search for regex pattern in a single ABAP object. Returns matches with line numbers and optional context. |
| `GrepPackage` | Search for regex pattern across all source objects in a package. Returns matches grouped by object. |

**Features:**
- Full regex support (Go regexp syntax)
- Case-sensitive or case-insensitive matching
- Context lines (like `grep -C`)
- Object type filtering (programs, classes, interfaces, etc.)
- Max results limit for package-wide searches

**Use cases:**
- Find TODO comments before sprint planning
- Locate hardcoded values for refactoring
- Search for patterns across multiple programs/classes
- Discover all uses of a string literal
- Prepare for surgical edits with EditSource

## Installation

### Pre-built Binaries

Download from the [releases page](https://github.com/oisee/vibing-steamer/releases).

Available for:
- Linux (amd64, arm64, 386, arm)
- macOS (amd64, arm64/Apple Silicon)
- Windows (amd64, arm64, 386)

### From Source

```bash
git clone https://github.com/oisee/vibing-steamer.git
cd vibing-steamer

# Build for current platform
make build

# Build for all platforms
make build-all
```

## Configuration

The server supports multiple configuration methods with the following priority: **CLI flags > Environment variables > .env file > Defaults**

### CLI Flags

```bash
mcp-adt-go --url https://host:44300 --user admin --password secret
mcp-adt-go --url https://host:44300 --cookie-string "sap-usercontext=..."
mcp-adt-go --url https://host:44300 --cookie-file cookies.txt
mcp-adt-go --help  # Show all options
```

| Flag | Alias | Description |
|------|-------|-------------|
| `--url` | `--service` | SAP system URL (e.g., `https://host:44300`) |
| `--user` | `-u` | SAP username |
| `--password` | `-p`, `--pass` | SAP password |
| `--client` | | SAP client number (default: `001`) |
| `--language` | | SAP language (default: `EN`) |
| `--insecure` | | Skip TLS certificate verification |
| `--cookie-file` | | Path to Netscape-format cookie file |
| `--cookie-string` | | Cookie string (`key1=val1; key2=val2`) |
| `--verbose` | `-v` | Enable verbose logging to stderr |
| `--version` | | Show version information |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `SAP_URL` | SAP system URL |
| `SAP_USER` | SAP username |
| `SAP_PASSWORD` | SAP password |
| `SAP_CLIENT` | SAP client number |
| `SAP_LANGUAGE` | SAP language |
| `SAP_INSECURE` | Skip TLS verification (`true`/`false`) |
| `SAP_COOKIE_FILE` | Path to cookie file |
| `SAP_COOKIE_STRING` | Cookie string |
| `SAP_VERBOSE` | Enable verbose logging |

### .env File Support

The server automatically loads `.env` from the current directory:

```bash
# .env
SAP_URL=https://host:44300
SAP_USER=developer
SAP_PASSWORD=secret
SAP_CLIENT=001
SAP_INSECURE=true
```

### Authentication Methods

The server supports two authentication methods (only one can be used at a time):

1. **Basic Authentication** (username/password)
   ```bash
   mcp-adt-go --url https://host:44300 --user admin --password secret
   ```

2. **Cookie Authentication** (session cookies)
   ```bash
   # From cookie string
   mcp-adt-go --url https://host:44300 --cookie-string "sap-usercontext=abc; SAP_SESSIONID=xyz"

   # From Netscape-format cookie file
   mcp-adt-go --url https://host:44300 --cookie-file cookies.txt
   ```

Cookie authentication is useful when you have an existing browser session or need to bypass SSO.

## Usage with Claude Desktop

Add to your Claude Desktop configuration (`~/.config/claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "abap-adt": {
      "command": "/path/to/mcp-adt-go",
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
      "command": "/path/to/mcp-adt-go",
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
SAP_URL=http://host:port SAP_USER=user SAP_PASSWORD=pass \
  go test -tags=integration -v ./pkg/adt/

# Build for current platform
make build

# Build for all platforms
make build-all

# Build for specific OS
make build-linux
make build-darwin
make build-windows
```

## Architecture

```
vibing-steamer/
├── cmd/mcp-adt-go/          # CLI entry point (cobra/viper)
│   └── main.go              # Config resolution, auth, server startup
├── pkg/adt/                 # ADT client library
│   ├── client.go            # Main client facade + read operations
│   ├── config.go            # Configuration with functional options
│   ├── cookies.go           # Cookie parsing (Netscape format)
│   ├── http.go              # HTTP transport (CSRF, sessions, auth)
│   ├── crud.go              # CRUD operations (lock, create, update, delete)
│   ├── devtools.go          # Development tools (syntax check, activate, tests)
│   ├── codeintel.go         # Code intelligence (definition, refs, completion)
│   ├── workflows.go         # High-level composite operations
│   ├── fileparser.go        # ABAP file parser (detect type/name from files)
│   └── xml.go               # XML types and parsing
├── internal/mcp/            # MCP server implementation
│   └── server.go            # Tool registration and handlers (39 tools)
├── reports/                 # Project documentation and research
└── build/                   # Cross-platform binaries
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed architecture documentation.

## Project Status

| Metric | Value |
|--------|-------|
| **Tools** | 43 |
| **Unit Tests** | 91 |
| **Integration Tests** | 21+ |
| **Platforms** | 9 (Linux, macOS, Windows × amd64/arm64/386) |

## Roadmap

### In Progress
- [ ] **External Debugging & Trace Tools** ([Research Report](reports/2025-12-02-013-abap-debugging-and-trace-research.md) | [Verification](reports/2025-12-02-014-debugging-tools-summary-and-verification.md))
  - [ ] DebuggerAttach - Attach to running process
  - [ ] DebuggerSetBreakpoint - Set breakpoints with conditions
  - [ ] DebuggerStep - Step through execution
  - [ ] DebuggerGetVariables - Inspect variables
  - [ ] DebuggerSetVariable - Mock values during execution
  - [ ] DebuggerGetCallStack - Get call stack
  - [ ] DebuggerStartTrace - Start ABAP/SQL/profile trace
  - [ ] DebuggerGetTraceResults - Analyze trace data
- [x] **CDS Dependency Analysis** - Extract dependency tree for CDS views ([Investigation](reports/2025-12-02-016-cds-and-zray-endpoint-investigation.md))
- [ ] **$ZRAY Local Implementation** - Execute framework code locally ([Investigation](reports/2025-12-02-016-cds-and-zray-endpoint-investigation.md) | [Plan](reports/2025-12-02-015-cds-dependency-and-zray-local-implementation.md))

### Planned Features
- [ ] Transport Management (create, release, add objects)
- [ ] ATC (ABAP Test Cockpit) integration
- [ ] CDS View source read and annotations
- [ ] RAP/BDEF support (behavior definitions)
- [ ] Message class support
- [ ] Domain/Data element support

### Future Considerations
- [ ] WebSocket transport for real-time updates
- [ ] Batch operations for bulk changes
- [ ] Code generation templates
- [ ] Time-travel debugging with execution replay
- [ ] AI-powered test generation from traces
- [ ] Call graph visualization and export

## License

MIT

## Contributing

Contributions are welcome! Please see [ARCHITECTURE.md](ARCHITECTURE.md) and [CLAUDE.md](CLAUDE.md) for development guidelines.
