# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
- vsp config init/show commands ([`bf90c25`](https://github.com/oisee/vibing-steampunk/commit/bf90c25b983caa4a7879112c887e65f7412467d1))
- vsp config mcp-to-vsp and vsp-to-mcp commands ([`717cd9a`](https://github.com/oisee/vibing-steampunk/commit/717cd9adb8909707c68a28cea1a1f8b954cd539c))
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
- Force Replay - State Injection (Phase 5.5) ([`70fb43f`](https://github.com/oisee/vibing-steampunk/commit/70fb43fe85da3d46759b40ef44321701a044a63d))
- Phase 5 TAS-Style Debugging Complete (v2.15.0) ([`19405b2`](https://github.com/oisee/vibing-steampunk/commit/19405b2a4a13210f8809748d263f80f0524e4a61))



## [2.14.0] - 2025-12-21
### Features

- Lua scripting integration (Phase 5.1) ([`0e5c5c2`](https://github.com/oisee/vibing-steampunk/commit/0e5c5c2681fcca270d21a476139a387dfd73461a))



## [2.13.0] - 2025-12-21
### Bug Fixes

- External debugger breakpoint XML format & unit test parsing ([`296b8f3`](https://github.com/oisee/vibing-steampunk/commit/296b8f31530810440db43eeb5609527bc9ec156c))
- GetDumps Accept header & add WebSocket debugging ADR ([`2eb4a5e`](https://github.com/oisee/vibing-steampunk/commit/2eb4a5efd27241c866bc7a8c6234fa2f6471b7d5))


### Features

- ZADT_VSP APC WebSocket handler - RFC domain operational ([`c9109be`](https://github.com/oisee/vibing-steampunk/commit/c9109be2feb84a5bae21155e954997c4470dadfd))
- WebSocket RFC Handler (ZADT_VSP) with embedded ABAP source ([`d36b1d6`](https://github.com/oisee/vibing-steampunk/commit/d36b1d6197154f38c97d33411c9ea3635f54e479))
- Full WebSocket debugging with TPDAPI integration ([`fa4ada8`](https://github.com/oisee/vibing-steampunk/commit/fa4ada8b49c3ea504bb824abfa49ebab8a335b86))
- Add call graph traversal and RCA tools ([`d8e3742`](https://github.com/oisee/vibing-steampunk/commit/d8e3742e3544c665b4c70386647a3fa12d3c5140))



## [2.12.0] - 2025-12-07
### Features

- abapGit-compatible format & batch operations ([`c731e2e`](https://github.com/oisee/vibing-steampunk/commit/c731e2e8a13670bc0cc318a328d8b618978c8f0f))
- EditSource support for class includes (testclasses, locals) ([`3782380`](https://github.com/oisee/vibing-steampunk/commit/3782380101b3ba2edc155896c97ee580e40c786d))
- Feature Detection & Safety Network ([`0d5693d`](https://github.com/oisee/vibing-steampunk/commit/0d5693d279e31e4f85c29d88584aa2b4300d9b04))
- Auto-reconnect on SAP session timeout ([`610bfeb`](https://github.com/oisee/vibing-steampunk/commit/610bfeb36e7680cbe977beee78707fc7dd634cd7))



## [1.4.0] - 2025-12-02
### Features

- Add file-based deployment tools solving token limit problem ([`dc6b541`](https://github.com/oisee/vibing-steampunk/commit/dc6b541ae7e133169bb6fa741c38a0f63c787d43))



## [1.1.0] - 2025-12-02
### Features

- Implement workflows for writing and creating ABAP programs and classes ([`cdf3f98`](https://github.com/oisee/vibing-steampunk/commit/cdf3f98d401f2d571b93742c9e3755cd6027d9a7))
