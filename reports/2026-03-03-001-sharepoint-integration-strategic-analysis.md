# SharePoint Integration for SAP Development Workflows — Strategic Analysis

**Date:** 2026-03-03
**Report ID:** 001
**Subject:** Integration of SharePoint design documents into VSP-powered SAP development workflows
**Related Documents:** [marianfoo/llm-client-sap-system-integration-sharepoint](https://github.com/marianfoo/llm-client-sap-system-integration-sharepoint)

---

## Problem Statement

Thousands of SAP-related design documents (functional specs, technical designs, architecture decisions, data model documentation, interface specs, configuration guides) live in SharePoint Online. These are essential context for development work but completely disconnected from the AI-powered VSP development workflow.

**Current state:** Documents are accessed via SharePoint keyword search, folder browsing, tribal knowledge (asking colleagues), or the ChatGPT SharePoint connector (attachment-only, no SAP context).

**Desired state:** When Claude is helping with SAP development via VSP, it should autonomously find and reference relevant design documents — e.g., checking the functional spec before modifying a BAPI, or finding the interface specification for a partner system.

## Reference Repository Analysis

[`marianfoo/llm-client-sap-system-integration-sharepoint`](https://github.com/marianfoo/llm-client-sap-system-integration-sharepoint) is a LibreChat deployment template that bundles:
- **SharePoint integration** via Entra ID OBO (On-Behalf-Of) token flow — attachment-only (manual File Picker iframe, 25 MB limit, no search/indexing)
- **VSP** as a read-only MCP server for SAP ADT access
- **SAP docs MCP** for offline SAP documentation full-text search
- **Ollama** for local LLM inference

**Key limitation for our use case:** The SharePoint integration is attachment-only. Users manually pick files per conversation. This doesn't scale to thousands of documents and doesn't support autonomous retrieval by the LLM.

**Architectural takeaways:** The project demonstrates a solid Docker-compose pattern for running multiple MCP servers as sidecars (VSP + SAP docs), with security hardening (iptables egress lockdown, read-only mode, tool allowlists). This deployment pattern is directly reusable.

---

## Three Approaches

### Approach A: SharePoint MCP Server (Native VSP Tool Group)

**What:** Build a new tool group inside VSP that connects directly to SharePoint via Microsoft Graph API.

```
Claude/MCP Client
    ↓ (MCP stdio)
VSP Binary
    ├── SAP ADT tools (existing, pkg/adt/)
    └── SharePoint tools (new, pkg/sharepoint/)
            ↓ (Microsoft Graph API)
        SharePoint Online
```

**New MCP Tools (5-6):**
| Tool | Description |
|------|-------------|
| `SPSearchDocuments` | Search SharePoint by keyword/metadata (KQL) |
| `SPGetDocument` | Retrieve document content (with text extraction) |
| `SPListSiteDocuments` | Browse a site/library folder structure |
| `SPGetDocumentMetadata` | Get metadata (author, modified date, custom columns) |
| `SPListRecentDocuments` | Recently modified documents |

**New Config:** `SP_TENANT_ID`, `SP_CLIENT_ID`, `SP_CLIENT_SECRET`, `SP_SITE_URL`, `SP_DRIVE_ID`, `SAP_FEATURE_SHAREPOINT` (auto/on/off). Tool group `"S"` — disabled with `--disabled-groups S`.

**Text Extraction:** Go libraries for docx, xlsx, pptx, pdf → plain text.

**Pros:** Single binary, tools alongside SAP ADT tools, follows existing VSP patterns, works with any MCP client.

**Cons:** Text extraction adds binary size/complexity. No semantic search (relies on KQL). VSP scope creep — it's an SAP tool, not a document tool. Client credentials = no per-user access control.

**Effort:** Medium-High (2-3 weeks).

---

### Approach B: Standalone SharePoint MCP Server (Sidecar)

**What:** Build a separate MCP server dedicated to SharePoint. Run alongside VSP as a second MCP server in Docker.

```
Claude/MCP Client
    ├── MCP connection 1 (stdio)
    │   └── VSP Binary (SAP ADT tools — unchanged)
    │
    └── MCP connection 2 (stdio or SSE)
        └── sp-docs-mcp (new, standalone)
                ↓ (Microsoft Graph API)
            SharePoint Online
```

**Implementation options:** Go (consistent with VSP), TypeScript (best Graph SDK + Office parsing), Python (best doc parsing ecosystem).

**Same MCP tools as Approach A**, plus enhanced capabilities easier in a dedicated server: chunking, caching, metadata enrichment, robust format conversion.

**Pros:** Clean separation of concerns, best-of-breed language/libraries, independent release cycle, standalone community value, no VSP impact.

**Cons:** Two binaries to configure, no shared server context between VSP and SharePoint tools.

**Effort:** Medium (1-2 weeks for TypeScript).

---

### Approach C: RAG Pipeline with Vector Search (Index + Retrieve)

**What:** Index all SharePoint documents into a vector database. Expose semantic search via MCP.

```
                    ┌─────────────────────────┐
                    │  Indexing Pipeline       │
                    │  (scheduled/webhook)     │
                    │  SharePoint → Extract →  │
                    │  Chunk → Embed → Store   │
                    └──────────┬──────────────┘
                               ↓
                    ┌─────────────────────────┐
                    │  Vector DB              │
                    │  (ChromaDB / Qdrant /   │
                    │   SQLite-vec / Postgres)│
                    └──────────┬──────────────┘
                               ↑
Claude/MCP Client              │
    ├── VSP (SAP ADT tools)    │
    └── sp-rag-mcp ────────────┘
         Tools: SemanticSearch, GetChunk, GetFullDocument
```

**MCP Tools:** `SemanticSearchDocuments`, `GetDocumentChunk`, `GetFullDocument`, `ListIndexedDocuments`, `GetDocumentSummary`.

**Pros:** Semantic search finds conceptually relevant documents (not just keyword matches). Handles thousands of documents efficiently. Pre-chunked content fits context windows. Can pre-filter by SAP module, project, date.

**Cons:** Significant infrastructure (vector DB, embedding model, indexer). Embedding costs. Index staleness. Most complex to build/maintain.

**Effort:** High (3-5 weeks).

---

## Comparison Matrix

| Criteria | A: Native VSP | B: Sidecar MCP | C: RAG Pipeline |
|----------|:---:|:---:|:---:|
| **Search quality** | Medium (KQL) | Medium (KQL) | High (semantic) |
| **Scales to 1000s of docs** | Yes (search) | Yes (search) | Best (pre-indexed) |
| **Infrastructure complexity** | Low | Low | High |
| **Deployment simplicity** | Best | Good | Complex |
| **Separation of concerns** | Poor | Best | Good |
| **Context window efficiency** | Low (full docs) | Medium (chunking) | Best (relevant chunks) |
| **Development effort** | Medium-High | Medium | High |
| **Maintenance burden** | Low | Low | High |
| **Per-user access control** | Possible (OBO) | Possible (OBO) | Complex (index-time ACLs) |

## Recommendation

**Start with Approach B (Sidecar MCP Server), designed from day one with a clear path to Approach C (RAG).**

The "tribal knowledge" search pattern is the strongest signal: when people find documents by asking colleagues who just *know* where things are, no keyword search will fully replace that. Semantic/vector search is the AI equivalent. So the architecture should be:

**Phase 1 (B):** Standalone MCP server with Graph API keyword search (KQL) + folder browsing + document retrieval. Ships in Docker alongside VSP. Immediate value — Claude can search and read design docs during ABAP development.

**Phase 2 (B→C):** Add a local vector index to the same server. The MCP tool interface (`SPSearchDocuments`) stays the same — it just gets smarter underneath. Indexer runs as a Docker sidecar service that crawls SharePoint on a schedule.

**Why this path:**
1. **Separation of concerns** — VSP stays focused on SAP ADT. Document access is a different domain.
2. **Fastest to value** — KQL search + document retrieval works today without any indexing infrastructure.
3. **Smooth upgrade** — Same tool interface, same Docker deployment. Adding vector search is additive, not a rewrite.
4. **Docker-native** — Slots directly into the marianfoo docker-compose pattern as another service.
5. **Community value** — A standalone SharePoint MCP server is useful beyond VSP and could be contributed to the broader MCP ecosystem.
6. **Mixed format support** — TypeScript/Node has mature libraries for all four formats (mammoth for docx, pdf-parse for PDF, exceljs for xlsx, pptx-parser for pptx).

## User Requirements (Collected)

| Question | Answer |
|----------|--------|
| Document formats | Mixed — Word, PDF, Excel, PowerPoint |
| Search patterns | All: keyword, SAP object/module, project code, folder browse, tribal knowledge |
| Hosting | Shared server (Docker) |
| Access control | Start shared (client credentials), plan for per-user (OBO) later |

---

## Appendix: ChatGPT Research Prompt for Diverse Ideation

The following prompt can be used with ChatGPT (GPT-4o or o3) to explore additional approaches beyond the three analysed above. It is deliberately broad and non-prescriptive.

---

You are a senior enterprise architect specializing in SAP development workflows, AI-assisted coding, and Microsoft 365 integration. I need your help thinking through a complex integration challenge. Please provide diverse, creative approaches — don't limit yourself to obvious solutions. I want breadth of ideas, including unconventional ones.

### The Problem

I work in an SAP development environment where we use an AI-powered MCP (Model Context Protocol) server called **VSP** (vibing-steampunk) to give Claude and other LLMs direct access to our SAP system via the ADT (ABAP Development Tools) REST API. VSP provides ~120 tools for reading source code, searching objects, running tests, debugging, deploying code, etc. It runs as a single Go binary and communicates over MCP stdio or HTTP.

**The gap:** We have **thousands of SAP-related design documents** stored in **SharePoint Online** — functional specs, technical designs, architecture decisions, data model documentation, interface specs, configuration guides, test plans, and more. These documents are essential context for development work, but they're completely disconnected from our AI development workflow.

**Current state:** Developers access these documents by:
- Searching SharePoint by keyword or project code
- Browsing known folder structures
- Asking colleagues who "just know" where things are (tribal knowledge)
- Using the SharePoint connector in ChatGPT (attachment-only, no SAP context)

**Desired state:** When Claude (or another LLM) is helping with SAP development via VSP, it should be able to autonomously find and reference relevant design documents. For example:
- "Before modifying this BAPI, let me check the functional specification" → searches SharePoint, finds the spec, reads the relevant sections
- "What was the design decision for this data model?" → finds the architecture decision record
- "Show me the interface specification for this partner system" → retrieves the relevant document

### Context Details

**Document characteristics:**
- Formats: Mixed — Word (.docx), PDF, Excel (.xlsx), PowerPoint (.pptx)
- Volume: Thousands of documents across multiple SharePoint sites and document libraries
- Organization: Folder hierarchies by project, module (MM, SD, FI, etc.), and document type
- Metadata: SharePoint columns include project codes, SAP module, document type, status, author
- Size: Varies from 2-page specs to 200-page functional specifications
- Language: Primarily English, some German
- Freshness: Documents are actively maintained; some are current, many are historical reference

**Technical environment:**
- SharePoint Online (Microsoft 365)
- Azure AD / Entra ID for authentication
- SAP systems accessible via ADT REST API (VSP handles this)
- MCP protocol for LLM ↔ tool communication
- Deployment target: Docker (shared server for the development team)
- VSP is open-source Go, but the SharePoint solution doesn't need to be Go

**Reference implementation:**
There's an existing project ([marianfoo/llm-client-sap-system-integration-sharepoint](https://github.com/marianfoo/llm-client-sap-system-integration-sharepoint)) that bundles LibreChat + VSP + SharePoint. However, its SharePoint integration is **attachment-only** — users manually pick files via a File Picker iframe, and the content is sent in-context to the LLM. No search, no indexing, no autonomous retrieval. This doesn't scale to thousands of documents.

**MCP Protocol basics (for context):**
- MCP servers expose "tools" (functions) that LLMs can call
- Each tool has a name, description, input schema, and returns text/content
- MCP supports stdio (local) and HTTP/SSE (remote) transports
- Multiple MCP servers can run simultaneously — the LLM sees all tools from all connected servers
- Example tool: `SearchObject` takes a query string, returns matching SAP objects

### What I Need From You

Please think broadly and provide **at least 5 distinct approaches** to solving this problem. For each approach:

1. **Name and one-line description**
2. **How it works** — architecture, data flow, key components
3. **What makes it unique** — why this approach vs. others
4. **Strengths** — what it does well
5. **Weaknesses** — where it falls short
6. **Complexity and effort** — rough implementation effort
7. **Best suited for** — what scenario makes this the right choice

### Dimensions to Explore

Don't limit yourself to these, but consider:

- **Search paradigm:** Keyword (KQL), full-text, semantic/vector, hybrid, graph-based, metadata-driven
- **Architecture:** Native VSP integration, standalone MCP server, proxy/gateway, existing platform extension, cloud service
- **Document processing:** Real-time extraction, pre-indexing, caching, summarization, chunking strategies
- **Context delivery:** Full document, relevant sections only, summaries, structured extracts, multi-document synthesis
- **Authentication:** App-level (client credentials), delegated (user context/OBO), mixed
- **Infrastructure:** Zero-infra (API-only), lightweight (single binary), medium (Docker services), heavy (managed platform)
- **AI-native approaches:** Using LLMs themselves for document understanding, classification, or routing
- **Existing ecosystem:** Microsoft 365 Copilot APIs, SharePoint Search REST API, Microsoft Graph Search, Azure AI Search, Azure OpenAI on your data, Copilot Studio connectors
- **Hybrid approaches:** Combining SharePoint with other document sources (Confluence, Git wikis, SAP Solution Manager, etc.)
- **Progressive enhancement:** What's the minimal viable integration, and how does it evolve?

### Evaluation Criteria

Rate each approach against:
- **Search quality** — Can it find the right document for a vague query like "the interface spec for the MM procurement integration"?
- **Scale** — Does it handle thousands of documents efficiently?
- **Latency** — How fast is document retrieval during an interactive coding session?
- **Maintenance** — What's the ongoing operational burden?
- **Security** — Does it respect SharePoint permissions? Can it handle sensitive documents?
- **Developer experience** — How seamless is it from the LLM's perspective?
- **Portability** — Does it lock into a specific LLM/platform, or work broadly?

### Bonus Questions

After presenting your approaches, please also address:

1. **What's the "cheat code"?** — Is there an existing product, service, or open-source project that already solves 80% of this problem and just needs to be wired in?
2. **What would a large enterprise (5000+ developers) do differently than a small team (5-10)?**
3. **What's the approach you'd take if you had unlimited budget vs. zero budget?**
4. **Are there approaches that combine document retrieval with document *understanding* — e.g., not just finding the spec, but extracting the relevant data model definition from it?**
5. **How would you handle document versioning — when Claude finds a spec, how does it know it's reading the current version vs. an obsolete draft?**
