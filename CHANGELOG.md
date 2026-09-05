# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.56.0] - 2026-09-05
### Bug Fixes

- **fmtest:** Take a leading FM or FUNC, the way the other commands take a type ([`ac302a2`](https://github.com/oisee/vibing-steampunk/commit/ac302a24869568396150aed6f978118096c0b7ba))


### Features

- **cluster:** --layout names the fields from DD03L, and STXL reads as text ([`18ee30a`](https://github.com/oisee/vibing-steampunk/commit/18ee30a923f4bd7c99ccf401e2947d1856ebe39c))
- **cluster:** Deep data — tables in rows, string objects, DD40L line types ([`fb181bf`](https://github.com/oisee/vibing-steampunk/commit/fb181bf67eca323246c81ee88fb8fa3d2a976427))
- **cluster:** Version 5 clusters, and a decompress command for bare streams ([`04f3a29`](https://github.com/oisee/vibing-steampunk/commit/04f3a292ad1cba19a2ed6c5405dd8abec9f824f9))
- **spool,jobs:** SM37 and SP01 as tables, the TemSe list format decoded ([`79c0f56`](https://github.com/oisee/vibing-steampunk/commit/79c0f56e271ba545d2d7a11aa73a7a27f86e765d))
- **inspect:** Variants, SE37 test data, SE61 documentation and the IMG ([`d6eba35`](https://github.com/oisee/vibing-steampunk/commit/d6eba3588c32b2ec1321d5af1cd3954852271a48))



## [2.55.0] - 2026-09-04
### Bug Fixes

- **query:** --top 0 and MCP all_rows now return every row instead of capping at 100 ([`8a61d02`](https://github.com/oisee/vibing-steampunk/commit/8a61d022e8b7e41b411c4365a5aa9fd14ba08228))
- **mcp:** The dead return that has kept CI red since 25 August ([`51c8c67`](https://github.com/oisee/vibing-steampunk/commit/51c8c67928fa3936ced3caffecd0038d0566bd50))
- **mcp:** Route read for SRVB — advertised but dropped by the switch ([`2432a33`](https://github.com/oisee/vibing-steampunk/commit/2432a33d0c2c9aa85a7f59bbc191c90efb683b70))
- **srvb:** Correct inverted binding_category docs (0=UI, 1=A2X) ([`44dd292`](https://github.com/oisee/vibing-steampunk/commit/44dd292b5b06f8e6467c4dd274094e2634053300))
- **adt:** CSRF HEAD→GET fallback + SAP_SESSION_TYPE env var ([`9303a05`](https://github.com/oisee/vibing-steampunk/commit/9303a05bcc3bdddc8d94f535185dbb70e77cabad))
- Fixed/added client to browser auth ([`68688b9`](https://github.com/oisee/vibing-steampunk/commit/68688b9dd4358d2b892819b08145838e40eb7dc7))
- **install:** Propagate Description + surface !Success in zadt-vsp deploy ([`4bab1ca`](https://github.com/oisee/vibing-steampunk/commit/4bab1ca56c2c880644d439a44295b69e8bc313df))
- **install:** Detect pre-existing package via direct probe ([`ca5f17b`](https://github.com/oisee/vibing-steampunk/commit/ca5f17b04dfc46b5504534b1abd80ff8ac19b168))
- **adt:** A lock handle dies of the request nobody flagged ([`2aff8cf`](https://github.com/oisee/vibing-steampunk/commit/2aff8cff258c812867610c10ef05bafe7fad6945))
- **search:** Pass --type to ADT server-side so --max applies after type filter ([`cdb3dea`](https://github.com/oisee/vibing-steampunk/commit/cdb3dea0bae1c7a73a29663b929d5812984a516a))
- **search:** Add TODO for INCL canonical type pending upstream PR #121 ([`47e8de0`](https://github.com/oisee/vibing-steampunk/commit/47e8de0d7e8658a666ae47f93c91cef74f3148bd))
- **search:** Wire MCP path + move CanonicalObjectType to adt (PR #126 review) ([`ec65287`](https://github.com/oisee/vibing-steampunk/commit/ec652871fd176393dabf931843f446ceb7df4fe0))
- **adt:** Extract INCL name from filename; move SyntaxCheck before Lock ([`48fcf5a`](https://github.com/oisee/vibing-steampunk/commit/48fcf5a92f6f0135c669e32908e3b2be2a3d8d67))
- **adt:** The transport parsers each knew one shape, and neither was the tree ([`6c8c5c7`](https://github.com/oisee/vibing-steampunk/commit/6c8c5c7e6132b25787b0b28d07d4a5ce075126d7))
- **adt:** An activation that failed could still report success ([`584ce45`](https://github.com/oisee/vibing-steampunk/commit/584ce45567a8a0f5b41b1639d17b007131299005))
- **adt:** Report the compensating unlock instead of dropping it ([`f65087d`](https://github.com/oisee/vibing-steampunk/commit/f65087d24ddce0687c4a00b4f0bcd43a0419f743))
- **cache:** The shipped SQLite cache was a stub, and the local one was broken ([`4931e8d`](https://github.com/oisee/vibing-steampunk/commit/4931e8dab1a56d588b0a3084db90dfb20851308a))
- **adt:** The keep-alive kept the session by ending it ([`e62e359`](https://github.com/oisee/vibing-steampunk/commit/e62e359a951f35706dd3ada32f3a4335d7a0eaa4))
- **adt:** A syntax warning is reported, not enforced ([`b2a9cd6`](https://github.com/oisee/vibing-steampunk/commit/b2a9cd6458ce21d56a2faa9de64c40bb0a211979))
- **adt:** Fail closed on logical mutation results ([`4610a03`](https://github.com/oisee/vibing-steampunk/commit/4610a0318457eefd18f9e1202a8964158a633122))
- **lua:** Surface writeSource failures and options ([`4aeeb26`](https://github.com/oisee/vibing-steampunk/commit/4aeeb2693fbc148901a2db0075eb88bfef303e2b))
- **install:** Verify packages and deployed sources ([`0c87aee`](https://github.com/oisee/vibing-steampunk/commit/0c87aee942fc8eea2fe064a28f0b47a45f6ca239))
- **copy:** Reject incomplete deployments ([`6b63809`](https://github.com/oisee/vibing-steampunk/commit/6b63809fd2ccf147e9bcb89d712ea55f136fdfba))
- **mcp:** Fail closed without discarding the diagnosis ([`f0544e2`](https://github.com/oisee/vibing-steampunk/commit/f0544e2016207e3c0adb75173c298e6e9c5bf15e))
- **mcp:** A lock handle stops being something a model has to carry ([`f217820`](https://github.com/oisee/vibing-steampunk/commit/f217820250aeab15f7c74a7970364e5a22852b9e))
- **saprfc:** A breakpoint went to the wrong object when the name was shared ([`0ad10d1`](https://github.com/oisee/vibing-steampunk/commit/0ad10d1013af442b4674abd33714d5d5f3493c31))
- **cli:** -s <system> reached five commands only as an empty config ([`1226501`](https://github.com/oisee/vibing-steampunk/commit/122650182b4e8046235834f8f252b6c133f00ae7))
- **debug:** The source pane pushed the controls off the page ([`5b2096e`](https://github.com/oisee/vibing-steampunk/commit/5b2096ec0c658a0f193421a7e27921b4b069d768))
- **jseval:** The tokeniser cut identifiers in the middle of a rune ([`70dd191`](https://github.com/oisee/vibing-steampunk/commit/70dd191ca8f70f7d3bdc77da29a36ef3c940d9d7))


### Features

- **adt:** Add INCL (PROG/I) write support for WriteSource, EditSource, CLI ([`02e2182`](https://github.com/oisee/vibing-steampunk/commit/02e2182d40c34a86a245de30c537a11ce1da1b82))
- **debug:** A local UI, so the debugger can be looked at ([`e45c2f0`](https://github.com/oisee/vibing-steampunk/commit/e45c2f0309841c03dafe58fc837c85ee023675dd))
- **debug:** The UI opens on an object and stops in it ([`640fea0`](https://github.com/oisee/vibing-steampunk/commit/640fea054d2d9f7a9e3f6617e895194fc8fe2681))
- **debug:** Run sets the breakpoint, calls the target and catches it ([`9f26be8`](https://github.com/oisee/vibing-steampunk/commit/9f26be8a4e602a6235aed94481acf37a55ad2a5c))
- **cluster:** Decode EXPORT data clusters — BALDAT, INDX, STXL — over plain ADT ([`64d1734`](https://github.com/oisee/vibing-steampunk/commit/64d1734909ade811a48e747b3a85558094e0394b))



## [2.54.0] - 2026-08-27
### Performance

- **mcp:** Defaults chosen for a terminal, paid for in a context window ([`ffe2066`](https://github.com/oisee/vibing-steampunk/commit/ffe206692cc1335723348af55842b6f0eb08b740))



## [2.53.0] - 2026-08-26
### Features

- **mcp:** An empty SAP() call answers "what am I talking to?" ([`5cbd757`](https://github.com/oisee/vibing-steampunk/commit/5cbd757cfa87532dc8583ce7e18a0be62e3f08a5))



## [2.52.0] - 2026-08-25
### Bug Fixes

- **mcp:** Document the parameter names the handlers actually read ([`2937215`](https://github.com/oisee/vibing-steampunk/commit/2937215add0a87775a44dc243645a57d28c5a545))
- **adt:** Upsert treats only a 404 as "the object is not there" ([`2883736`](https://github.com/oisee/vibing-steampunk/commit/2883736c9e39c9f6de6f54c7ce5a233c09fca428))
- **i18n,sweep:** Probe the eleven, and fix the three the probes killed ([`b486833`](https://github.com/oisee/vibing-steampunk/commit/b486833acd0e131729ca3da3efa2f5d7a3cf1a3b))
- **sweep:** Tell a gap in coverage apart from a rule about it ([`9173998`](https://github.com/oisee/vibing-steampunk/commit/91739985712d455689d07d6595c80fa73e834a34))


### Features

- **mcp:** One declaration per capability, and everything else derived ([`4674497`](https://github.com/oisee/vibing-steampunk/commit/4674497c567665980d7e107989cf10932f6ea8a3))



## [2.51.0] - 2026-08-25
### Bug Fixes

- **sweep:** A target resolved without its type made every probe assert CLAS ([`4998964`](https://github.com/oisee/vibing-steampunk/commit/4998964ed048228a1cdd8a28a338db3ff9e918ef))
- **sweep:** The probes asserted a type the sweep had resolved ([`036548a`](https://github.com/oisee/vibing-steampunk/commit/036548a968ec3e2a4416f5ef46de52cad8e1dd48))
- **sweep:** The new target had a case and no substitution ([`9074682`](https://github.com/oisee/vibing-steampunk/commit/90746826d6eee53c1760683d0f9469d5a4a626f4))
- **sweep:** A type assertion that manufactured a fact about the system ([`4305dba`](https://github.com/oisee/vibing-steampunk/commit/4305dbac160b1e75feddfed53976ec98a206ac71))
- **sweep:** Four capabilities became reachable and none of them entered the denominator ([`96a004f`](https://github.com/oisee/vibing-steampunk/commit/96a004f593357f4698279728cf982c61b3af7fd6))
- **sweep:** A client-side filter the row limit could starve ([`32a3cfa`](https://github.com/oisee/vibing-steampunk/commit/32a3cfae6969dee8703eb2a3d8bdef7457b466fd))
- **sweep:** The report could not name the release its verdicts are about ([`449f20d`](https://github.com/oisee/vibing-steampunk/commit/449f20d4bb6baa8510b66fd078af211e876385ed))


### Features

- **mcp:** The last eleven capabilities are reachable from the universal tool ([`9a2e623`](https://github.com/oisee/vibing-steampunk/commit/9a2e623e0a7e4c39103c52ff5b4e013c7be502df))



## [2.50.0] - 2026-08-25
### Bug Fixes

- **ctxcomp:** A dependency one layer did not look for is not a false positive ([`330d2ca`](https://github.com/oisee/vibing-steampunk/commit/330d2cafa80bb793dd86a9dc272b4bd152f9b3e1))


### Features

- **ctxcomp:** Rank candidates by what a reader needs, and spend the budget on what arrives ([`d710ceb`](https://github.com/oisee/vibing-steampunk/commit/d710cebfa165f60c790d355fc3c21608bf12d48e))
- **ctxcomp:** Who calls this, which the source cannot say ([`533eb51`](https://github.com/oisee/vibing-steampunk/commit/533eb51c1041e276e81a1c629765bd7b4436d2bc))
- **ctxcomp:** A contract narrowed to the methods the code actually calls ([`ee5a239`](https://github.com/oisee/vibing-steampunk/commit/ee5a23944c2a54fa5c30d055630a2ee35af9c72e))



## [2.49.0] - 2026-08-25
### Bug Fixes

- **cli:** The rename preview could say "nothing references this" without having looked ([`81efc9f`](https://github.com/oisee/vibing-steampunk/commit/81efc9fc3125e65f4ed4057a9639cd4665559d62))
- **examples:** Callers were read from the wrong half of the object ([`f7b8a77`](https://github.com/oisee/vibing-steampunk/commit/f7b8a77ed7d4f05ece9eac38b216046a9192c34b))
- **ctxcomp:** The context appended to a read could not see CREATE OBJECT ([`e34c059`](https://github.com/oisee/vibing-steampunk/commit/e34c059e760aba71b80fda4b956392d562a1a601))


### Features

- **graph:** The section a cross-reference row points at, which was computed and discarded ([`4f17b5b`](https://github.com/oisee/vibing-steampunk/commit/4f17b5b8e8a15f9dbcfb86104698380631bb319e))


### Performance

- **scans:** Read sources six at a time, and let health cover the package ([`4f24b64`](https://github.com/oisee/vibing-steampunk/commit/4f24b64ca71d5e2246e8611d0930866b3ddcffd0))



## [2.48.0] - 2026-08-25
### Bug Fixes

- **graph:** The parser saw no functional-style call, and no exception at all ([`b837e59`](https://github.com/oisee/vibing-steampunk/commit/b837e5933a046be77a15b7ad093a698d440a4e45))
- **boundaries:** A clean verdict over a package read in part ([`7b8f370`](https://github.com/oisee/vibing-steampunk/commit/7b8f3705856ce834ae8e3c1085c887d5e1b2e271))
- **sourcestamp:** The invalidation signal exists after all, and here it is ([`c161e70`](https://github.com/oisee/vibing-steampunk/commit/c161e70cebc6cc87108455f323b0896b7236b2ad))
- **graph:** A class's generated companion was reported as another object loading it ([`38bac88`](https://github.com/oisee/vibing-steampunk/commit/38bac880bb9257f375e0a99263880116809cf44a))
- **loads:** The loaded-by list named the object instead of its loader ([`4cb4059`](https://github.com/oisee/vibing-steampunk/commit/4cb4059bf43d95bedabcffac2469a0b39e5ee266))
- **adt:** A package listed 55 includes as programs, and none of them could be read ([`4dff03f`](https://github.com/oisee/vibing-steampunk/commit/4dff03fb244539339de4d614931502074da4662e))


### Features

- **loads:** The command half, and a suffix I had named without asking ([`c6f754d`](https://github.com/oisee/vibing-steampunk/commit/c6f754d95192e821bd271048d7604c039d838832))


### Performance

- **boundaries:** 18.8s to 1.6s, and the cache is deliberately not built ([`35f573e`](https://github.com/oisee/vibing-steampunk/commit/35f573e3a413e6f329b6eb73f72cd87f98fc7f90))



## [2.47.0] - 2026-08-24
### Bug Fixes

- **sweep:** The coverage figure could contradict itself ([`b17d6ef`](https://github.com/oisee/vibing-steampunk/commit/b17d6ef5f4b7c98ba8006f04b82ee85fdf8b9e2f))


### Features

- **mcp:** Every mode reaches the analyze surface, and the sweep now checks it ([`82588fc`](https://github.com/oisee/vibing-steampunk/commit/82588fc8d529ad5c97db1d0af6673d0db94a4568))
- **effects:** The LUW analysis is reachable, four months after it was written ([`a714cbf`](https://github.com/oisee/vibing-steampunk/commit/a714cbf7839a330f8797f77cef60f4797f83687b))
- **graph:** Graph_stats answers about an object or a package, not only about pasted source ([`1d527f8`](https://github.com/oisee/vibing-steampunk/commit/1d527f85e77d46ab4e7c6398ad204dd6d751c829))
- **graph:** The inactive index is reachable, and never mixed in ([`370b478`](https://github.com/oisee/vibing-steampunk/commit/370b4783aa0ae845e6f2178846b85e70c768a6e4))
- **graph:** D010INC, the load graph — the one source that is not a cross-reference ([`4203ca9`](https://github.com/oisee/vibing-steampunk/commit/4203ca94e2f1bc6c3f9776c01be888834515cc97))



## [2.46.0] - 2026-08-24
### Bug Fixes

- **graph:** Trace_execution went quiet in four places, and the surface was two larger than counted ([`e678848`](https://github.com/oisee/vibing-steampunk/commit/e678848984bd705161eb1449668ed0e9030d40bb))
- **graph:** Check_boundaries called a package clean without opening a single file ([`b3a3bbc`](https://github.com/oisee/vibing-steampunk/commit/b3a3bbcdfbc41aba6891dce19541eb480e4f394f))
- **graph:** Two nodes for twenty-seven edges, and coverage measured against things that cannot run ([`b1b4f29`](https://github.com/oisee/vibing-steampunk/commit/b1b4f29376a581e1be65a6f0f4338460a9f0b216))
- **graph:** The references answer was too large to be read by the agent asking ([`84487ae`](https://github.com/oisee/vibing-steampunk/commit/84487ae412319f8b9081eaebb0d9fe8931eeab63))
- **graph:** The parser invented a function module out of a variable name ([`62c4c8e`](https://github.com/oisee/vibing-steampunk/commit/62c4c8e639fe0bc45d544def178e6e86c845e33d))
- **deploy:** A function module could be created from a file but never updated ([`e58097d`](https://github.com/oisee/vibing-steampunk/commit/e58097da82092914f82d02560252e8162c176c92))
- **rename:** The same lost parameter, in the other handler ([`849b70d`](https://github.com/oisee/vibing-steampunk/commit/849b70d1e791c3068825dda59a6be20a1dc1eb06))
- **graph:** A namespaced object had no name by the time the query was built ([`dc5a10d`](https://github.com/oisee/vibing-steampunk/commit/dc5a10d9a7d8bf3220bb3e5e1d8de0aec61e1a97))
- The three findings the sweep left standing, and a fourth under one of them ([`3a84261`](https://github.com/oisee/vibing-steampunk/commit/3a84261c57120d544a8f807a36d076d8d2e6f8af))
- **dumps:** A dump addressed by its own id arrived with nothing but the id ([`256e2c0`](https://github.com/oisee/vibing-steampunk/commit/256e2c0e0e65b40bb48372cae9e9d033593f8029))


### Features

- **amdp:** The call stack, with both positions for the same statement ([`5e4c72a`](https://github.com/oisee/vibing-steampunk/commit/5e4c72ab625e5d78e7b28d58c5674a89ab7bf9b8))
- **sweep:** Call everything we advertise, and report what did not answer ([`9bfe190`](https://github.com/oisee/vibing-steampunk/commit/9bfe1907d4db7084c67dd394e471f2af186e30ad))
- **edit:** A DDIC table can be edited like any other source ([`438c3d8`](https://github.com/oisee/vibing-steampunk/commit/438c3d877072e926f669540f5e6d0c728698c3de))
- **sweep:** Probes for the last ten types, and two kinds of "cannot ask" ([`c1d1ccd`](https://github.com/oisee/vibing-steampunk/commit/c1d1ccdd82d4a0b78636eed5a7a373799944b730))



## [2.45.0] - 2026-08-23
### Bug Fixes

- **grep:** A search that skipped objects said it had searched them ([`92d23cc`](https://github.com/oisee/vibing-steampunk/commit/92d23cc2eab0db03074ce79cf9e27149b17037dd))
- **analysis:** A whole ADT namespace that does not exist, and three features on it ([`9b3a1bb`](https://github.com/oisee/vibing-steampunk/commit/9b3a1bbc56ae312f37d19fc6ed3c90fd2c427cac))
- **cli:** Reports that could not look everywhere said so nowhere ([`77357aa`](https://github.com/oisee/vibing-steampunk/commit/77357aa42d5f98d4cd30aa6d5b970dc62801dc01))
- **gaps:** Checks that never ran were reported as checks that found nothing ([`834f878`](https://github.com/oisee/vibing-steampunk/commit/834f878eb82c8f377e869c43291ecf20e49ec5d9))
- **tr-boundaries:** A transport holding nothing was reported SELF-CONSISTENT ([`8b94c47`](https://github.com/oisee/vibing-steampunk/commit/8b94c4719f7d4edf5b37c9da798a2f2bc63ac9ad))
- **graph:** An include numbered above U19 resolved to the wrong kind of object ([`869a835`](https://github.com/oisee/vibing-steampunk/commit/869a8355ff6285ca1679a274b25c83fc24ac6d6a))
- **debug:** An interrupted session leaked a debug work process ([`4833260`](https://github.com/oisee/vibing-steampunk/commit/48332608169803a9e440bbdadfd434a9b84b769c))
- **amdp:** A breakpoint verdict nobody could see, and a trace that lost its debuggee ([`7a1f897`](https://github.com/oisee/vibing-steampunk/commit/7a1f897c9f9ee3f1d90e1297ee83ead1991f68be))
- **execute:** A program that will not compile is not a program that ran ([`1628605`](https://github.com/oisee/vibing-steampunk/commit/162860593b07be5fd9a86f21f35976a8dcb5d39c))
- **graph:** Half of a callee list was reported as all of it ([`c7cb144`](https://github.com/oisee/vibing-steampunk/commit/c7cb144174cf0ec64b5e0485d5da10b2f64f039e))
- **graph:** Four more places where a source that failed counted as a source that was empty ([`ddd236f`](https://github.com/oisee/vibing-steampunk/commit/ddd236f7bbc52122331b8e216f54f37ceedb9c81))
- **graph:** The last three, so the sweep leaves nothing named and unfixed ([`610c383`](https://github.com/oisee/vibing-steampunk/commit/610c3833358017ccc6ca88c29f039adb2243fa66))
- **graph:** A SHA-1 was being reported as the name of a referenced object ([`7bf0f61`](https://github.com/oisee/vibing-steampunk/commit/7bf0f61823388dbd7e13e05ede90ef3e95e37348))


### Features

- **amdp:** A statement-level trace of SQLScript running inside HANA ([`bd21ba4`](https://github.com/oisee/vibing-steampunk/commit/bd21ba439a985401f3efe7cd5bf0b5f8f2eea807))
- **graph:** Decode a method include, which unblocks upward tracing ([`fc229bd`](https://github.com/oisee/vibing-steampunk/commit/fc229bd03679c28fa29cc54ec53788211cc2cfaa))
- **amdp:** Read a variable of a stopped SQLScript, values and all ([`6bb803e`](https://github.com/oisee/vibing-steampunk/commit/6bb803e1948254cb1f41c772f3bba7e4c155f5dd))
- **amdp:** Table variables — the address is right, the values are not, and that is written down ([`9cf8de3`](https://github.com/oisee/vibing-steampunk/commit/9cf8de31c3f80bee356951d51fe8d3a54d8d4e18))
- **amdp:** The stop already describes the whole scope, so alocals costs nothing ([`c238955`](https://github.com/oisee/vibing-steampunk/commit/c23895544e5deea0fc74fc0c85fa9e8deb4e70e0))
- **graph:** An empty callee list now says when the references are filed elsewhere ([`0d0390e`](https://github.com/oisee/vibing-steampunk/commit/0d0390e56ca67b704393c0ef883ad4e3df3d1135))



## [2.44.0] - 2026-08-23
### Bug Fixes

- **report:** Two claims about the AMDP session were wrong, and the tool hid why ([`19e2796`](https://github.com/oisee/vibing-steampunk/commit/19e2796cc284ca82f13317a9e42de339ae5065cf))
- **dumps:** The graph rung cannot fire, and my own code was hiding it ([`12a5398`](https://github.com/oisee/vibing-steampunk/commit/12a5398a0708a50dd27514e4807f5c2bb60e4a5e))
- **execute:** Notice that the code died, in the response and in ST22 ([`989ea8f`](https://github.com/oisee/vibing-steampunk/commit/989ea8f65eacb1fa60fbe652e9d3dad8f2a47e23))
- **test:** Two agents named a helper 'at'; the names now say which is which ([`3bb1f06`](https://github.com/oisee/vibing-steampunk/commit/3bb1f067b7c155303ce09c9e50065ce0b19633ac))
- **graph:** Answer callers and callees from sources that exist ([`28b5aed`](https://github.com/oisee/vibing-steampunk/commit/28b5aed475f952bb06c63f60314480df0d334e3a))


### Features

- **dumps:** The similar-dump ladder, and what the detail actually carries ([`02bae70`](https://github.com/oisee/vibing-steampunk/commit/02bae70f5d7a0810b8866c8762fe558e232b808b))
- **dumps:** Graph up as blast radius, and why it is not a rung ([`d40b2ca`](https://github.com/oisee/vibing-steampunk/commit/d40b2ca91d93219b1ee434a510839745acaf9c0d))
- **amdp:** The breakpoint fires — AMDP debugging over plain ADT, no Z code ([`61790b5`](https://github.com/oisee/vibing-steampunk/commit/61790b5bff29c20d0edd74a94919386fa4cdf787))
- **mcp:** The post-mortem surface an agent could not reach, and the old path it was hiding ([`6098bd8`](https://github.com/oisee/vibing-steampunk/commit/6098bd8236536669d7f26032248fe4fc8d89317a))
- **amdp:** AMDP debugging over ADT reaches MCP, on the session already held ([`4cd3ce4`](https://github.com/oisee/vibing-steampunk/commit/4cd3ce47b44fa019f4cdaf14496c87b04e22c7b7))



## [2.43.0] - 2026-08-22
### Bug Fixes

- **fileio:** Create the output directory instead of failing on it ([`5d567d4`](https://github.com/oisee/vibing-steampunk/commit/5d567d4d1721071061bde75fde4a8781006f3a97))
- **ws:** Open WebSockets with the session in use, not the one from startup ([`c214df2`](https://github.com/oisee/vibing-steampunk/commit/c214df2942f8a29855b97dc4dc05602521f4232d))
- **amdp:** Probe the AMDP debugger resource that exists ([`58eabaa`](https://github.com/oisee/vibing-steampunk/commit/58eabaab2d3fa30c78714d00ecc784dd4a6143a2))
- **debug:** Read the call stack on releases that have no stack resource ([`3b01ff9`](https://github.com/oisee/vibing-steampunk/commit/3b01ff9a6d4f7056e29719a90e83a2e906bb16a8))
- **debug:** Expand an internal table into its rows, and move frames by number ([`caf1218`](https://github.com/oisee/vibing-steampunk/commit/caf12185eedded92a8e3a7d087fc802b83d752fc))
- **debug:** Every recorded trace came out with no values in it ([`4247b6a`](https://github.com/oisee/vibing-steampunk/commit/4247b6a100bfe3f25b0a4ba16e6b5904c9d7e846))
- **dumps:** 7.50 has the dump feed but not the detail resource ([`64108f0`](https://github.com/oisee/vibing-steampunk/commit/64108f0072df64cfd87e653fa1559a45779bbf9c))


### Features

- **deps:** Build the abapGit archive from upstream instead of from a SAP system ([`462e1c7`](https://github.com/oisee/vibing-steampunk/commit/462e1c7b3687327f6ad293d79728b0a88a0116c0))
- **debug:** Sample a large table head, middle and end rather than its first rows ([`bef11e1`](https://github.com/oisee/vibing-steampunk/commit/bef11e1bd22a22565f1401374673df940251ecba))
- **applog:** Read the application log without RFC, and without remembering BALHDR ([`db863e4`](https://github.com/oisee/vibing-steampunk/commit/db863e47623b6498d79c969ccd219fd3dc05e4cd))
- **dumps:** Read runtime errors, group them, and rank what was logged around one ([`663a1f1`](https://github.com/oisee/vibing-steampunk/commit/663a1f14e96faa4b4925dc57be1b163805315e6f))
- **dumps:** Read the call stack, and rank a log written by any frame on it ([`fc47a25`](https://github.com/oisee/vibing-steampunk/commit/fc47a25b297073e5ab12513bd0d203e494a67045))
- **dumps:** Add the graph rung — a log written by something a stack frame calls ([`5b6154e`](https://github.com/oisee/vibing-steampunk/commit/5b6154ed63b772541557fd71e673291a3222a4cc))



## [2.42.0] - 2026-08-22
### Bug Fixes

- **adt:** Check syntax before locking, and let a forbidden HEAD fall back to GET ([`ff32cd7`](https://github.com/oisee/vibing-steampunk/commit/ff32cd713a26507f79569a0988edece5e24df83a))
- **cli:** A read_only system must be read-only on the command line too ([`b9769d4`](https://github.com/oisee/vibing-steampunk/commit/b9769d490a83430c086bb7029c2d8fd79651d087))
- **install:** Refuse an embedded archive that is empty instead of deploying nothing ([`a9db9eb`](https://github.com/oisee/vibing-steampunk/commit/a9db9eb1e4e792f41143b97e223c85a5c4a84492))
- **adt:** Stop reading NoModification as read-only, and stop leaking the lock ([`9b98997`](https://github.com/oisee/vibing-steampunk/commit/9b9899703cccf76b570914d8020e8fdd44237707))
- **http:** Recover a browser session that expires without a 401 ([`e66bc18`](https://github.com/oisee/vibing-steampunk/commit/e66bc183ece3d40ed1f67af6060cb0dca084b603))
- **cli:** Let a system declare its transport safety, so the transport commands can run ([`ae5f684`](https://github.com/oisee/vibing-steampunk/commit/ae5f684228a548ac71c39cffe9f7b90e52b517b6))
- **mcp:** Say what the call is missing instead of that the action does not exist ([`8a5670b`](https://github.com/oisee/vibing-steampunk/commit/8a5670bfff9be850ef465bacca030d95e0a2d8ea))
- **abap:** Let the git service compile against either abapGit release ([`64560a1`](https://github.com/oisee/vibing-steampunk/commit/64560a15aea7cfe51128d84c07843ea2c7713693))
- **make:** Install wrote to /bin, and add a link target for development ([`ff88b00`](https://github.com/oisee/vibing-steampunk/commit/ff88b0065deb34ad5c7a02507ba3bc306618a91a))
- **http:** Take the session id SAP reissues, instead of sending two ([`b9c22f3`](https://github.com/oisee/vibing-steampunk/commit/b9c22f3fb4417a149ce819447a86763959444505))
- **make:** Build the platform-named binary, and link build/vsp at it ([`cd8f550`](https://github.com/oisee/vibing-steampunk/commit/cd8f550704357aeeb66d0aa307660d1b877bb7f3))
- **landscape:** Find and read what SAP GUI for Java writes ([`56505cb`](https://github.com/oisee/vibing-steampunk/commit/56505cb15e9b6821631f4192084c100cecb5d31e))
- **adt:** Return the modules of a function group, on old releases too ([`e76c5f5`](https://github.com/oisee/vibing-steampunk/commit/e76c5f51ac1308ff4337c9a23514353d929910fe))
- **landscape:** Stop inventing hosts, and address systems the way they answer ([`9fd0421`](https://github.com/oisee/vibing-steampunk/commit/9fd042191606c2ece2686e478ce01f1f6fbe6619))
- Publish the tool counts the server actually registers, and pin them ([`4f78dcb`](https://github.com/oisee/vibing-steampunk/commit/4f78dcb471c148d5fbc9c0037f19db323431b138))
- **test:** Drop a discarded fmt.Sprint that vet rejects ([`915f5d2`](https://github.com/oisee/vibing-steampunk/commit/915f5d2b92727ab3f393cb825d8e39b113ca5751))


### Features

- **saprfc:** A password that refuses to print, and auth acceptance criteria ([`6884161`](https://github.com/oisee/vibing-steampunk/commit/6884161bea1bcaf2e1893362f7eb518b04b912ae))
- **adt:** The debugger over plain HTTPS, for systems with no RFC channel ([`89542ef`](https://github.com/oisee/vibing-steampunk/commit/89542ef87028627d24d5c01077bf04fdfd9fbd0e))
- **debug:** Read debugger variables over the ADT tunnel ([`5392864`](https://github.com/oisee/vibing-steampunk/commit/539286469e705fe8c1890bb0bcc59124d3aea7c3))
- **debug:** A typed variable model, and two bugs the transports hid ([`2460c9b`](https://github.com/oisee/vibing-steampunk/commit/2460c9bd29aad97b8ee7ee1b1245f9687cfc7096))
- **debug:** Breakpoints through ADT, so the debugger needs no Z code at all ([`01640c3`](https://github.com/oisee/vibing-steampunk/commit/01640c3b0e1ca66696193f0ee5751656f501945f))
- **mcp:** The debugger tools work, and are enabled by default again ([`1d94300`](https://github.com/oisee/vibing-steampunk/commit/1d943009ff3566a4cfd23829812cd045b9d3feb1))
- **trace:** The measured call tree, over either transport ([`13cc062`](https://github.com/oisee/vibing-steampunk/commit/13cc0621de1ad81847138cfad795065576315823))
- **debug:** Keep customer code the default, and report the lines SAP refused ([`db08ea4`](https://github.com/oisee/vibing-steampunk/commit/db08ea4452c76e119ef0ba887cda7f59f80fc791))
- **trace:** Record a unit statement by statement, with its values ([`bc88bfc`](https://github.com/oisee/vibing-steampunk/commit/bc88bfc0d1434f6ad780fd3f569a513771c46941))
- **debug:** Write variables, and move between stack frames ([`f469af7`](https://github.com/oisee/vibing-steampunk/commit/f469af7ea0696d615fcf2249878bb3bfd9f7fb56))
- **lua:** One debug session for the whole script, and one API instead of two ([`4c6fbb3`](https://github.com/oisee/vibing-steampunk/commit/4c6fbb3ffd53cbc9b5b2f40c3fde56f3174abb8e))
- **adt:** Create RFC-enabled function modules, no SE37 shell needed ([`3f948e4`](https://github.com/oisee/vibing-steampunk/commit/3f948e4d6c5d61d395629e29d12f932f5e4be658))
- **auth:** Browser SSO that keeps its own session ([`26ce707`](https://github.com/oisee/vibing-steampunk/commit/26ce7076d64d6e4795dccd41dcd7b32ba2b1bc2f))
- **adt:** Edit a function module in one call, and read one without its group ([`cf39e41`](https://github.com/oisee/vibing-steampunk/commit/cf39e413d42d4de4aa1f55a1f66d401e8e834224))
- **ws:** Authenticate the WebSocket transport with a browser session ([`feb6cda`](https://github.com/oisee/vibing-steampunk/commit/feb6cda034cdf41210fbbc98625be1b66622bc61))
- **landscape:** Read the systems SAP GUI already knows about ([`88a9273`](https://github.com/oisee/vibing-steampunk/commit/88a9273d8abf63a8163d89683f01e3478faa0473))
- **landscape:** Find every landscape this machine can reach, VMs included ([`4396051`](https://github.com/oisee/vibing-steampunk/commit/4396051757639107a4a3dd8cf0ecc336235ddfae))
- **compat:** Ask a system what it supports, and how to route each capability ([`4894713`](https://github.com/oisee/vibing-steampunk/commit/4894713a8cd99d615d3ad8bbb9351f3606a264f3))
- **detect:** Find the port a system serves ADT on, before configuring it ([`571f198`](https://github.com/oisee/vibing-steampunk/commit/571f1989bab6565f539a2daa2d212f32dc423973))
- **detect:** --all sweeps every port, and the instance shapes the shortlist ([`05a7930`](https://github.com/oisee/vibing-steampunk/commit/05a79304723e5273e7ca6f1a3057c25a83ae1b5c))
- **detect:** Prefer TLS, and follow the name a certificate points at ([`3211cce`](https://github.com/oisee/vibing-steampunk/commit/3211cce2db925b24cc149909d8e4355c7a11c235))
- **detect:** Print both config templates, and say which one to take ([`7d15030`](https://github.com/oisee/vibing-steampunk/commit/7d15030272603c4ad85a4d1bfabe47c99ab7ac51))



## [2.41.0] - 2026-08-21
### Bug Fixes

- **rfc:** WHERE splitting, wide-table fallback, one shared ReadTable ([`9528ba1`](https://github.com/oisee/vibing-steampunk/commit/9528ba1d9ed97b48bd199eac7129390953db40ab))
- **mcp:** Authenticate the HTTP transport; gate live tests; add the agenda ([`b4f6ffe`](https://github.com/oisee/vibing-steampunk/commit/b4f6ffe1afd6841d8dd819596116e2ba7adff2d7))
- **mcp:** Ping the idle RFC connection every minute ([`4930926`](https://github.com/oisee/vibing-steampunk/commit/4930926db3a7c81c53d73452607067d68e2c9ddb))
- **adt:** CSRF GET fallback and proxy-aware WebSocket dialing ([`6b136b7`](https://github.com/oisee/vibing-steampunk/commit/6b136b7a9b36959a5d87dff96f013acb55ffc4b0))
- **adt,mcp:** Message classes are writable again; ship the Apache notice ([`4a9e01f`](https://github.com/oisee/vibing-steampunk/commit/4a9e01f0a9c1d6776e707ccd45e6baeaee0faee0))
- **debug:** A breakpoint inside a function module needs its include ([`7b8d518`](https://github.com/oisee/vibing-steampunk/commit/7b8d5181b84ecdcb4814d9916e5ced18b4f7ed35))
- **debug:** Attach must activate external debugging for its own session ([`a40156b`](https://github.com/oisee/vibing-steampunk/commit/a40156bb244c5b2eebde098827e126046dd12d57))
- **debug:** Read the stop location TPDAPI actually sends ([`698a6e4`](https://github.com/oisee/vibing-steampunk/commit/698a6e4094b033be4eb6c6c7cba4318ea5866b30))
- **debug:** Project the stack instead of serialising TPDAPI's own table ([`f787d01`](https://github.com/oisee/vibing-steampunk/commit/f787d013bffa268364fc1aa52b04dc27a8b4fe2c))
- **debug:** A closed conversation is how detach succeeds ([`520f854`](https://github.com/oisee/vibing-steampunk/commit/520f85461c7372b9c21b76f59f6de8e4ed7fbf4d))
- **debug:** The adt command must send headers ([`5b32cd3`](https://github.com/oisee/vibing-steampunk/commit/5b32cd3087ba916af2aab37205a14865b8123028))
- **debug:** Detach sweeps a stale listener even from a fresh session ([`efb1135`](https://github.com/oisee/vibing-steampunk/commit/efb1135534a9103cedad1557a7361826716f8bc0))
- **adt:** Send an Accept header through the RFC tunnel ([`f95098e`](https://github.com/oisee/vibing-steampunk/commit/f95098e5514b9f9fc89d0248207024bbe0597b87))
- **adt:** Default Accept to */* , not a concrete type ([`d6a208a`](https://github.com/oisee/vibing-steampunk/commit/d6a208acb5185b581cdd818d9f9135c3a7d41024))


### Features

- **rfc:** Vsp rfc probe — fingerprint a system, including what the user may call ([`6169305`](https://github.com/oisee/vibing-steampunk/commit/61693054f12efad18e4acb301f40abb718fbede8))
- **rfc:** Vsp rfc export — abapGit ZIP in one call ([`60aacbf`](https://github.com/oisee/vibing-steampunk/commit/60aacbfe33d338ef769be8c6f6f298ad01ea68e6))
- **rfc:** Run reports as background jobs, and read job spools ([`eef5f81`](https://github.com/oisee/vibing-steampunk/commit/eef5f814eb61f9dadae0f3f96a4ceeff5cc005fd))
- **rfc:** ADT REST over the classic-RFC tunnel — `vsp rfc adt` ([`676ebe3`](https://github.com/oisee/vibing-steampunk/commit/676ebe39b5c1bbc6a4e6d8ace61140a00b6f0170))
- **rfc:** The debugger's read half, and the ZADT_DEBUG facade source ([`45298ad`](https://github.com/oisee/vibing-steampunk/commit/45298adc4b9e81d5e76c8a044e125d6326f3c6f1))
- **abap:** ZADT_DEBUG facade over TPDAPI, deployed to A4H ([`a4552e5`](https://github.com/oisee/vibing-steampunk/commit/a4552e57c94212dfea90614302f16bb6ffaea97d))
- **rfc:** Drive the ABAP debugger over a pinned session ([`5837e73`](https://github.com/oisee/vibing-steampunk/commit/5837e73089a3b5df352d72a5afc4a0745db8c92c))
- **debug:** Catch — listen and attach on the same pinned session ([`f555b7f`](https://github.com/oisee/vibing-steampunk/commit/f555b7ffa2e6e64e0aac5ab88ebb9ded67505137))
- **rfc:** Tunnel ADT REST through the pinned debug session ([`d1e407c`](https://github.com/oisee/vibing-steampunk/commit/d1e407ca125f110b4fdb010457e3a984cb1c9dcb))
- **debug:** Drive SAP's own ADT debugger over the RFC tunnel ([`6a7c6ed`](https://github.com/oisee/vibing-steampunk/commit/6a7c6ed9b49751c42beda7d5b431625272f06e51))
- **debug:** A body for adt requests, from a file ([`0bbc94f`](https://github.com/oisee/vibing-steampunk/commit/0bbc94fefdc9142a98130b8c6dfe87f81e331c9d))



## [2.40.0] - 2026-08-20
### Bug Fixes

- Health tests signal now scans full package hierarchy ([`9ebc9db`](https://github.com/oisee/vibing-steampunk/commit/9ebc9db969689ce812e03218ef33dc8a84d011f0))
- Health report filename uses _ for $ prefix in package names ([`a2bccfe`](https://github.com/oisee/vibing-steampunk/commit/a2bccfe83044d360e2523a62e1edfa15a99e7fdd))
- Pad progress lines with %-40s to prevent display artifacts ([`13ebb80`](https://github.com/oisee/vibing-steampunk/commit/13ebb803704159408355b60e67a95d6f60822b4d))
- External crossings now detected + TADIR resolution batched ([`387bae1`](https://github.com/oisee/vibing-steampunk/commit/387bae1c008e3aae21520628dc3639d6b29477b9))
- Resolve more targets + deduplicate crossing entries ([`52f8743`](https://github.com/oisee/vibing-steampunk/commit/52f8743d7b935cdafedec902e672e747006a1dd2))
- EXTERNAL crossings are WARN, not OK ([`890bb77`](https://github.com/oisee/vibing-steampunk/commit/890bb77ecc9a1a6dac2744695ea9f46030121e72))
- Resolve default system from .vsp.json and fix packageExists false negatives ([`81416d3`](https://github.com/oisee/vibing-steampunk/commit/81416d3703f62dec20ffc83a29d2c769adf0579b))
- TADIR package resolution now fetches OBJECT type and validates existence ([`012db57`](https://github.com/oisee/vibing-steampunk/commit/012db578dc1798013b7a15675f53df2f2af25a89))
- Two-pass package resolution (TADIR + TFDIR) and what-package debug command ([`b0e37c6`](https://github.com/oisee/vibing-steampunk/commit/b0e37c6eee215aae291d6d691b23a41744068e2d))
- What-package command now resolves FMs via TFDIR fallback ([`3a191b6`](https://github.com/oisee/vibing-steampunk/commit/3a191b6a95ad40f8bc960f658e94632b5d26ecc1))
- TADIR batch size reduced to 5 to stay under SAP 255-char query limit ([`1e8b239`](https://github.com/oisee/vibing-steampunk/commit/1e8b2393b4828d05c9f2bf9adf650b25980b7538))
- Never fail silently — add WARN stderr logging for all resolve/query errors ([`7687d38`](https://github.com/oisee/vibing-steampunk/commit/7687d386c53293981e89584c119433266e4d9a31))
- Batch all SAP IN-clause queries to 5 items (255-char limit) ([`5049e07`](https://github.com/oisee/vibing-steampunk/commit/5049e0769ed7dec932737f0c944d3286b1f9a6e6))
- Fixed wrong parameter ([`c736611`](https://github.com/oisee/vibing-steampunk/commit/c736611ed6677c1c690cb817ccd130775668996c))
- Fix GetDependencyZip ([`7870cae`](https://github.com/oisee/vibing-steampunk/commit/7870caef7fdcae6acfbad928a8b19738242962c0))
- Fixed APIGetReleaseState ([`5fa30ff`](https://github.com/oisee/vibing-steampunk/commit/5fa30ff162f6f9799ff64f26b6704044a8331c13))
- Correct releaseState bug and update tests for C0-C4 API structure ([`a66bcd5`](https://github.com/oisee/vibing-steampunk/commit/a66bcd5f5bafd876fa9d0f090daaf45f821b40b8))
- Enforce SAP_ALLOWED_PACKAGES on existing-object mutations (#101) ([`0713d75`](https://github.com/oisee/vibing-steampunk/commit/0713d75d74a3e84811d3e8a16de7b6629b51e5c2))
- **cr-config-audit:** Drop OR-LIKE batching, parallelise per-object instead ([`6826446`](https://github.com/oisee/vibing-steampunk/commit/6826446a269606d8e3ddd7ceaf9de4a044d8e555))
- **adt:** Reconcile partial-create on 5xx + cr-config-audit v2a.1 polish ([`3d1353e`](https://github.com/oisee/vibing-steampunk/commit/3d1353ebf5a3dacbd4d2dc8e800b5b00bb514d8c))
- Two high defects from f6b1726 review + statement-order literal scope ([`afbc19d`](https://github.com/oisee/vibing-steampunk/commit/afbc19dc0f3e74ed89dd0eb71c344a1b3a0a8adc))
- **saml:** Address PR #97 review follow-up notes ([`87ce9c7`](https://github.com/oisee/vibing-steampunk/commit/87ce9c76929619e695371717e96a913f6e274ce4))
- **adt:** Close the lock-handle bug class — Stateful + ModificationSupport guard ([`22517d4`](https://github.com/oisee/vibing-steampunk/commit/22517d46241852f473e619eeeb6a5fd827305a70))
- **adt:** Reuse an object's existing open transport on write (#144) ([`130c4d0`](https://github.com/oisee/vibing-steampunk/commit/130c4d0e05e9291adced1e4276957cb09f60f07d))
- **adt:** Extend open-transport reuse to the remaining update paths (#144) ([`53c9db3`](https://github.com/oisee/vibing-steampunk/commit/53c9db3e5d3e2dec8c3a4f430f94c844629ae7db))


### Features

- Health --details, --format md/html, --report for file output ([`bf36bce`](https://github.com/oisee/vibing-steampunk/commit/bf36bce900d41b263568a4679bbd482f0e1621d7))
- Health report groups tests by parent object, shows alert details ([`b5061a1`](https://github.com/oisee/vibing-steampunk/commit/b5061a15812ffd3f2e2fdd498607ed421c4148ca))
- Directional package boundary crossing analysis ([`53fb790`](https://github.com/oisee/vibing-steampunk/commit/53fb790d4b31d084abfbd2a54231af3f564a67f8))
- Standalone vsp boundaries command + crossing details in health reports ([`2dff9a2`](https://github.com/oisee/vibing-steampunk/commit/2dff9a23ab3aeeeefd856415828346accbd87147))
- Crossing entries show edge kind, ref detail, and object types ([`698884e`](https://github.com/oisee/vibing-steampunk/commit/698884e4cb3ea43c5088bd3e46ed2cca64a93489))
- Separate columns in crossing reports + package name guesser ([`952be05`](https://github.com/oisee/vibing-steampunk/commit/952be05dac54fb9cec5e9becdc35aec351c5ec91))
- Mermaid graph output with package subgraphs and edge coloring ([`8a60b62`](https://github.com/oisee/vibing-steampunk/commit/8a60b62641f2cbe395fe745d9f861860b9c3350c))
- Extract CALL TRANSACTION, CALL TRANSFORMATION, LEAVE TO TRANSACTION ([`8254536`](https://github.com/oisee/vibing-steampunk/commit/82545360f0d27954f27e37c979f85749de0bc263))
- Side effect extraction + LUW classification (Phase 1) ([`11c2253`](https://github.com/oisee/vibing-steampunk/commit/11c2253aee103a6b402a4a9454d7b6636276636f))
- Graph export formats — DOT, PlantUML, GraphML ([`91b49f1`](https://github.com/oisee/vibing-steampunk/commit/91b49f105cc8f1b7e5859669e88a5bf2fd275180))
- Cache config support in .vsp.json and env vars ([`7c8dfbc`](https://github.com/oisee/vibing-steampunk/commit/7c8dfbc114518488fb74ef52e0617c1e3b59a4cf))
- CR-level co-change expansion via E070A transport attributes ([`ade71be`](https://github.com/oisee/vibing-steampunk/commit/ade71be48a6253d56952c3a35da1cb0f9d7dad82))
- CO_TRANSPORTED edge kind for weaker co-change impact signals ([`8565769`](https://github.com/oisee/vibing-steampunk/commit/8565769f352145179bc310d757237c0adef2f6d3))
- Tr-boundaries, cr-boundaries, cr-history + HTML report TOC and test filtering ([`fc99eb3`](https://github.com/oisee/vibing-steampunk/commit/fc99eb33b527bd5a5a03c1d45111349572c09226))
- Default mode changed from focused to hyperfocused ([`880aa68`](https://github.com/oisee/vibing-steampunk/commit/880aa6879c9e534a4e34e52f1e6e42593b6d019b))
- --report html for tr-boundaries and cr-boundaries ([`2115afb`](https://github.com/oisee/vibing-steampunk/commit/2115afb215a5e25650610a59cd7f4b5aee35908e))
- --details flag for tr/cr-boundaries shows cross-package deps within scope ([`1e8034a`](https://github.com/oisee/vibing-steampunk/commit/1e8034a1cc54eda520116888b14bb72618300b2a))
- Detect HANA database from S4CORE component in GetSystemInfo (#100) ([`d96c38e`](https://github.com/oisee/vibing-steampunk/commit/d96c38e98a7f65b8b733a000419e585a090f6137))
- Add SAML SSO authentication for S/4HANA Public Cloud (#97) ([`e62c7d5`](https://github.com/oisee/vibing-steampunk/commit/e62c7d5e85408297f0eea52867848431cd7c385a))
- Cr-config-audit v1 and FUGR source extraction fix ([`edd94bc`](https://github.com/oisee/vibing-steampunk/commit/edd94bc2f1d342a12bcc98370ac0922131d45223))
- **cr-config-audit:** V1.2a — DDIC metadata chain ([`5b36f53`](https://github.com/oisee/vibing-steampunk/commit/5b36f538d4cde129b199f1f5da4d31c72d6b2764))
- **cr-audit:** Stable order, FUGR progress, and L2 sqlite cache ([`792ce58`](https://github.com/oisee/vibing-steampunk/commit/792ce584f0226ca2dbae9b03f9049ecd05c6bd34))
- **cr-config-audit:** V2a-min value-level literal matcher ([`2a15190`](https://github.com/oisee/vibing-steampunk/commit/2a15190b9433e7a7319db3e95250163e6bccbcf5))
- **cr-config-audit:** Per-object L2 cache for CROSS/WBCROSSGT scans ([`ab665c4`](https://github.com/oisee/vibing-steampunk/commit/ab665c4107ab81b096e1ba987924a6e2fee8bd87))
- **cr-audit:** 1-hop transitive reach + MD report parity + scope fix ([`49173e8`](https://github.com/oisee/vibing-steampunk/commit/49173e86a8ee4e8fe9117a23da6d8c8612fbabed))
- **cr-audit:** DDIC delivery class filter — transactional and views no longer false-positive as MISSING ([`e0fef2a`](https://github.com/oisee/vibing-steampunk/commit/e0fef2a65c0902e979a2addd471b3bb43015e947))
- **mcp:** Phase 3 — RecoverFailedCreate recovery primitive ([`f00356a`](https://github.com/oisee/vibing-steampunk/commit/f00356a7df785b47c76346a33542cd7a7bd110d2))
- **cli:** Vsp recover-failed-create — CLI wrapper for the recovery primitive ([`1b05441`](https://github.com/oisee/vibing-steampunk/commit/1b054417e2563e5600b0cd79e30c55fd252591aa))
- **cr-audit:** Classify orphans by DDIC delivery class ([`4b5b0e9`](https://github.com/oisee/vibing-steampunk/commit/4b5b0e9e67099ed9ad4ae79826a6fdb4bbef57ec))
- **cr-audit:** Treat DOMA in CR as implicit cover for its FIXVAL node ([`ce1f191`](https://github.com/oisee/vibing-steampunk/commit/ce1f1919026ba3f7af83de69c20fc539f9ab3eb1))
- **rfc:** Call SAP function modules over classic RFC (vsp rfc …) ([`d4e51ea`](https://github.com/oisee/vibing-steampunk/commit/d4e51ead6a1570b53d96a886b1f38032b1aa6092))
- **mcp:** Classic RFC as an action of the universal SAP tool ([`2f79046`](https://github.com/oisee/vibing-steampunk/commit/2f79046d62dc89da2e390fb56cbcf36b364175dc))


### Performance

- **adt,cr-audit:** Parallel FUGR source fetch, deterministic helper order ([`5aed8ab`](https://github.com/oisee/vibing-steampunk/commit/5aed8ab0e1ec0ec95ef0008a620cea9440fab7bc))
- **cr-audit:** Batch CROSS/WBCROSSGT, TFDIR batch, TADIR by (type,name), maxObjects warning ([`55f4c65`](https://github.com/oisee/vibing-steampunk/commit/55f4c6502baf384797e18e04ffeaacbe26262a78))



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




