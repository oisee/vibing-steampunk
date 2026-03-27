# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.32.0] - 2026-03-22
### Features

- **CLI Toolchain** — 28 commands, full ABAP DevOps from the terminal:
  - `vsp query <table> --top N --where "..."` — query SAP tables
  - `vsp grep <pattern> --package PKG` — search source code
  - `vsp graph CLAS ZCL_FOO --direction callers` — call graph with WBCROSSGT/CROSS fallback
  - `vsp deps '$PKG' --format summary` — package dependency analysis + transport readiness
  - `vsp system info` — SAP version, kernel, ZADT_VSP availability check
  - `vsp lint --file src.abap` — offline ABAP linter (8 rules, abaplint-compatible)
  - `vsp parse --stdin --format json` — offline ABAP parser
  - `vsp compile wasm prog.wasm --class ZCL_DEMO` — WASM→ABAP compiler (offline)
  - `vsp compile ts lexer.ts --prefix zcl_` — TypeScript→ABAP transpiler
  - `vsp execute "WRITE 'hello'."` — run ABAP on SAP
  - `vsp context CLAS ZCL_FOO --depth 2` — multi-level dependency context
- **Graph with Fallback** — `vsp graph` tries ADT call graph API first, falls back to WBCROSSGT+CROSS table queries. Supports CLAS, INTF, PROG, FUGR, TRAN (resolves via TSTC).
- **Package Deps Analysis** — `vsp deps` classifies all references as internal/external-custom/SAP-standard. Shows transport readiness: "self-contained" vs "needs prerequisites".
- **CLI Documentation** — `docs/cli-guide.md`: complete reference, feature requirements matrix, pipeline examples, multi-system profiles.
- **WASM Self-Host Verified** — 3-way correctness proof (Native 51/51, Go compiler OK, ABAP self-host 11/11). 12 functions including recursion, loops, if-result blocks, select.
- **Native ABAP Lexer+Parser** — abaplint port: 100% oracle match on 22K tokens, 3K statements, 91 types.
- **ABAP Linter** — 8 rules, 100% oracle match on 4 verified rules, 795μs/file.
- **TS→Go Transpiler** — `pkg/ts2go`: produces valid Go from abaplint TypeScript (LexerBuffer, LexerStream, Lexer all compile).
- **Lua Bindings** — 5 new functions: `query()`, `lint()`, `parse()`, `context()`, `systemInfo()`. Total: 50+ Lua→SAP bindings.
- **Example Scripts** — `package-audit.lua` (lint+parse package), `table-explorer.lua` (SQL queries), `dependency-check.lua` (transport readiness).
- **YAML Workflow** — `quality-gate.yaml` for pre-transport quality checks.

## [2.31.0] - 2026-03-20
### Features

- **Native Go ABAP Lexer** (`pkg/abaplint`): Mechanical port of the [abaplint](https://github.com/abaplint/abaplint) TypeScript lexer to native Go. 48 token types, 6 lexer modes (normal, string, backtick, template, comment, pragma), whitespace-context encoding. Oracle-verified: 100% match on 22,612 tokens across 29 ABAP files. ~3.5M tokens/sec, zero dependencies.
- **Dependency Context Depth** (`pkg/ctxcomp`): `GetContext` now supports `depth` parameter (1-3) for multi-level dependency expansion. Level 1 = direct deps (default), level 2 = deps of deps, level 3 = three levels deep. Tracks visited objects to avoid cycles, shares maxDeps budget across levels.
- **Oracle Differential Testing Framework** (`pkg/abaplint/testdata/oracle.js`): Node.js script runs the real TypeScript abaplint on ABAP files, generates JSON fixtures. Go tests compare token-by-token: string, type, row, col. Reports per-file and aggregate KPIs (str/type/pos match rates).

## [2.30.0] - 2026-03-20
### Features

- **WASM-to-ABAP AOT Compiler** (`pkg/wasmcomp`): Compiles WebAssembly binaries to native ABAP source code. Parses .wasm binaries, converts stack machine to SSA form, emits ABAP classes or function groups. 100% opcode coverage for QuickJS (453K instructions). Three backends: FUGR, Class, Hybrid.
- **Self-Hosting WASM Compiler on SAP** (`embedded/abap/wasm_compiler`): 785 lines of ABAP that parse any .wasm binary and compile it to executable ABAP — entirely within SAP. Uses `GENERATE SUBROUTINE POOL` for runtime compilation. Verified: `add(2,3)=5`, `factorial(10)=3,628,800` on SAP A4H.
- **TypeScript-to-ABAP Transpiler** (`pkg/ts2abap`): Direct TS→ABAP transpilation without WASM intermediate. Produces clean OO ABAP with proper class definitions, method signatures, and ABAP naming conventions. 800x smaller output than the WASM path.
- **abaplint Lexer on SAP**: Lars Hvam's abaplint ABAP lexer transpiled from TypeScript to native ABAP (51 classes, 495 lines). Deployed to SAP A4H, tokenizes ABAP source code at native speed.
- **QuickJS compiled to ABAP**: Full QuickJS JavaScript engine (1,410 WASM functions) compiled to 101K lines of ABAP. 5.5x line compression via aggressive statement packing.
- **abaplint parser compiled to ABAP**: Full @abaplint/core (26.5MB WASM) compiled to 396K lines of ABAP via the WASM compiler.
- **Line Packing**: Pack multiple ABAP statements per line (up to 240 chars). Control flow, DATA declarations, assignments all pack together. 557K→101K lines (5.5x reduction).
- **Function Deduplication**: SHA256 hash of function type+locals+bytecode identifies identical functions. Duplicates redirect to canonical implementation.
- **WASI Shim**: All 9 QuickJS WASI imports implemented (fd_write with iov parsing, clock_time_get, environ stubs).
- **Batch Deploy via CLI**: `vsp deploy *.clas.abap '$PKG'` — deploy multiple ABAP files in a shell loop. 40 token classes deployed with zero failures.

## [2.29.0] - 2026-03-19
### Features

- **Hyperfocused Mode** (`--mode hyperfocused`): Single `SAP(action, target, params)` tool replaces 122 individual tools. Reduces MCP schema overhead from ~40K to ~200 tokens (99.5% reduction). All safety controls work identically.
- **Method-Level Surgery**: Read/write individual class methods without pulling entire class source. 95% token reduction. Context compression scopes to the method's own dependencies.
- **Unified SAP_MODE**: Merged `SAP_TOOL_MODE` into `SAP_MODE` with three values: focused (81 tools), expert (122 tools), hyperfocused (1 tool). Removed `--tool-mode` flag.

## [2.28.0] - 2026-03-18
### Features

- **Context Compression** (`pkg/ctxcomp`): New package that extracts dependencies from ABAP source and produces compressed "prologue" text containing only public API contracts. Classes are reduced to PUBLIC SECTION only (7-30x compression), interfaces pass through as-is, function modules extract signature blocks.
- **GetSource `include_context` flag**: `GetSource` now appends dependency context by default — one MCP call returns source + compressed public API of all referenced objects. Set `include_context: false` for raw source only. `max_deps` parameter controls limit (default 20).
- **GetContext MCP tool**: Standalone tool for dependency analysis — accepts source or fetches it, resolves contracts from SAP, returns formatted prologue. Added to focused mode whitelist (code intelligence group).
- **LSP context push**: `vsp/context` notification sent on `didOpen` with compressed dependency prologue (best-effort, non-blocking).
- 10 regex patterns for ABAP dependency extraction: TYPE REF TO, NEW, =>, ~, INHERITING FROM, INTERFACES, CALL FUNCTION, CAST, RAISING, ZCX_* exception references.
- 37 unit tests in `pkg/ctxcomp` including tests against embedded ABAP files and real SAP A4H sources.


## [2.27.0] - 2026-03-01
### Features

- Iterative activation with package filtering + 100 stars article ([`8d2c343`](https://github.com/oisee/vibing-steampunk/commit/8d2c343e50f79f48663418568deade412337cd03))



## [2.26.0] - 2026-02-04
### Bug Fixes

- PackageExists fails for local packages with $ in name ([`83e8626`](https://github.com/oisee/vibing-steampunk/commit/83e86269f56eb5a3d6983385de3ff5276083d31e))



## [2.25.0] - 2026-02-03
### Bug Fixes

- Namespace URL encoding for all ADT operations ([`59b4b90`](https://github.com/oisee/vibing-steampunk/commit/59b4b9061497d86fb6e599e5b37382edee865a1e))


### Features

- Allow transportable package creation with --enable-transports ([`e483537`](https://github.com/oisee/vibing-steampunk/commit/e483537958dfd7243abfbce8be37214d0abe8ac2))
- CreatePackage software_component + viper env var fix ([`c18309b`](https://github.com/oisee/vibing-steampunk/commit/c18309b0b9e14d90cd65e00eb2f77595a0d0f7cd))



## [2.24.0] - 2026-02-03
### Features

- V2.23.0 - GitExport to disk, GetAbapHelp via WebSocket ([`ddf5c22`](https://github.com/oisee/vibing-steampunk/commit/ddf5c22f84ebdd9fbcfc5dcf771989487106af7f))
- V2.24.0 - Transportable Edits Safety Feature ([`3a9b0b0`](https://github.com/oisee/vibing-steampunk/commit/3a9b0b0bea276e7ca9ae556a55cc710fd5a44831))



## [2.23.0] - 2026-02-02
### Features

- Add granular tool visibility control via .vsp.json ([`f8fd717`](https://github.com/oisee/vibing-steampunk/commit/f8fd717c0acbd62590aec602e88efc618be13d77))
- Add GetAbapHelp tool for ABAP keyword documentation (#10) ([`434ed5e`](https://github.com/oisee/vibing-steampunk/commit/434ed5e83240cf52f3be334930c5b8602071c0cf))
- Add Level 2 GetAbapHelp - real docs from SAP system via ZADT_VSP ([`b78803d`](https://github.com/oisee/vibing-steampunk/commit/b78803d339f76b2d3b92de4276cabcec106dc30a))
- GitExport saves ZIP to disk, GetAbapHelp uses amdpWSClient ([`7c01351`](https://github.com/oisee/vibing-steampunk/commit/7c01351a783ca7588424a65c2fa64e2c21bce794))



## [2.22.0] - 2026-02-01
### Bug Fixes

- Transport API 406 error and EditSource transport support ([`c726bfe`](https://github.com/oisee/vibing-steampunk/commit/c726bfeb08d43357622853a4fa7d34d58a01469b))
- Honor HTTP_PROXY/HTTPS_PROXY environment variables (#13) ([`a1af66f`](https://github.com/oisee/vibing-steampunk/commit/a1af66f83ad050a0799442c75645861c9a5ba680))


### Features

- Add MoveObject tool and refactor WebSocket code ([`2d3d40c`](https://github.com/oisee/vibing-steampunk/commit/2d3d40cb472d4f0193f62870a5fcd172b35380cf))
- Add SAP_TERMINAL_ID config for SAP GUI breakpoint sharing ([`677e7ce`](https://github.com/oisee/vibing-steampunk/commit/677e7cee84d456f5eb2b6009a4c47d9afcd7af31))



## [2.21.0] - 2026-01-06
### Bug Fixes

- WebSocket reconnection check in report handlers ([`52e17c9`](https://github.com/oisee/vibing-steampunk/commit/52e17c9d654607271bc923a47c863fff830ef0dd))
- Improve error handling in GetSystemInfo and CSRF fetch ([`b9fb06b`](https://github.com/oisee/vibing-steampunk/commit/b9fb06b444a86c0057d26083d79176cee98a08eb))


### Features

- Add function module support to ImportFromFile ([`c7997c0`](https://github.com/oisee/vibing-steampunk/commit/c7997c07105f1a35ac45e2fa1967bac56479762f))
- Add method-aware breakpoints with include resolution ([`54417f6`](https://github.com/oisee/vibing-steampunk/commit/54417f6e9cdb06052332f81d0475aadbd83ea31f))
- Method-level source operations for GetSource, EditSource, WriteSource ([`1fa5065`](https://github.com/oisee/vibing-steampunk/commit/1fa5065390f191fe1eeb4183d0a491c468082186))



## [2.20.0] - 2026-01-06
### Bug Fixes

- WebSocket client parameter order & mcp-to-vsp password sync ([`29abb0c`](https://github.com/oisee/vibing-steampunk/commit/29abb0ce7e564720e165d528428e0618273750e5))
- Add .abapgit.xml to GitExport ZIP output ([`93dc5ef`](https://github.com/oisee/vibing-steampunk/commit/93dc5ef05426d6ebdfbb1e96a5301711e0b08327))
- Use FULL folder logic for multi-package exports ([`dafd1f5`](https://github.com/oisee/vibing-steampunk/commit/dafd1f52c6f4d55f92742a4b48d839fafdbdea6c))


### Features

- Make sync-embedded for exporting ZADT_VSP from SAP ([`ab47d27`](https://github.com/oisee/vibing-steampunk/commit/ab47d273b6c033e6cad98cc986eba877f4fc5f1b))
- CLI subcommands with system profiles ([`cdab42c`](https://github.com/oisee/vibing-steampunk/commit/cdab42cb961d7bde5156e8a4e764daf5a94e20c8))
- Vsp config init/show commands ([`bf90c25`](https://github.com/oisee/vibing-steampunk/commit/bf90c25b983caa4a7879112c887e65f7412467d1))
- Vsp config mcp-to-vsp and vsp-to-mcp commands ([`717cd9a`](https://github.com/oisee/vibing-steampunk/commit/717cd9adb8909707c68a28cea1a1f8b954cd539c))
- Cookie authentication support in CLI system profiles ([`d83080b`](https://github.com/oisee/vibing-steampunk/commit/d83080bbd466ad71cb97f1baae0b9b7f85049002))



## [2.19.1] - 2026-01-06
### Bug Fixes

- WebSocket TLS for self-signed certificates (#1) ([`181f523`](https://github.com/oisee/vibing-steampunk/commit/181f52365c057a9aeb1c9184cf94ee4d34373b0e))


### Features

- Tool aliases and heading texts support ([`d29549a`](https://github.com/oisee/vibing-steampunk/commit/d29549a8ef29806639b9561d50ae1972435735e1))



## [2.19.0] - 2026-01-05
### Bug Fixes

- GetSystemInfo uses SQL fallback for reliability ([`3c454a6`](https://github.com/oisee/vibing-steampunk/commit/3c454a6a3fd3d9f9e08e30aa9cdc49eebf2d24ef))


### Features

- Interactive CLI debugger (vsp debug) ([`f1358e9`](https://github.com/oisee/vibing-steampunk/commit/f1358e9773e4b3f07ae32287126a5ceb3786cc94))
- Quick wins - GetMessages, ListDumps, ActivatePackage, X group ([`2706797`](https://github.com/oisee/vibing-steampunk/commit/27067971ef521c7337257d0d534570f812f65be4))
- CreateTable tool + GetMessages fix ([`a71ec42`](https://github.com/oisee/vibing-steampunk/commit/a71ec427e0548afdc572d78887aaae5eefa822e3))
- CompareSource, CloneObject, GetClassInfo tools ([`8550435`](https://github.com/oisee/vibing-steampunk/commit/8550435b6bb82f0e9822cbed3772791788daa800))
- RunReportAsync and GetAsyncResult for background execution ([`56dc11a`](https://github.com/oisee/vibing-steampunk/commit/56dc11af633cec85d13ddee46c2b149708c375b5))



## [2.18.0] - 2026-01-02
### Features

- WebSocket-based debugger tools via ZADT_VSP ([`c3a3780`](https://github.com/oisee/vibing-steampunk/commit/c3a3780006c80c8d380d52ed3cfe41b60d25684e))
- Consolidate $ZADT_VSP package + lock cleanup fix ([`5e4530a`](https://github.com/oisee/vibing-steampunk/commit/5e4530a4f3ea6f88acb3bb7e132078c531c1c4a5))
- Report execution tools + packageExists fix ([`3df8955`](https://github.com/oisee/vibing-steampunk/commit/3df8955f110fd870ef24c98c7681865cbb6a0baf))



## [2.17.1] - 2025-12-24
### Bug Fixes

- Install tools upsert - proper package/object existence checks ([`4505237`](https://github.com/oisee/vibing-steampunk/commit/450523755f3f9ad47151b1d0887e3d0bc4ee5d38))


### Features

- InstallZADTVSP tool for one-command deployment ([`1ee4962`](https://github.com/oisee/vibing-steampunk/commit/1ee496222403301e7db6615158d96b362c20aa07))
- InstallAbapGit tool + dependency embedding architecture ([`a3f1fa0`](https://github.com/oisee/vibing-steampunk/commit/a3f1fa09960c7f554be5a9f919474d6690636bc5))



## [2.16.0] - 2025-12-23
### Features

- AbapGit WebSocket integration (Git domain) ([`a73d2a6`](https://github.com/oisee/vibing-steampunk/commit/a73d2a6c9a9e797413a77c6ce61e2c4a1a5dfa45))
- Complete abapGit WebSocket integration (v2.16.0) ([`78e2c6d`](https://github.com/oisee/vibing-steampunk/commit/78e2c6d16733a01cce29e2c7b4a7641bd1aba389))



## [2.15.1] - 2025-12-22
### Bug Fixes

- Correct unit test count 216 → 244 ([`c931533`](https://github.com/oisee/vibing-steampunk/commit/c93153344683b579f061766d9d5cbef557e79966))



## [2.15.0] - 2025-12-21
### Features

- Variable History Recording (Phase 5.2) ([`29e192d`](https://github.com/oisee/vibing-steampunk/commit/29e192d4c4510cd0b66204495547cae38da28888))
- Extended breakpoint types + Watchpoint Scripting (Phase 5.4) ([`3dd20cd`](https://github.com/oisee/vibing-steampunk/commit/3dd20cd7b506264808dcec50ec649e6ee6351298))
- Force Replay - State Injection (Phase 5.5) - THE KILLER FEATURE ([`70fb43f`](https://github.com/oisee/vibing-steampunk/commit/70fb43fe85da3d46759b40ef44321701a044a63d))
- Phase 5 TAS-Style Debugging Complete (v2.15.0) ([`19405b2`](https://github.com/oisee/vibing-steampunk/commit/19405b2a4a13210f8809748d263f80f0524e4a61))



## [2.14.0] - 2025-12-21
### Features

- Lua scripting integration (Phase 5.1) ([`0e5c5c2`](https://github.com/oisee/vibing-steampunk/commit/0e5c5c2681fcca270d21a476139a387dfd73461a))



## [2.13.0] - 2025-12-21
### Bug Fixes

- External debugger breakpoint XML format & unit test parsing ([`296b8f3`](https://github.com/oisee/vibing-steampunk/commit/296b8f31530810440db43eeb5609527bc9ec156c))
- GetDumps Accept header & add WebSocket debugging ADR ([`2eb4a5e`](https://github.com/oisee/vibing-steampunk/commit/2eb4a5efd27241c866bc7a8c6234fa2f6471b7d5))


### Features

- ZADT-VSP APC handler with RFC domain (ABAP) ([`67e0024`](https://github.com/oisee/vibing-steampunk/commit/67e0024c750c4d6eae89c74067a7e5f8b0d16150))
- ZADT_VSP APC WebSocket handler - RFC domain operational ([`c9109be`](https://github.com/oisee/vibing-steampunk/commit/c9109be2feb84a5bae21155e954997c4470dadfd))
- WebSocket RFC Handler (ZADT_VSP) with embedded ABAP source ([`d36b1d6`](https://github.com/oisee/vibing-steampunk/commit/d36b1d6197154f38c97d33411c9ea3635f54e479))
- Add debug domain to WebSocket handler (ZADT_VSP) ([`307d231`](https://github.com/oisee/vibing-steampunk/commit/307d23194918472feed5006c1d7340310a3c1d53))
- Full WebSocket debugging with TPDAPI integration (v2.0.0) ([`fa4ada8`](https://github.com/oisee/vibing-steampunk/commit/fa4ada8b49c3ea504bb824abfa49ebab8a335b86))
- TPDAPI breakpoint integration verified working (v2.0.1) ([`64050c6`](https://github.com/oisee/vibing-steampunk/commit/64050c600b2a793f2082ca25b7b8b35a75f9afd3))
- Add call graph traversal and RCA tools ([`d8e3742`](https://github.com/oisee/vibing-steampunk/commit/d8e3742e3544c665b4c70386647a3fa12d3c5140))



## [2.12.6] - 2025-12-10
### Features

- EditSource support for class includes (testclasses, locals) ([`3782380`](https://github.com/oisee/vibing-steampunk/commit/3782380101b3ba2edc155896c97ee580e40c786d))



## [2.12.5] - 2025-12-09
### Bug Fixes

- Normalize line endings in EditSource (CRLF → LF) ([`fafbccf`](https://github.com/oisee/vibing-steampunk/commit/fafbccf304283dd44a698e26c987a3d8bd6214d7))



## [2.12.4] - 2025-12-09
### Features

- V2.12.4 - Feature Detection & Safety Network ([`0d5693d`](https://github.com/oisee/vibing-steampunk/commit/0d5693d279e31e4f85c29d88584aa2b4300d9b04))



## [2.12.3] - 2025-12-08
### Bug Fixes

- Properly detect 404 in DeployFromFile for class includes ([`d489743`](https://github.com/oisee/vibing-steampunk/commit/d489743dd965741466251447f47f54883c69f9d1))


### Features

- Auto-reconnect on SAP session timeout ([`610bfeb`](https://github.com/oisee/vibing-steampunk/commit/610bfeb36e7680cbe977beee78707fc7dd634cd7))



## [2.12.2] - 2025-12-08
### Bug Fixes

- Extract class name from filename for class includes ([`85fb919`](https://github.com/oisee/vibing-steampunk/commit/85fb919e58b12a00d875b6d592c4891c373b3169))



## [2.12.1] - 2025-12-07
### Features

- Add CreatePackage tool to focused mode ([`7452c48`](https://github.com/oisee/vibing-steampunk/commit/7452c484151fbfb3f57ca8d1dc79a7790ffb471b))



## [2.12.0] - 2025-12-07
### Features

- **amdp:** Enhance breakpoint functionality and testing ([`76ca83b`](https://github.com/oisee/vibing-steampunk/commit/76ca83b539c1824f86b22f64abb29c6d5d78406e))
- V2.12.0 - abapGit-compatible format & batch operations ([`c731e2e`](https://github.com/oisee/vibing-steampunk/commit/c731e2e8a13670bc0cc318a328d8b618978c8f0f))



## [1.5.0] - 2025-12-03
### Features

- Enhance tool descriptions with usage examples and workflows ([`c52bd4f`](https://github.com/oisee/vibing-steampunk/commit/c52bd4fe2d4d0027281a8e89d3afbdf7555d272a))



## [1.4.1] - 2025-12-03
### Bug Fixes

- Add missing SaveToFile and RenameObject MCP tool registrations ([`67a5f1a`](https://github.com/oisee/vibing-steampunk/commit/67a5f1a061a0863cfff132f158039f93ac05cd4d))



## [1.4.0] - 2025-12-02
### Features

- Add file-based deployment tools solving token limit problem ([`dc6b541`](https://github.com/oisee/vibing-steampunk/commit/dc6b541ae7e133169bb6fa741c38a0f63c787d43))



## [1.3.0] - 2025-12-02
### Features

- Add comprehensive research report on ABAP debugging and tracing capabilities ([`0a1bb1e`](https://github.com/oisee/vibing-steampunk/commit/0a1bb1ef3d633e11598dce065a80f69fb662a4e6))
- Add roadmap section with ongoing and planned features for debugging and analysis tools ([`b6c08db`](https://github.com/oisee/vibing-steampunk/commit/b6c08db98cdccbba75b4c3bbc4252224c514ab24))



## [1.1.0] - 2025-12-02
### Features

- **adt:** Implement workflows for writing and creating ABAP programs and classes ([`cdf3f98`](https://github.com/oisee/vibing-steampunk/commit/cdf3f98d401f2d571b93742c9e3755cd6027d9a7))




