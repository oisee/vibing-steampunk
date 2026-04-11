# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.38.1] - 2026-04-07
### Bug Fixes

- Add progress indicators to package-level health command ([`8306218`](https://github.com/oisee/vibing-steampunk/commit/830621855e7998516905e31100d0ec38668045c5))



## [2.38.0] - 2026-04-07
### Bug Fixes

- Detect dynamic PERFORM IN PROGRAM (variable) calls ([`1d88127`](https://github.com/oisee/vibing-steampunk/commit/1d88127d02c76949ce46c842079eefde01bef94a))
- Preserve auth headers on redirects (#90) + stateful lock sessions (#88) ([`27f4d7c`](https://github.com/oisee/vibing-steampunk/commit/27f4d7c071883ac8d38c06a30a4f922009053262))
- Slim reverse ref queries — ADT freestyle doesn't support OR with LIKE ([`f17cf04`](https://github.com/oisee/vibing-steampunk/commit/f17cf04b3cc7367ea3a839c990419d76f31c85b5))
- Skip local-only JS and TS transpile fixtures in CI ([`b20293e`](https://github.com/oisee/vibing-steampunk/commit/b20293e641362dd424e9e4125c5000a826bd5f56))
- GoReleaser v2.15 dropped changelog.use git-cliff ([`668b66a`](https://github.com/oisee/vibing-steampunk/commit/668b66adece571a37a73c58ea9432d831496f76c))
- Goreleaser v2 uses changelog.disable not changelog.skip ([`a08f451`](https://github.com/oisee/vibing-steampunk/commit/a08f451c460b662ebfc6b1d0b718da6008cb1a16))
- Gitignore RELEASE_NOTES.md to avoid goreleaser dirty state ([`124bdb3`](https://github.com/oisee/vibing-steampunk/commit/124bdb347cdc8b8dc12a51fd1992ebf51f4a1b1b))


### Features

- Add graph knowledge MVP with CLI and MCP queries ([`fcb9efa`](https://github.com/oisee/vibing-steampunk/commit/fcb9efa10c49afef5bda52f1a09789619f436158))
- Add where-used-config analysis in CLI and MCP ([`562b5f8`](https://github.com/oisee/vibing-steampunk/commit/562b5f87105dfa69330c5c8e87d0f37842202472))
- Add mermaid and html graph exports ([`f613c01`](https://github.com/oisee/vibing-steampunk/commit/f613c014a4fbd6de9c93d12c71cdcb89f227a3b8))
- Augment impact analysis with parser overlay ([`19c9e88`](https://github.com/oisee/vibing-steampunk/commit/19c9e88502ea4b99c768971bc5d7288a86de3ba8))
- Add usage examples analysis in CLI and MCP ([`b74e7a8`](https://github.com/oisee/vibing-steampunk/commit/b74e7a8327099b52b2ea7c9c81b69b96a49108b8))
- Add health analysis in MCP and CLI ([`74efe5e`](https://github.com/oisee/vibing-steampunk/commit/74efe5ea868aa3532d6ffd27efd11ebbd1e4ed5d))
- Add fast mode for package health ([`1d4e0f4`](https://github.com/oisee/vibing-steampunk/commit/1d4e0f43db4bc18e3a2212d56ff411240e3d39ed))
- Add api surface inventory for custom packages ([`aa5aa5b`](https://github.com/oisee/vibing-steampunk/commit/aa5aa5bd0ee261c81a6caa85f60a2697d5b856c6))
- Add slim dead-code candidate analysis ([`7027b83`](https://github.com/oisee/vibing-steampunk/commit/7027b83ce420290f1f4b43dca6263f10139b0156))
- Add rename preview analysis ([`dcaa358`](https://github.com/oisee/vibing-steampunk/commit/dcaa358fe7c4dbbb9fe7962d33f29d492855010b))
- Add class sections reader ([`5293d2c`](https://github.com/oisee/vibing-steampunk/commit/5293d2c6ddc1ab0c06ac649cfc86eab3029f082e))
- Add method signature reader ([`79643b6`](https://github.com/oisee/vibing-steampunk/commit/79643b6501e2802130550bfd52b1e4dd65776149))
- Slim V2 — hierarchical scope + internal/external ref classification ([`54c9b5f`](https://github.com/oisee/vibing-steampunk/commit/54c9b5f287d0f95994709c0fa16bded7b99aedcd))
- Slim V2 TDEVC hierarchy resolution + prefix fallback ([`ba11028`](https://github.com/oisee/vibing-steampunk/commit/ba1102816cf9c6a14a3d8953262af109e98a4a4d))
- Slim V2 Phase 3 — method-level dead code + --level flag ([`1ecafe7`](https://github.com/oisee/vibing-steampunk/commit/1ecafe76ba20597a6f3b37f88b113d18741a5850))
- Add AnalyzeABAPCode — abaplint-based static analysis (from PR #89) ([`8623acd`](https://github.com/oisee/vibing-steampunk/commit/8623acdb9aff6a23b102775aa571d2a9df5808e0))
- Health MVP — E070 transport fallback for staleness signal ([`9ae10f3`](https://github.com/oisee/vibing-steampunk/commit/9ae10f3c709ea5570c547cfe80e6035e1fe495f8))
- Add package changelog and CTS change grouping ([`8194cc4`](https://github.com/oisee/vibing-steampunk/commit/8194cc4e2b42eb7a7f0c5c47e358993e902afe47))



## [2.37.0] - 2026-04-05
### Features

- Add graph engine with package boundary analysis, dynamic call detection, and improved help ([`b661c09`](https://github.com/oisee/vibing-steampunk/commit/b661c09f7840964da6fabdcbe3f9dbd5b0ea1733))



## [2.36.0] - 2026-04-05
### Features

- Upgrade mcp-go v0.17.0 → v0.47.0, add Streamable HTTP transport (closes #21) ([`daedc99`](https://github.com/oisee/vibing-steampunk/commit/daedc99dfb8d1715e3b295a035a74d70773a6db2))
- Add browser-based SSO authentication and session keep-alive (from PR #77) ([`e986577`](https://github.com/oisee/vibing-steampunk/commit/e9865772e74f17a11bf0c0c39959427358312654))



## [2.35.0] - 2026-04-05
### Features

- Add GetAPIReleaseState for S/4HANA Clean Core checks (closes PR #53) ([`7270ad7`](https://github.com/oisee/vibing-steampunk/commit/7270ad730d75b930056a1f43d509d56fea9a043f))
- Add 10 gCTS tools for git-enabled CTS (from PR #41, closes #39) ([`81cce41`](https://github.com/oisee/vibing-steampunk/commit/81cce4105117321de662765ce89aa360620f5673))
- Add 7 i18n/translation tools with per-request language override (from PR #42, closes #40) ([`566f1f7`](https://github.com/oisee/vibing-steampunk/commit/566f1f73627df3b59db1c93f2c67424b047a9e89))



## [2.34.0] - 2026-04-04
### Bug Fixes

- WebSocket auth fallback for SAP systems rejecting standalone Basic Auth ([`03e89f3`](https://github.com/oisee/vibing-steampunk/commit/03e89f3379067c8488a218953de4834d95476845))
- Stateless session default, installer resilience, graph namespace ([`d84db03`](https://github.com/oisee/vibing-steampunk/commit/d84db03887c7bfc58719955a40c5ecae4c47515b))
- Resolve all test failures (dsl fmt.Errorf, jseval oracle escaping) ([`8429e28`](https://github.com/oisee/vibing-steampunk/commit/8429e28b28c74c27527e585fd05e34ab59bb54cf))


### Features

- Add offset and columns_only to GetTableContents (closes #34) ([`9fb6c8a`](https://github.com/oisee/vibing-steampunk/commit/9fb6c8ab0bc346669a0ba3ea934c861a8b1f845f))
- Add version history tools (3 tools, 8 tests) ([`dd06202`](https://github.com/oisee/vibing-steampunk/commit/dd06202d88e2a8148887c0c123d07b067dd29256))
- Add CDS impact analysis and element info tools (from PR #85) ([`6c67140`](https://github.com/oisee/vibing-steampunk/commit/6c67140a48ed14a436d0b53daeb5ed974d13b490))
- Add GetCodeCoverage and GetCheckRunResults tools (from PR #84) ([`333f462`](https://github.com/oisee/vibing-steampunk/commit/333f4625d0f3aab8047292c43f4e607f1b16de49))



## [2.33.0] - 2026-04-03
### Bug Fixes

- QuickJS compilation — xstring write, include naming, graceful parse ([`3ee62c1`](https://github.com/oisee/vibing-steampunk/commit/3ee62c1e9da63ad4feaf787975754fcfe1dc4fd2))
- ABAP codegen — xstring types, FORM WASM_INIT, DATA/init separation ([`01c5d48`](https://github.com/oisee/vibing-steampunk/commit/01c5d48c9d06a112bb2b95adca86e0c44b780839))
- ABAP codegen — ty_x4 type for bitwise ops, no CONV x4 ([`0a05d2e`](https://github.com/oisee/vibing-steampunk/commit/0a05d2e76ed5c6322892ae169bc08db9098fc68f))
- GENERATE SUBROUTINE POOL recursion via manual call stack ([`7b60285`](https://github.com/oisee/vibing-steampunk/commit/7b602858fcac5a83d6d70721e403388469249c8d))
- Unique local names per function, indent-aware packing ([`d2f335c`](https://github.com/oisee/vibing-steampunk/commit/d2f335cbf13b1380602d8dc80fb0aee4c1ddb1b8))
- Discard partially-parsed function bodies, indent-aware packing ([`c92baa0`](https://github.com/oisee/vibing-steampunk/commit/c92baa04823a6dd4aa522ac5260be888b3b7940f))
- Dead code elimination, RETURN non-packable, block-level gv_br skip ([`b7306cf`](https://github.com/oisee/vibing-steampunk/commit/b7306cff2aa4d2c5389fff615531092303dd5356))
- Emit FORM line via emit_raw_line, RETURN non-packable, guard rv=0 ([`8a3ff0d`](https://github.com/oisee/vibing-steampunk/commit/8a3ff0d727679b456ca4accafc1cc19725056720))
- Proper dead code nesting, indent-based discard, stack underflow guard ([`7bd01e1`](https://github.com/oisee/vibing-steampunk/commit/7bd01e1833f57b1f66cc689d72e27668cb8351f1))
- Imported memory allocation, bounds checks, runtime debugging ([`980fb4e`](https://github.com/oisee/vibing-steampunk/commit/980fb4ec811feb20cadbc06f2c1e0ad971ce577c))
- Mem_st_i32 TYPE x conversion, imported memory, bounds checks ([`0ff95b5`](https://github.com/oisee/vibing-steampunk/commit/0ff95b5a04f6b07b3a84abf44434387a3479e9ea))
- Simplify QuickJS test — GENERATE success is the assertion ([`ecc61d0`](https://github.com/oisee/vibing-steampunk/commit/ecc61d0a55e0640cf1a858cd755c7a134128c97c))
- IF/ELSE stack depth bug in codegen, add wazero execution tests ([`50ebc3b`](https://github.com/oisee/vibing-steampunk/commit/50ebc3b13b8c09a1a030c494f011dc6c6973b006))
- ABAP codegen packer bugs — emit_raw_line for DO/METHOD/ENDMETHOD ([`e4b38ef`](https://github.com/oisee/vibing-steampunk/commit/e4b38efe63233db2876510444634fc737d93010a))
- ABAP codegen — ELSE guard, local index scan, max vars fix ([`d28c653`](https://github.com/oisee/vibing-steampunk/commit/d28c6539cc6fddbe12a4ffe11f69fe750f9b1f11))
- QuickJS GENERATE SUBROUTINE POOL succeeds — rc=0 on SAP! ([`c57dd7e`](https://github.com/oisee/vibing-steampunk/commit/c57dd7eed0ea6c03d38ad621580ae884cc671dba))
- Phi node resolution in LLVM IR → ABAP compiler ([`3f463a0`](https://github.com/oisee/vibing-steampunk/commit/3f463a062818ccdc25acd62b6d9a4e909907509c))
- Tail call intrinsics (abs/smax/smin) + regenerate test program ([`5cde0e1`](https://github.com/oisee/vibing-steampunk/commit/5cde0e1e2d520f7a6578118dd856e63688677c82))
- DATA line splitting + LLVM intrinsics for QuickJS ([`d7e4b17`](https://github.com/oisee/vibing-steampunk/commit/d7e4b17127394bc02f1a3edb21a3acb2b65433fc))
- REPORT header + empty TYPE guard in LLVM→ABAP ([`0672191`](https://github.com/oisee/vibing-steampunk/commit/0672191ef84f6317ddc161f46a2da12f0bb76425))
- Indirect calls (function pointers) — 1949 empty calls → 0 ([`989e13f`](https://github.com/oisee/vibing-steampunk/commit/989e13f57ec645da94e518bb29549c773f75012b))
- Null→0, TYPES x4 declaration for ABAP compatibility ([`b16206a`](https://github.com/oisee/vibing-steampunk/commit/b16206adf2018e04aca14206f23669841fd59e32))
- BIT-AND type, @globals, null, dereferenceable attrs ([`dbf53a9`](https://github.com/oisee/vibing-steampunk/commit/dbf53a985e71519658505d1a6e50a6cbed31e252))
- Transpiler compat — no inline DATA, type-aware dispatch, libc stubs ([`dc8aa76`](https://github.com/oisee/vibing-steampunk/commit/dc8aa761075c13661b1b66f0c7a3d35219a199f5))
- DATA declarations max 5 per line for transpiler compat ([`6c5ad56`](https://github.com/oisee/vibing-steampunk/commit/6c5ad56cf357e6cb27b84a9f2aba12a93e143df2))
- Byval/sret attrs, more C stubs, scientific notation ([`de4531f`](https://github.com/oisee/vibing-steampunk/commit/de4531f08276972df4eca5c8bd200120b8587d6f))
- CONV #() all args + auto-stub externals + hex constants ([`2fb8ab3`](https://github.com/oisee/vibing-steampunk/commit/2fb8ab3bd7585915bd68aa83adc9993f2fc29e32))
- Jseval parser token-kind checks + ABAP space handling ([`5ed8cb3`](https://github.com/oisee/vibing-steampunk/commit/5ed8cb31b2dd35d49826bb65e12f5833cfc20e8d))
- Jseval NodeBool — typeof true now returns "boolean" ([`43315ba`](https://github.com/oisee/vibing-steampunk/commit/43315baa87eef87ad58333684a71d640721de248))
- --verbose flag now works for all CLI subcommands ([`4143190`](https://github.com/oisee/vibing-steampunk/commit/4143190c1bfc52a06cbcda1130cb0adfb0b4e7f7))


### Features

- Add `make release` and `make refresh-deps` targets ([`efdc744`](https://github.com/oisee/vibing-steampunk/commit/efdc744b0c8e72b7a2cbc470beba934a9cffd790))
- WASM test binaries, ABAP codegen implementation, interactive compiler report ([`480537b`](https://github.com/oisee/vibing-steampunk/commit/480537bf0578e37a2e761c6979d3681af32af467))
- QuickJS WASM binary, persistent program generation, class wrapper ([`9c6d27c`](https://github.com/oisee/vibing-steampunk/commit/9c6d27c1e838aa4395e8ba728e60ae37825b43d7))
- Smart DATA declarations, USING VALUE(), split to INCLUDEs ([`64b9708`](https://github.com/oisee/vibing-steampunk/commit/64b970859e13a20ce05dab063abb9843a95fbf82))
- QuickJS GENERATE progress — uniform int8, void return, MESSAGE X ([`9bff122`](https://github.com/oisee/vibing-steampunk/commit/9bff122d059d651e0dea0719115429345448c9a4))
- ABAP codegen — line packing, LEB128 fix, block closure, WASI stubs ([`669bf4d`](https://github.com/oisee/vibing-steampunk/commit/669bf4dc8444503bd3b896e91a85aa537a131c6e))
- Eliminate DO 1 TIMES for blocks, fix QuickJS GENERATE ([`ddfff69`](https://github.com/oisee/vibing-steampunk/commit/ddfff69c75f1d9c2734ff894f9c8b9e12485eb4f))
- QuickJS WASM executes on SAP — 7/7 tests pass! ([`2f41659`](https://github.com/oisee/vibing-steampunk/commit/2f416595961c6f4dca291a3a2316b66e513f2129))
- Parse coverage 217K→453K instructions, overflow-safe LEB128 ([`6efd8ef`](https://github.com/oisee/vibing-steampunk/commit/6efd8ef99758f1cf9e2544804d5f74fd3c279a39))
- WASI fd_write implementation for console output ([`01e7d8c`](https://github.com/oisee/vibing-steampunk/commit/01e7d8cab7cd8f707c84c82a6a98955e3467d184))
- Complete WASI stubs for all 9 QuickJS imports ([`5c7209a`](https://github.com/oisee/vibing-steampunk/commit/5c7209a8b99e82a047def283f7b5f3f1002225c2))
- Block-as-CLASS-METHOD codegen for WASM-to-ABAP compiler ([`5f2e448`](https://github.com/oisee/vibing-steampunk/commit/5f2e448384964826a52178025a37a3681c11ff8f))
- LLVM IR → ABAP compiler — typed CLASS-METHODS from C/Rust ([`3536edb`](https://github.com/oisee/vibing-steampunk/commit/3536edb60d4ec4bcc1b494dda41c80ba1492098e))
- Struct/GEP + load/store + zext/sext in LLVM IR → ABAP ([`f2632f7`](https://github.com/oisee/vibing-steampunk/commit/f2632f7350f42f8e5fe633ad9d29b0ccdabc8264))
- Alloca/switch/freeze + FatFS compiles — 28 functions, 0 TODOs ([`acdcd19`](https://github.com/oisee/vibing-steampunk/commit/acdcd191b8430a652be484b07ba6bb60298422e3))
- QuickJS C→LLVM→ABAP: 537 functions, 121K lines, 0 TODOs ([`a39034a`](https://github.com/oisee/vibing-steampunk/commit/a39034a8d881f97b47996e94b186a749f0bbffd2))
- Generated ABAP test program for SAP deployment ([`c62509a`](https://github.com/oisee/vibing-steampunk/commit/c62509a809bd01a4010da94772ce0fa2b4dc179d))
- Abapgit-pack CLI — create abapGit ZIP from ABAP sources ([`d613791`](https://github.com/oisee/vibing-steampunk/commit/d61379110838b48e99bc9569c3e6e3f773abb574))
- AbapGit ZIPs — compiled ABAP ready for SAP import ([`092d5d1`](https://github.com/oisee/vibing-steampunk/commit/092d5d136fcc845b4f69fcd6871f95a23dfdbdac))
- Fun/ — hands-on experiments with vsp compilers ([`d373243`](https://github.com/oisee/vibing-steampunk/commit/d373243a2bf5daaabeb24c40094afc743f913254))
- Vsp compile llvm — C/LLVM IR to ABAP in one command ([`01629d0`](https://github.com/oisee/vibing-steampunk/commit/01629d0655ec60e5171309458de247bad6321801))
- Make install-user — install vsp to ~/.local/bin ([`649c0ef`](https://github.com/oisee/vibing-steampunk/commit/649c0ef3601ae53644200a5a7f40b63a175609cd))
- Vsp compile llvm --cflags for extra clang flags ([`abba890`](https://github.com/oisee/vibing-steampunk/commit/abba890f2f3bc33113395b89447fe58ebd92d083))
- Updated quickjs_llvm.zip — fresh build with clang 14 ([`0cef1e0`](https://github.com/oisee/vibing-steampunk/commit/0cef1e00b8b5efcc4c4baf83328cf3f249fc5bd7))
- Function pointer dispatch via CASE trampoline ([`c6aabc0`](https://github.com/oisee/vibing-steampunk/commit/c6aabc054c9df7f378bf13d4a9a3f666f4744197))
- Memory runtime + zext nneg fix + mini VM test corpus ([`6d4c6b6`](https://github.com/oisee/vibing-steampunk/commit/6d4c6b6d6f4546dcfa371529bcd3f334210a7d21))
- Auto-split large CASE dispatchers (IF/ELSEIF chunks of 12) ([`2145505`](https://github.com/oisee/vibing-steampunk/commit/2145505fb1979891b3d471437b96f3c2c7d47f10))
- Multi-class split + memory class for transpiler compat ([`29a163b`](https://github.com/oisee/vibing-steampunk/commit/29a163b6555c06eaca7097e1c563eb837f0b67a9))
- Pure CLASS-METHODS — no FORMs in multi-class mode ([`a2a6754`](https://github.com/oisee/vibing-steampunk/commit/a2a675446af1f4c9784cf17ee7b72c3c94afb088))
- Pkg/jseval — minimal JavaScript evaluator in pure Go (500 lines) ([`f000c0b`](https://github.com/oisee/vibing-steampunk/commit/f000c0b82751cb91054c9c8a4021d16427eb1632))
- Jseval — objects, arrays, strings, for, typeof, closures, classes ([`46f81ea`](https://github.com/oisee/vibing-steampunk/commit/46f81ea3da0c0d6fc405bb38471bdbf7cba168e1))
- Abaplint lexer runs on our Go JS eval! ([`a721fd6`](https://github.com/oisee/vibing-steampunk/commit/a721fd617b0486d9df329e871ab7c87a3e9561ab))
- Zcl_jseval — JavaScript evaluator in pure ABAP (2200 lines) ([`7664964`](https://github.com/oisee/vibing-steampunk/commit/7664964f7afcf502f18c8d3db4f94218202157d3))
- Jseval — ternary, arrow functions, throw/try/catch, expr calls ([`97aca80`](https://github.com/oisee/vibing-steampunk/commit/97aca80097bf5f666172b88694569ea8e6cab7aa))
- Jseval — for...of, for...in, template literals ([`0d55b54`](https://github.com/oisee/vibing-steampunk/commit/0d55b5423b72941a6811675621ee147128174898))
- Jseval — function expressions, static methods, mini-runtime pattern ([`02c706c`](https://github.com/oisee/vibing-steampunk/commit/02c706cd333e36de0866f678df72427c84fa2441))
- Jseval — nullish coalescing, optional chaining, extends, Error, static ([`041d914`](https://github.com/oisee/vibing-steampunk/commit/041d9142259530f2b4bf901c5ffaacbd90e9e19e))
- Jseval — spread/rest operators, complete open-abap-core feature set ([`f2a89fe`](https://github.com/oisee/vibing-steampunk/commit/f2a89fe479c687554a94bec4b8511b2c281259d0))
- Jseval — new expr.prop(), function constructors, open-abap-core shim ([`54af5cd`](https://github.com/oisee/vibing-steampunk/commit/54af5cdfabc767a386f71a5b8828900029f94118))
- Jseval — constructor return value, transpiled ABAP runs! ([`24e0ff5`](https://github.com/oisee/vibing-steampunk/commit/24e0ff57a599d51fc18d9c8195827e3f136fd75c))



## [2.32.0] - 2026-03-22
### Bug Fixes

- NativeSQL handler for AMDP — statement type match now 100.0% ([`1da0c69`](https://github.com/oisee/vibing-steampunk/commit/1da0c6999b25fff382e9958c02e8064bf4877871))
- Graph command with WBCROSSGT fallback — works on all systems ([`e62fb73`](https://github.com/oisee/vibing-steampunk/commit/e62fb7328a9b84b9c2428fa7e1b3695294ee71ef))
- Use T000 instead of MARA in examples (A4H has no MM module) ([`4265032`](https://github.com/oisee/vibing-steampunk/commit/426503213b9fd2be83884438957fb2696af09fd2))


### Features

- Parse_abap + analyze_deps MCP tools — ABAP parser as MCP service ([`0756e94`](https://github.com/oisee/vibing-steampunk/commit/0756e9433ade151cbf55e597d8605c0f873a3b74))
- Native Go ABAP lexer (abaplint port) + context depth expansion ([`5c875a5`](https://github.com/oisee/vibing-steampunk/commit/5c875a52acfb5946f148aa21f85eb234766964ca))
- Statement parser + combinator DSL + type matcher (99.97% oracle match) ([`f897d57`](https://github.com/oisee/vibing-steampunk/commit/f897d574d241b1ff67934a352ff03e1b53875fde))
- Ts2go transpiler — TypeScript AST to Go code generator ([`c2abd6f`](https://github.com/oisee/vibing-steampunk/commit/c2abd6f57d54ef4e4220b7672996e84e9315b59a))
- Ts2go produces valid Go from abaplint lexer (383 lines, 3 files) ([`c313e7c`](https://github.com/oisee/vibing-steampunk/commit/c313e7c04e9a0051ab81b39769809c0a81a58f8d))
- ABAP linter with 8 rules — 864 issues on real corpus, 795μs/file ([`474c90f`](https://github.com/oisee/vibing-steampunk/commit/474c90fa1a3c63c44555205feffc7b9797cb8455))
- Linter oracle differential — 100% match on 4 rules, 29 files ([`ef4ebd1`](https://github.com/oisee/vibing-steampunk/commit/ef4ebd16abe1f9713f2949f03648ebb7f8ff2459))
- WASM test suite — 5 functions, 22 test cases, 226 bytes ([`8f5285f`](https://github.com/oisee/vibing-steampunk/commit/8f5285f7d153684e1fe9c9b52001c700587264ed))
- WASM self-host test on SAP — 3/5 functions pass (add, factorial, is_prime) ([`5d0d900`](https://github.com/oisee/vibing-steampunk/commit/5d0d900220cd4d484c8336f0cac6deaee61386ee))
- WASM self-host compiler 5/5 tests PASS on SAP A4H ([`467d323`](https://github.com/oisee/vibing-steampunk/commit/467d323ece6424def826b742d2a402b9ef00c9db))
- WASM self-host 11/11 — synced codegen fixes from SAP ([`49f2275`](https://github.com/oisee/vibing-steampunk/commit/49f2275c61b9535a64be7610752395d439a28108))
- CLI surface — query, grep, system info, lint, execute commands ([`bab0415`](https://github.com/oisee/vibing-steampunk/commit/bab0415d6ab433bb5fd20f0735515d8439bda80f))
- V2.32.0 — CLI toolchain, WASM 11/11 verified, ABAP linter ([`b2014f3`](https://github.com/oisee/vibing-steampunk/commit/b2014f3b4ae15405e1f2047d1a46d2895fc0b597))
- Graph CLI + context --depth + updated docs ([`2fdea48`](https://github.com/oisee/vibing-steampunk/commit/2fdea48a0179a98155078b941e9f6deb027dc86b))
- Graph supports CLAS, PROG, FUGR, TRAN + dual CROSS/WBCROSSGT query ([`558a300`](https://github.com/oisee/vibing-steampunk/commit/558a30007ac26863657454e2b3912c4b27575df3))
- Vsp deps — package dependency analysis + transport readiness ([`ba83e22`](https://github.com/oisee/vibing-steampunk/commit/ba83e22dbc07143b631d971f49ef1c6d1f8ef970))
- Lua bindings for query/lint/parse/context + showcase scripts ([`c67dfe6`](https://github.com/oisee/vibing-steampunk/commit/c67dfe62e67c0f1d1e1f095e658b5a0f2c191168))



## [2.30.0] - 2026-03-20
### Bug Fixes

- Add node_modules to gitignore ([`7bf4af9`](https://github.com/oisee/vibing-steampunk/commit/7bf4af95afefc56bf9fda031f05a7feec96eea98))
- Abaplint lexer space comparison — ABAP string trimming gotcha ([`0ee157d`](https://github.com/oisee/vibing-steampunk/commit/0ee157dd53840b3bb03ce3a2b4ff4b457b681a4e))


### Features

- WASM-to-ABAP AOT compiler prototype (pkg/wasmcomp) ([`149233c`](https://github.com/oisee/vibing-steampunk/commit/149233c93d0ce74de8fa273364f12c2e40827b3c))
- **wasmcomp:** Fix control flow, return values, add i64/f64/call_indirect ([`ca6dd6b`](https://github.com/oisee/vibing-steampunk/commit/ca6dd6bf40a7774df6c12b75dbfa43a207975248))
- **wasmcomp:** QuickJS WASM compilation — 1,410 functions, 99.8% opcodes ([`ca5290e`](https://github.com/oisee/vibing-steampunk/commit/ca5290eb3fe36a3ebfae46fa98fca31016871bd0))
- **wasmcomp:** 100% opcode coverage for QuickJS WASM ([`2c696ae`](https://github.com/oisee/vibing-steampunk/commit/2c696ae6c635aa620973332c6b497f5dbe881869))
- **wasmcomp:** Multi-class splitting, dedup pass, runtime class ([`971c6a7`](https://github.com/oisee/vibing-steampunk/commit/971c6a749f1b7716b23c92e32b70dd3a7d85758b))
- **wasmcomp:** Three backends (FUGR, Class, Hybrid) + WASI shim ([`0bd74ec`](https://github.com/oisee/vibing-steampunk/commit/0bd74ecacb6bf36f898a876e1ee4d10bc4908db3))
- **wasmcomp:** Line packing — 2.86x line count reduction ([`57cc729`](https://github.com/oisee/vibing-steampunk/commit/57cc7294ed863de89723d5c32cf63bfdcc944a80))
- **wasmcomp:** Pack DATA declarations + code together ([`959c7ed`](https://github.com/oisee/vibing-steampunk/commit/959c7ed42a4938e9b29f4fb8b5c7f55cf0b35e19))
- **wasmcomp:** Aggressive line packing — 5.45x total reduction ([`ab1d416`](https://github.com/oisee/vibing-steampunk/commit/ab1d41632091074ff6964f2da9cb701d7b2d4d29))
- **wasmcomp:** Chained DATA declarations ([`5719ce5`](https://github.com/oisee/vibing-steampunk/commit/5719ce594b17c9a456fb77395cad98830cb14d2e))
- **wasmcomp:** Compile abaplint parser to ABAP — 396K lines ([`21a3998`](https://github.com/oisee/vibing-steampunk/commit/21a39983d9c9a4c2657bcfdbde6c3d0ab33d7e45))
- TS→ABAP direct transpiler prototype (pkg/ts2abap) ([`ade95e1`](https://github.com/oisee/vibing-steampunk/commit/ade95e17a7d836dc709ecc7172040e793e82d043))
- Abaplint lexer running on SAP — transpiled from TypeScript ([`1cdd800`](https://github.com/oisee/vibing-steampunk/commit/1cdd800ca4efef6b70ecfc4f209f29d867a4f7c9))
- Native ABAP WASM parser running on SAP — Phase 1 complete ([`e35e292`](https://github.com/oisee/vibing-steampunk/commit/e35e292b3a4d912f97f857763641eaeecf7dcf0d))
- SELF-HOSTING WASM compiler on SAP — parse, compile, execute! ([`0de3867`](https://github.com/oisee/vibing-steampunk/commit/0de3867b9843b6e1271aa693eceaf4e79142362a))
- Export native ABAP WASM compiler from SAP — 785 lines ([`39958a6`](https://github.com/oisee/vibing-steampunk/commit/39958a6d89f5fe954c1af355d77a89b0024b0d11))
- Statement parser on SAP — splits tokens into statements with chaining ([`8760fdd`](https://github.com/oisee/vibing-steampunk/commit/8760fdd2dd4ce4566117fff6f787c0dcb5aff9a5))
- Unified 5-layer code intelligence analyzer ([`0c2bace`](https://github.com/oisee/vibing-steampunk/commit/0c2bace3aa6f5b80ccdda8079723544e3791dc99))
- Parser-primary confidence model, CROSS staleness documented ([`7769367`](https://github.com/oisee/vibing-steampunk/commit/77693679436663c07b59a7ac9fb659f63f272dc8))



## [2.29.0] - 2026-03-19
### Bug Fixes

- Add missing `items` to DebuggerGetVariables array schema (#24) (#25) ([`9a7eebe`](https://github.com/oisee/vibing-steampunk/commit/9a7eebe9e31fe11abc03d0cfd799cb4dd7ee907b))
- Auto-retry on 401 Unauthorized after idle timeout (#35) ([`d73460a`](https://github.com/oisee/vibing-steampunk/commit/d73460ade7035903fe638b4caf0500c64ef2a776))
- CreatePackage safety check uses package name being created (#71) ([`2ef8c3e`](https://github.com/oisee/vibing-steampunk/commit/2ef8c3e067979337f99c8cc0e22eb4baa71c2638))
- Install tools bypass SAP_ALLOWED_PACKAGES restrictions (#54) ([`512996c`](https://github.com/oisee/vibing-steampunk/commit/512996c12eda4fb041beb7877075f8e6953bcad1))
- SyntaxCheck uses shorter object URI for long namespaced classes (#52) ([`6d1f00a`](https://github.com/oisee/vibing-steampunk/commit/6d1f00aad1d75c69a6f909190aa313fbef80e930))
- CreateTransport uses S/4HANA 757 compatible endpoint and format (#70) ([`ca02f47`](https://github.com/oisee/vibing-steampunk/commit/ca02f47f656749aa7a002f639078cc6f278a1764))
- Fix references to zadt_cl_tadir_move, now zcl_vsp_tadir_mov ([`751ab10`](https://github.com/oisee/vibing-steampunk/commit/751ab104659437a1ae7ca5b545d10383136ed62c))
- Add --parent, --include, --method flags to CLI source command ([`7dc7a82`](https://github.com/oisee/vibing-steampunk/commit/7dc7a82959d464446110ea82463cc69999415c27))


### Features

- Add GetDependencyZIP function and tests for dependency retrieval (#60) ([`5317105`](https://github.com/oisee/vibing-steampunk/commit/531710515c939cf6e3dbe8d67a5a79e4a07e033a))
- Context compression — GetSource auto-appends dependency contracts (v2.28.0) ([`9fde5d8`](https://github.com/oisee/vibing-steampunk/commit/9fde5d8801a43ac4c3660273a0b615f219bf0dcd))
- Add ignore_warnings parameter to EditSource ([`7fbfbba`](https://github.com/oisee/vibing-steampunk/commit/7fbfbba8be6b80680f904f9158437dfac3d45492))
- Strategic decomposition, one-tool mode, and CLI DevOps surface ([`def027a`](https://github.com/oisee/vibing-steampunk/commit/def027ac379b9d613801bd2cf78669ce2640fcd8))
- Unify SAP_MODE with hyperfocused (one-tool) mode ([`5c942b9`](https://github.com/oisee/vibing-steampunk/commit/5c942b9358fbbf80f8d54ddb0fe0be4cb15de2e4))



## [2.27.0] - 2026-03-01
### Features

- Iterative activation with package filtering + 100 stars article ([`8d2c343`](https://github.com/oisee/vibing-steampunk/commit/8d2c343e50f79f48663418568deade412337cd03))
- ABAP LSP server with online diagnostics and go-to-definition ([`6b801df`](https://github.com/oisee/vibing-steampunk/commit/6b801df0f06fad76cb0fb0563e6f0c00c8796e36))



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




