# MCP SDK Comparison Across Languages

**Date:** 2026-01-08
**Report ID:** 002
**Subject:** Comparative analysis of MCP SDK implementations
**Related Documents:** 2026-01-08-001-language-assessment-meta-tool-architecture.md

---

## Executive Summary

This report compares Model Context Protocol (MCP) SDK implementations across Rust, Go, TypeScript, Python, and Java. The key insight: **most surface-level differences don't matter**. What matters is schema-handler binding safety and feature completeness.

For VSP specifically: **no changes needed**. VSP already uses mark3labs/mcp-go, which has 2.5x the adoption and more features than the official Go SDK.

---

## What Actually Matters

### 1. Schema-Handler Binding (The Only Meaningful Difference)

MCP requires JSON Schema definitions for each tool. The critical architectural difference:

| Approach | Languages | Risk |
|----------|-----------|------|
| **Separate** - schema defined in one place, handler in another | Go, TS, Python, Java | Schema/handler drift, runtime errors |
| **Unified** - schema derived from handler types | Rust (rmcp) | Compile error if mismatched |

**Go example (separate):**
```go
// Schema says "program_name"...
mcp.WithString("program_name", mcp.Required())

// ...handler typos "programName" = silent runtime bug
args["programName"].(string)  // nil, no compile error
```

**Rust example (unified):**
```rust
// Schema IS the struct - impossible to typo
#[derive(Tool)]
struct GetProgram {
    #[tool(required)]
    program_name: String,  // used as self.program_name
}
```

### 2. Error Surface

| SDK | Missing Required Param | Wrong Type | Handler Panic |
|-----|------------------------|------------|---------------|
| Go | Runtime nil/empty check | Runtime type assertion panic | Recoverable |
| TypeScript | Runtime validation | Runtime (or Valibot) | Uncaught exception |
| Python | Runtime KeyError | Runtime TypeError | Uncaught exception |
| Rust | **Compile error** | **Compile error** | Must handle Result |

### 3. Performance (Irrelevant for SAP)

| Metric | Difference | Reality for VSP |
|--------|------------|-----------------|
| Startup | 10ms (Go/Rust) vs 500ms (Python) | Negligible vs SAP auth overhead |
| Memory | 20MB (Go) vs 100MB (Python) | Both fine for any deployment |
| Throughput | All handle 1000+ req/sec | SAP is 2-5 req/sec bottleneck |

**SAP HTTP latency (100-500ms per request) dominates everything.** SDK performance differences are noise.

### 4. What DOESN'T Differ

All mature SDKs handle these equally well:
- JSON-RPC transport (stdio/HTTP/WebSocket)
- Tool registration and dispatch
- Async operations
- Error responses
- Protocol compliance

---

## SDK Overview

| Language | SDK | Version | Status | Maintainer |
|----------|-----|---------|--------|------------|
| **TypeScript** | `@modelcontextprotocol/sdk` | v1.25.0 | Reference impl | Anthropic |
| **Python** | `mcp` | v1.25.0 | Official | Anthropic |
| **Go** | `mark3labs/mcp-go` | v0.43.2 | Community (dominant) | Mark Adams |
| **Go** | `modelcontextprotocol/go-sdk` | v1.2.0 | Official | Anthropic + Google |
| **Rust** | `rmcp` | v0.12.0 | Official | Anthropic |
| **Java** | Spring AI MCP | v1.0.0-M5 | Official Spring | VMware/Pivotal |

### Go SDK Comparison (Detailed)

VSP uses `mark3labs/mcp-go`. Here's how it compares to the official SDK:

| Metric | mark3labs/mcp-go | Official go-sdk |
|--------|------------------|-----------------|
| **Adoption** | 1,020+ importers | 395 importers |
| **GitHub Stars** | 7.9k | ~400 |
| **Releases** | 66 (v0.43.2) | 3 (v1.2.0) |
| **MCP Spec** | 2025-11-25 | 2025-11-25 |

**Transports:**

| Transport | mark3labs | Official |
|-----------|-----------|----------|
| stdio | ✅ | ✅ |
| SSE | ✅ | ❌ |
| Streamable HTTP | ✅ | ❌ |
| CommandTransport | ❌ | ✅ |

**Features:**

| Feature | mark3labs | Official |
|---------|-----------|----------|
| Tools/Resources/Prompts | ✅ | ✅ |
| Session management | ✅ | ❌ |
| Middleware | ✅ | ❌ |
| Panic recovery | ✅ | ❌ |
| Completions | ✅ | ❌ |
| OAuth | ❌ | ✅ |

**Official SDK acknowledges mark3labs:**
> "Several third party Go MCP SDKs inspired the development of this official SDK" - citing mcp-go as "notably influential" and noting it "continues to be a viable alternative."

**Verdict:** mark3labs/mcp-go is more mature and feature-complete. No reason to migrate.

---

## Detailed Analysis

### TypeScript - Reference Implementation

```typescript
import { Server } from "@modelcontextprotocol/sdk/server";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio";

const server = new Server({
  name: "example",
  version: "1.0.0"
}, {
  capabilities: { tools: {} }
});

server.setRequestHandler(ListToolsRequestSchema, async () => ({
  tools: [{ name: "greet", description: "Greet user", inputSchema: {...} }]
}));

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  if (request.params.name === "greet") {
    return { content: [{ type: "text", text: "Hello!" }] };
  }
});

const transport = new StdioServerTransport();
await server.connect(transport);
```

**Pros:**
- Reference implementation - always up-to-date with spec
- Excellent TypeScript types
- First-class async/await
- Well-documented with examples

**Cons:**
- Node.js/Bun runtime required (or compile with Bun)
- Larger binary size when compiled (~50MB)

---

### Python - Official SDK

```python
from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import Tool, TextContent

server = Server("example")

@server.list_tools()
async def list_tools():
    return [Tool(name="greet", description="Greet user", inputSchema={...})]

@server.call_tool()
async def call_tool(name: str, arguments: dict):
    if name == "greet":
        return [TextContent(type="text", text="Hello!")]

async def main():
    async with stdio_server() as (read, write):
        await server.run(read, write)
```

**Pros:**
- Decorator-based API (Pythonic)
- Excellent for data science workflows
- PyRFC integration possible
- Jupyter notebook support

**Cons:**
- Distribution complexity (PyInstaller ~100MB, or requires Python)
- Higher memory footprint (~80-150MB)
- Slower startup (~500ms)

---

### Go - Official SDK (NEW: v1.0.0 stable Dec 2025)

```go
package main

import (
    "github.com/modelcontextprotocol/go-sdk/mcp"
    "github.com/modelcontextprotocol/go-sdk/server"
)

func main() {
    s := server.NewServer("example", "1.0.0")

    s.AddTool(mcp.NewTool("greet",
        mcp.WithDescription("Greet user"),
        mcp.WithString("name", mcp.Required()),
    ), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        name := req.Params.Arguments["name"].(string)
        return mcp.NewToolResultText("Hello, " + name), nil
    })

    s.ServeStdio()
}
```

**Pros:**
- Official Anthropic SDK (reached v1.0.0 stable)
- Single binary (~15MB)
- Fast startup (~10ms)
- Low memory (~20MB)
- Excellent HTTP/WebSocket support

**Cons:**
- Schema and handler disconnected (runtime type assertions)
- No compile-time validation of parameter access
- Verbose error handling

**Note:** VSP currently uses `mark3labs/mcp-go` (community SDK). Migration to official SDK is recommended.

---

### Rust - Official SDK (rmcp)

```rust
use rmcp::{Server, ServerHandler, Tool, tool};
use serde::Deserialize;

#[derive(Deserialize, Tool)]
#[tool(name = "greet", description = "Greet user")]
struct Greet {
    #[tool(required)]
    name: String,
}

#[async_trait]
impl ServerHandler for Greet {
    async fn handle(&self) -> Result<ToolResult, Error> {
        Ok(ToolResult::text(format!("Hello, {}!", self.name)))
    }
}

#[tokio::main]
async fn main() {
    let server = Server::new("example", "1.0.0")
        .with_tool::<Greet>();
    server.serve_stdio().await;
}
```

**Pros:**
- Compile-time schema validation (derive macros)
- Zero-cost abstractions
- Smallest binary (~10MB)
- Memory safety guarantees
- Exhaustive pattern matching for tool dispatch

**Cons:**
- Steeper learning curve
- Longer compile times (2-5 min full rebuild)
- Newer ecosystem (v0.12.0 - still maturing)

---

### Rust - Community SDKs

#### rust-mcp-sdk (v0.8.1)

```rust
use rust_mcp_sdk::{McpServer, ToolDefinition};

let server = McpServer::new("example", "1.0.0");
server.add_tool(ToolDefinition {
    name: "greet".into(),
    description: "Greet user".into(),
    input_schema: json!({...}),
    handler: |args| async { Ok(json!({"text": "Hello!"})) },
});
```

- 11K SLoC, async-first
- More mature than rmcp (started earlier)
- Good documentation

#### mcp-protocol-sdk (v0.5.1)

- Production-ready focus
- Emphasizes stability over features
- Smaller API surface

---

### Java - Spring AI MCP

```java
@Configuration
public class McpConfig {
    @Bean
    public McpServer mcpServer() {
        return McpServer.builder()
            .name("example")
            .version("1.0.0")
            .tool("greet", "Greet user", this::greet)
            .build();
    }

    private ToolResult greet(Map<String, Object> args) {
        String name = (String) args.get("name");
        return ToolResult.text("Hello, " + name);
    }
}
```

**Pros:**
- Spring ecosystem integration
- JCo (SAP Java Connector) available
- Enterprise-grade (transactions, security)
- GraalVM native image support

**Cons:**
- Heavy runtime without GraalVM (~200MB, 2s startup)
- GraalVM compilation slow (5-10 min)
- Configuration complexity

---

## Summary Comparison

| Aspect | TS | Python | Go | Rust | Java |
|--------|-----|--------|-----|------|------|
| **Schema binding** | Runtime | Runtime | Runtime | **Compile** | Runtime |
| **Binary** | 50MB | 100MB+ | 15MB | 10MB | 50MB* |
| **Startup** | 50ms | 500ms | 10ms | 10ms | 200ms* |
| **SAP RFC** | No | PyRFC | No | FFI | JCo |
| **Maturity** | Reference | Stable | v1.0 stable | v0.12 | M5 |

*With GraalVM native image

---

## Real Decision Factors

The question isn't "which SDK is better" but "what do you actually need?"

| If You Care About... | Choose | Why |
|---------------------|--------|-----|
| **Eliminating runtime type bugs** | Rust | Only SDK with compile-time schema validation |
| **Fastest iteration cycle** | TypeScript or Python | No compile step, REPL available |
| **Single binary, low footprint** | Go or Rust | 10-15MB, 10ms startup |
| **SAP RFC (not ADT)** | Python or Java | PyRFC / JCo libraries |
| **Existing Go codebase** | Stay on Go | Migration cost > benefit |

---

## Recommendations

### For New MCP Projects

| Need | Recommended | Why |
|------|-------------|-----|
| **General Go server** | mark3labs/mcp-go | More features, larger community, battle-tested |
| **OAuth required** | Official go-sdk | Built-in OAuth support |
| **Type safety critical** | Rust rmcp | Compile-time schema validation |
| **Rapid prototyping** | TypeScript | Reference impl, great DX |

### For VSP Specifically

| Current State | Problem? | Action |
|---------------|----------|--------|
| Using `mark3labs/mcp-go` | None | **Keep it** |
| Schema/handler separate | Minor drift risk | Tests cover this (244 passing) |
| Runtime type assertions | Established patterns | Not worth rewrite |

**Bottom line:** VSP is already on the better Go SDK. mark3labs/mcp-go has 2.5x the adoption, more transports (SSE, Streamable HTTP), and more features (middleware, session management) than the official SDK. No migration needed.

---

## Conclusion

**Two meaningful SDK differences:**
1. **Schema-handler binding** - Rust compile-time vs others runtime
2. **Feature completeness** - mark3labs/mcp-go leads in Go ecosystem

**For VSP:**
- Already on the best Go SDK (mark3labs/mcp-go)
- 244 tests catch schema drift
- SAP latency dominates performance
- No changes needed

**Bigger wins are elsewhere:** The universal tool architecture (Report 001) saves 99% of token overhead - far more impactful than any SDK choice.
