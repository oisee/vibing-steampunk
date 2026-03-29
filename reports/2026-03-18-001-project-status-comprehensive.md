# VSP Project Status Report — Comprehensive Review

**Date:** 2026-03-18
**Report ID:** 2026-03-18-001
**Subject:** Complete analysis of all issues, PRs, and strategic alignment
**Version:** v2.29.0-dev (post-decomposition)

---

# Part I — English

## 1. Executive Summary

VSP (vibing-steampunk) is a Go-native MCP server for SAP ABAP Development Tools (ADT). As of March 18, 2026:

- **122 tools** across focused (81) and expert (122) modes
- **8 unique contributors** have merged PRs
- **13 open issues**, 4 bugs, 6 feature requests
- **12 open PRs** from 7 contributors
- **20 issues closed** in the last month
- **14 PRs merged** in the last month

Today's session delivered: strategic decomposition (Phase 1), one-tool mode (Phase 2), CLI DevOps surface (Phase 4), 4 community PRs merged, 5 bugs fixed, 6 issues closed.

---

## 2. Closed Issues (Resolved)

### #69 — License file missing
**What:** README said MIT but no LICENSE file existed.
**Resolution:** Added MIT LICENSE file. Trivial.

### #71 — CreatePackage safety check fails with empty package name
**What:** When creating a transportable package with `SAP_ALLOWED_PACKAGES` set, safety check read an empty string instead of the package being created.
**Root cause:** `checkPackageSafety` was checking `opts.PackageName` (parent) instead of `opts.Name` for package creation.
**Resolution:** Fixed in `crud.go`. Effort: 15 min.

### #70 — CreateTransport fails on S/4HANA 757
**What:** Wrong endpoint (`/cts/transports`) and content type for newer S/4HANA systems.
**Root cause:** Endpoint and XML format were for older systems. S/4HANA 757 requires `/cts/transportrequests` with `transportorganizer.v1` content type.
**Resolution:** Updated `transport.go` with correct endpoint, headers, and XML body format. Effort: 30 min.

### #54 — SAP_ALLOWED_PACKAGES blocks InstallZADTVSP
**What:** Bootstrap chicken-and-egg: install needs to create `$ZADT_VSP` but safety blocks it because it's not in the allowed list.
**Resolution:** Added `AllowPackageTemporarily()` method that temporarily adds the install target package. All install handlers use it with `defer` cleanup. Effort: 30 min.

### #52 — SyntaxCheck fails for long namespaced classes
**What:** `/source/main` suffix appended to already-long namespaced class URLs exceeds SAP URI limit.
**Resolution:** SyntaxCheck now uses bare object URL for the `checkObject` URI. Effort: 15 min.

### #33 — EditSource treats warnings as errors
**What:** Any syntax check result (including warnings) blocked saves.
**Resolution:** Merged PR #36 from kts982. EditSource now separates errors from warnings, adds `ignore_warnings` parameter.

### #58 — Local code behavior (wontfix)
### #57 — DebuggerGetVariables validation (resolved)
### #50 — Accidental PRs (housekeeping)
### #47 — OpenAI/GPT models with MCP (resolved — user config issue)
### #32 — 401 auto-retry (fixed in PR #35)
### #28 — macOS Apple Silicon build (resolved)
### #24 — DebuggerGetVariables schema (fixed in PR #25)
### #19 — Packages not found (fixed in PR #20)
### #18 — WriteSource namespaced objects (fixed)
### #17 — EditSource lock conflict (fixed)
### #15 — macOS M1 version lag (fixed)
### #13 — HTTP proxy support (fixed)
### #12 — Tool usage unclear (docs improved)

---

## 3. Open Issues — Bugs

### #55 — RunReport fails in APC context
**What:** RunReport's spool output retrieval times out because all standard SAP spool mechanisms (SUBMIT, COMMIT WORK AND WAIT, CALL FUNCTION...DESTINATION, RSTS_OPEN) are blocked inside APC WebSocket handler context.
**Impact:** RunReport via WebSocket is unreliable. Users get timeouts.
**Strategic alignment:** RunReport is a focused-mode tool. Reliability matters.
**Possible fix:** Wrapper report + cache table pattern. The ABAP side writes spool output to a Z-table, then the APC handler reads it. Requires ABAP-side changes to ZADT_VSP.
**Effort:** Medium (2-3 hours). Needs ABAP development + Go-side retry logic.
**Priority:** Medium. Workaround exists (use variants, use RunReportAsync).

### #56 — Unable to create new program
**What:** User reports "no such capability" when trying to create a program. Screenshots show the tool isn't visible.
**Impact:** User confusion. Likely a mode/configuration issue (focused mode doesn't expose CreateAndActivateProgram, only WriteSource with upsert).
**Possible fix:** Better error messages, documentation. WriteSource in focused mode handles create-if-not-exists via upsert.
**Effort:** Low (1 hour). Mostly docs/messaging.
**Priority:** Low. Not a code bug — user education.

### #43 — Missing commands
**What:** Originally about `deploy-handler` command not found. Updated to include questions about SICF activation and class naming discrepancies.
**Impact:** Confusion about installation workflow.
**Possible fix:** PR #67 fixed the naming issue. Remaining questions are documentation gaps.
**Effort:** Low. Close with documentation update.
**Priority:** Low.

### #26 — GetTransport fails
**What:** GetTransport returns "transport not found in response". User has `SAP_ENABLE_TRANSPORTS` set but system doesn't recognize it.
**Impact:** Transport feature unusable for this user.
**Possible fix:** May be related to #70 (S/4HANA endpoint differences). The fix for #70 may resolve this. Also check if `--enable-transports` flag vs env var parsing is correct.
**Effort:** Low-Medium. May already be fixed by #70.
**Priority:** Medium. Should verify after #70 fix.

---

## 4. Open Issues — Feature Requests

### #40 — i18n/Translation tools
**What:** 7 tools for managing ABAP translations across languages. GetObjectTextsInLanguage, GetDataElementLabels, GetMessageClassTexts, etc.
**Has PR:** #42 (874 additions, 6 tests)
**Strategic alignment:** Good. Translation is a common DevOps task. Extends the tool surface naturally.
**Effort:** Already implemented in PR #42. Review + merge.
**Priority:** Medium.

### #39 — gCTS tools
**What:** 10 tools for git-enabled CTS. Repository CRUD, clone, pull, commit, branch management.
**Has PR:** #41 (1,026 additions, 11 tests)
**Strategic alignment:** Good. gCTS is SAP's native Git integration. Complements our abapGit integration. Important for cloud/BTP customers.
**Effort:** Already implemented in PR #41. Review + merge.
**Priority:** Medium-High for cloud customers.

### #34 — GetTableContents pagination and schema
**What:** No pagination (offset) support. No way to get table schema without querying data.
**Has PR:** #37 (92 additions)
**Strategic alignment:** Good. Pagination is basic usability. Schema introspection helps LLMs understand data structures.
**Effort:** Already implemented in PR #37. Review + merge.
**Priority:** Medium.

### #30 — Cookie authentication docs
**What:** User wants more documentation on cookie auth — where to get cookies, what format.
**Strategic alignment:** Cookie auth is already implemented. Just needs docs.
**Effort:** Low (30 min). Add to README or docs site.
**Priority:** Low.

### #27 — More object types (AFF/NROB)
**What:** Request to support ABAP File Format (AFF) native objects like NROB (Number Range Object).
**Strategic alignment:** Good long-term. AFF is the future of ABAP object serialization. Aligns with abapGit compatibility.
**Effort:** Medium. Each object type needs URL mapping, XML parsing, and potentially different content types.
**Priority:** Low-Medium. Nice-to-have.

### #21 — Streaming HTTP support
**What:** "Sometime STDIO is not enough." Request for HTTP transport.
**Has PR:** #38 (mcp-go upgrade to v0.43.2 with streamable HTTP)
**Strategic alignment:** Critical for Docker deployment (#65) and remote/cloud usage. STDIO only works for local process spawning.
**Effort:** Already implemented in PR #38. Major dependency upgrade (37 files changed). Needs careful review.
**Priority:** High. Unblocks Docker and remote deployment.

---

## 5. Open Issues — Other

### #46 — Sync script: fix oisee references in markdown
**What:** Internal sync script improvement for fork maintenance.
**Effort:** Low. Script fix.
**Priority:** Low.

### #45 — Sync script: auto-resolve CLAUDE.md/README.md conflicts
**What:** Internal sync script improvement.
**Effort:** Low. Script fix.
**Priority:** Low.

### #2 — GUI debugger
**What:** Oldest open issue. Breakpoints set via vsp don't get hit in SAP GUI.
**Status:** Partially addressed with terminal ID feature (v2.21.0) and HTTP breakpoints. External debugger remains unreliable via REST → WebSocket (ZADT_VSP) is the recommended path.
**Effort:** High. Fundamental SAP GUI integration challenge.
**Priority:** Low. WebSocket debugger is the strategic direction.

---

## 6. Open PRs — Ready to Merge

### #37 — Table pagination + schema introspection
**Author:** kts982
**What:** Adds `offset` parameter to GetTableContents for pagination and `columns_only` flag for schema-only queries.
**Quality:** Clean, small (92 additions), live-tested. Closes #34.
**Merge recommendation:** ✅ Merge. Non-breaking, well-tested.

### #44 — Windows Quick Start docs
**Author:** mv0101
**What:** Adds Windows-specific setup instructions to README.
**Quality:** Docs-only (61 additions).
**Merge recommendation:** ✅ Merge. No risk.

### #53 — Clean Core API states
**Author:** andreasmuenster
**What:** Adds `GetAPIReleaseState` tool for clean core validation. Queries ADT API release states.
**Quality:** Small (44 additions), 4 files. Includes a build fix for GetDependencyZIP.
**Merge recommendation:** ✅ Merge after quick review. Useful for S/4HANA customers.

### #62 — Readonly mode
**Author:** marianfoo
**What:** Adds `--mode readonly` with ~46 read-only tools. Implies `--read-only` safety.
**Quality:** Clean concept (97 additions). But conflicts with our decomposed `tools_register.go` — needs adaptation.
**Merge recommendation:** ⚠️ Needs rebase. Adapt `readonlyTools` map into `tools_focused.go` pattern. Then merge.
**Effort to adapt:** 30 min.

---

## 7. Open PRs — Need Review

### #38 — mcp-go v0.43.2 with streamable HTTP
**Author:** danielheringers
**What:** Major dependency upgrade from v0.17.0 to v0.43.2. Migrates all handlers to new API. Adds `--transport http-streamable` flag.
**Quality:** Large (1,149 additions, 338 deletions, 37 files). Touches every handler file.
**Risk:** High — changes every handler's parameter extraction. Will conflict heavily with our Phase 2 routing changes.
**Strategic alignment:** Critical. Unblocks Docker (#65) and remote deployment. mcp-go is moving fast and we're 26 versions behind.
**Merge recommendation:** ⚠️ Needs careful review and rebase on our decomposed code. Consider as next major effort.
**Effort:** 2-4 hours to review, resolve conflicts, test.

### #42 — i18n/Translation tools
**Author:** Prolls
**What:** 7 new tools for ABAP translation management. New `pkg/adt/i18n.go`, `handlers_i18n.go`, 6 tests.
**Quality:** Well-structured (874 additions). Follows existing patterns. Closes #40.
**Strategic alignment:** Good. Common DevOps need.
**Merge recommendation:** ✅ Merge after review. May need tool registration updates for our new structure.
**Effort:** 1 hour to review and adapt registration.

### #41 — gCTS tools
**Author:** Prolls
**What:** 10 new tools for git-enabled CTS. New `pkg/adt/gcts.go`, `handlers_gcts.go`, 11 tests.
**Quality:** Well-structured (1,026 additions). Closes #39.
**Strategic alignment:** Good for cloud/BTP customers. Complements abapGit.
**Merge recommendation:** ✅ Merge after review. Same adaptation needed as #42.
**Effort:** 1 hour.

---

## 8. Open PRs — Draft/Blocked

### #66 — Integration test infrastructure overhaul
**Author:** marianfoo
**Status:** Draft. Part A of larger effort.
**What:** Overhauls integration test suite for Docker-based SAP Cloud Developer Trial. 23 new tests, helper functions, bug fixes.
**Strategic alignment:** Excellent. Automated testing is critical for quality with growing contributor base.
**Blockers:** None, but draft status suggests still in progress.
**Effort:** Review when author marks ready.

### #65 — Docker support
**Author:** marianfoo
**Status:** Draft. Blocked on HTTP transport (#38).
**What:** Dockerfile, GitHub Actions for GHCR, documentation.
**Strategic alignment:** High. Docker is the standard for cloud deployment.
**Blockers:** Needs HTTP streamable transport. STDIO doesn't work well in containers.

### #64 — Future plans
**Author:** marianfoo
**Status:** Draft. Planning document, not code.

### #63 — MkDocs documentation site
**Author:** marianfoo
**What:** Full documentation website with MkDocs Material theme. 15 doc pages, GitHub Actions auto-deploy.
**Strategic alignment:** Good. Professional docs attract enterprise users.
**Merge recommendation:** ⚠️ Review content accuracy. Large (2,562 additions).
**Effort:** 1-2 hours to review content.

---

## 9. Merged PRs (Recent) — Contributors

| PR | Author | What | Date |
|----|--------|------|------|
| #72 | oisee | Strategic decomposition + one-tool + CLI | 2026-03-18 |
| #68 | dominik-kropp | Fix ExportToFile for function modules | 2026-03-18 |
| #67 | AndreaBorgia-Abo | Fix class name reference | 2026-03-18 |
| #61 | marianfoo | Automated release workflow | 2026-03-13 |
| #60 | marianfoo | GetDependencyZIP function | 2026-03-12 |
| #59 | thm-ma | CLI source --parent/--include/--method | 2026-03-18 |
| #36 | kts982 | EditSource ignore_warnings | 2026-03-18 |
| #35 | kts982 | 401 auto-retry | 2026-03-13 |
| #25 | marianfoo | DebuggerGetVariables schema fix | 2026-03-12 |
| #20 | ingenium-it-engineering | Package $ name fix | 2026-02-04 |
| #14 | kts982 | Transport API fix + EditSource transport | 2026-02-01 |
| #6 | vitalratel | MoveObject + WebSocket refactor | 2026-01-07 |
| #4 | vitalratel | RunReport background jobs | 2026-01-07 |
| #3 | vitalratel | CLI mode + method breakpoints | 2026-01-06 |

**8 unique contributors:** oisee, marianfoo, kts982, dominik-kropp, AndreaBorgia-Abo, thm-ma, vitalratel, ingenium-it-engineering

---

## 10. Strategic Priorities

### Immediate (this week)
1. **Merge #37, #44, #53** — easy wins, no conflicts
2. **Adapt and merge #62** — readonly mode (30 min)
3. **Close #43** — documentation update
4. **Verify #26** — may be fixed by #70

### Short-term (next 2 weeks)
5. **Review and merge #38** — mcp-go upgrade. Critical path for Docker.
6. **Review and merge #42, #41** — i18n and gCTS tools from community
7. **Review #63** — documentation site
8. **Fix #55** — RunReport APC workaround

### Medium-term (next month)
9. **Phase 3** — WASM ABAP parser integration
10. **Merge #65** — Docker support (after #38)
11. **Merge #66** — Integration test infrastructure
12. **Address #27** — AFF object types

### Backlog
- #30 — Cookie auth docs
- #2 — GUI debugger improvements
- #45, #46 — Sync script enhancements
- Cross-system `vsp copy --from source -s target` command

---

## 11. Project Health

| Metric | Value | Trend |
|--------|-------|-------|
| Open issues | 13 | ↓ (was 19) |
| Open PRs | 12 | Stable |
| Contributors (active) | 8 | ↑ |
| Test count | 244 unit + 34 integration | Stable |
| Tool count | 122 (granular) / 1 (universal) | ↑ New mode |
| CLI commands | 12 | ↑ (was 5) |
| Code size | ~15K LOC Go | Stable (decomposed, not grown) |

The project is healthy. Community is active with quality contributions. The decomposition work keeps the codebase maintainable as it grows.

---

---

# Part II — Русский перевод

## 1. Краткое содержание

VSP (vibing-steampunk) — нативный Go MCP-сервер для SAP ABAP Development Tools (ADT). По состоянию на 18 марта 2026:

- **122 инструмента** в режимах focused (81) и expert (122)
- **8 уникальных контрибьюторов** объединили PR
- **13 открытых issue**, 4 бага, 6 запросов на фичи
- **12 открытых PR** от 7 контрибьюторов
- **20 issue закрыто** за последний месяц
- **14 PR смержено** за последний месяц

За сегодняшнюю сессию: стратегическая декомпозиция (Фаза 1), режим одного инструмента (Фаза 2), CLI DevOps поверхность (Фаза 4), 4 PR от сообщества смержены, 5 багов исправлены, 6 issue закрыты.

---

## 2. Закрытые issue (Решённые)

### #69 — Отсутствует файл лицензии
**Что:** README указывал MIT, но файла LICENSE не было.
**Решение:** Добавлен файл MIT LICENSE. Тривиально.

### #71 — Проверка безопасности CreatePackage падает с пустым именем пакета
**Что:** При создании транспортируемого пакета с установленным `SAP_ALLOWED_PACKAGES` проверка безопасности читала пустую строку вместо имени создаваемого пакета.
**Причина:** `checkPackageSafety` проверял `opts.PackageName` (родительский) вместо `opts.Name` для создания пакета.
**Решение:** Исправлено в `crud.go`. Трудозатраты: 15 мин.

### #70 — CreateTransport не работает на S/4HANA 757
**Что:** Неправильный endpoint (`/cts/transports`) и content type для новых систем S/4HANA.
**Причина:** Endpoint и XML-формат были для старых систем. S/4HANA 757 требует `/cts/transportrequests` с content type `transportorganizer.v1`.
**Решение:** Обновлены `transport.go` — корректный endpoint, заголовки и XML. Трудозатраты: 30 мин.

### #54 — SAP_ALLOWED_PACKAGES блокирует InstallZADTVSP
**Что:** Проблема курицы и яйца: установщику нужно создать `$ZADT_VSP`, но безопасность блокирует, потому что пакета нет в списке разрешённых.
**Решение:** Добавлен метод `AllowPackageTemporarily()`, временно добавляющий целевой пакет установки. Все обработчики установки используют его с `defer` для очистки. Трудозатраты: 30 мин.

### #52 — SyntaxCheck падает для классов с длинными пространствами имён
**Что:** Суффикс `/source/main`, добавляемый к и без того длинным URL классов с namespace, превышал лимит URI SAP.
**Решение:** SyntaxCheck теперь использует голый URL объекта для URI `checkObject`. Трудозатраты: 15 мин.

### #33 — EditSource считает предупреждения ошибками
**Что:** Любой результат проверки синтаксиса (включая предупреждения) блокировал сохранение.
**Решение:** Смержен PR #36 от kts982. EditSource теперь разделяет ошибки и предупреждения, добавлен параметр `ignore_warnings`.

---

## 3. Открытые issue — Баги

### #55 — RunReport падает в контексте APC
**Что:** Получение spool-вывода RunReport зависает, потому что все стандартные механизмы чтения spool SAP (SUBMIT, COMMIT WORK AND WAIT, CALL FUNCTION...DESTINATION, RSTS_OPEN) заблокированы внутри контекста APC WebSocket обработчика.
**Влияние:** RunReport через WebSocket ненадёжен. Пользователи получают таймауты.
**Стратегическое соответствие:** RunReport — инструмент focused-режима. Надёжность важна.
**Возможное решение:** Паттерн обёрточного отчёта + кэш-таблица. ABAP-сторона пишет spool-вывод в Z-таблицу, затем APC-обработчик его читает.
**Трудозатраты:** Средние (2-3 часа). Нужна ABAP-разработка + логика повторных попыток на Go.
**Приоритет:** Средний. Есть обходной путь (использовать варианты, RunReportAsync).

### #56 — Невозможно создать новую программу
**Что:** Пользователь сообщает «нет такой возможности» при попытке создать программу.
**Влияние:** Путаница. Вероятно, проблема режима/конфигурации (focused-режим не показывает CreateAndActivateProgram, только WriteSource с upsert).
**Возможное решение:** Улучшить сообщения об ошибках и документацию.
**Трудозатраты:** Низкие (1 час). В основном документация.
**Приоритет:** Низкий. Не баг кода — обучение пользователей.

### #43 — Отсутствующие команды
**Что:** Изначально о команде `deploy-handler`. Обновлено с вопросами о SICF и расхождениях в именах классов.
**Возможное решение:** PR #67 исправил именование. Оставшееся — пробелы в документации.
**Трудозатраты:** Низкие. Закрыть с обновлением документации.

### #26 — GetTransport не находит транспорт в ответе
**Что:** GetTransport возвращает «transport not found in response». Фича транспортов показана как отключённая.
**Возможное решение:** Может быть связано с #70. Исправление #70 может решить проблему.
**Трудозатраты:** Низкие-Средние. Возможно, уже исправлено #70.
**Приоритет:** Средний.

---

## 4. Открытые issue — Запросы фич

### #40 — Инструменты i18n/перевода
**Что:** 7 инструментов для управления переводами ABAP.
**Есть PR:** #42 (874 добавления, 6 тестов)
**Стратегическое соответствие:** Хорошее. Перевод — типичная задача DevOps.
**Трудозатраты:** Уже реализовано в PR #42. Ревью + мерж.

### #39 — Инструменты gCTS
**Что:** 10 инструментов для git-enabled CTS. CRUD репозиториев, клонирование, pull, commit, управление ветками.
**Есть PR:** #41 (1 026 добавлений, 11 тестов)
**Стратегическое соответствие:** Хорошее. gCTS — нативная Git-интеграция SAP. Важно для облачных/BTP клиентов.
**Трудозатраты:** Уже реализовано в PR #41. Ревью + мерж.

### #34 — Пагинация и схема GetTableContents
**Что:** Нет поддержки пагинации (offset). Нет способа получить схему таблицы без запроса данных.
**Есть PR:** #37 (92 добавления)
**Стратегическое соответствие:** Хорошее. Пагинация — базовое удобство.
**Трудозатраты:** Уже реализовано в PR #37. Ревью + мерж.

### #30 — Документация по cookie-аутентификации
**Что:** Нужна документация — где взять cookies, какой формат.
**Трудозатраты:** Низкие (30 мин). Добавить в README.

### #27 — Больше типов объектов (AFF/NROB)
**Что:** Поддержка объектов ABAP File Format (AFF), например NROB.
**Стратегическое соответствие:** Хорошее долгосрочно. AFF — будущее сериализации ABAP-объектов.
**Трудозатраты:** Средние. Каждый тип нуждается в маппинге URL.

### #21 — Streaming HTTP поддержка
**Что:** «Иногда STDIO недостаточно.»
**Есть PR:** #38 (обновление mcp-go до v0.43.2 со streamable HTTP)
**Стратегическое соответствие:** Критически важно для Docker (#65) и удалённого развёртывания.
**Трудозатраты:** Уже реализовано в PR #38. Масштабное обновление зависимости (37 файлов). Нужен тщательный ревью.
**Приоритет:** Высокий. Разблокирует Docker и удалённое развёртывание.

---

## 5. Открытые PR — Готовы к мержу

### #37 — Пагинация таблиц + интроспекция схемы
**Автор:** kts982. Чистый, маленький (92 добавления), протестирован на живой системе.
**Рекомендация:** ✅ Мержить. Без риска.

### #44 — Документация Quick Start для Windows
**Автор:** mv0101. Только документация (61 добавление).
**Рекомендация:** ✅ Мержить.

### #53 — API Release State для Clean Core
**Автор:** andreasmuenster. Маленький (44 добавления).
**Рекомендация:** ✅ Мержить после быстрого ревью.

### #62 — Режим Readonly
**Автор:** marianfoo. Чистая концепция (97 добавлений). Конфликтует с нашим `tools_register.go`.
**Рекомендация:** ⚠️ Нужен rebase. Адаптировать `readonlyTools` в структуру `tools_focused.go`. Затем мержить.
**Трудозатраты адаптации:** 30 мин.

---

## 6. Открытые PR — Требуют ревью

### #38 — Обновление mcp-go до v0.43.2 со streamable HTTP
**Автор:** danielheringers. Масштабное (1 149 добавлений, 37 файлов). Критический путь для Docker.
**Риск:** Высокий — меняет извлечение параметров во всех обработчиках.
**Рекомендация:** ⚠️ Нужен тщательный ревью и rebase.
**Трудозатраты:** 2-4 часа.

### #42 — Инструменты i18n/перевода
**Автор:** Prolls. Хорошо структурировано (874 добавления, 6 тестов). Закрывает #40.
**Рекомендация:** ✅ Мержить после ревью.

### #41 — Инструменты gCTS
**Автор:** Prolls. Хорошо структурировано (1 026 добавлений, 11 тестов). Закрывает #39.
**Рекомендация:** ✅ Мержить после ревью.

---

## 7. Открытые PR — Черновики/Заблокированы

### #66 — Инфраструктура интеграционных тестов (черновик)
### #65 — Docker-поддержка (заблокирован #38)
### #64 — Планы на будущее (черновик)
### #63 — Сайт документации MkDocs (2 562 добавления, нужен ревью контента)

---

## 8. Стратегические приоритеты

### Немедленно (эта неделя)
1. Смержить #37, #44, #53 — лёгкие победы
2. Адаптировать и смержить #62 — readonly режим
3. Закрыть #43 — обновление документации
4. Проверить #26 — может быть исправлено #70

### Краткосрочно (следующие 2 недели)
5. Ревью и мерж #38 — обновление mcp-go. Критический путь для Docker
6. Ревью и мерж #42, #41 — i18n и gCTS от сообщества
7. Ревью #63 — сайт документации
8. Исправить #55 — обходной путь для RunReport APC

### Среднесрочно (следующий месяц)
9. Фаза 3 — Интеграция WASM ABAP парсера
10. Мерж #65 — Docker (после #38)
11. Мерж #66 — Инфраструктура интеграционных тестов
12. Решить #27 — типы объектов AFF

### Бэклог
- #30 — Документация cookie-аутентификации
- #2 — Улучшения GUI отладчика
- #45, #46 — Улучшения скрипта синхронизации
- Кросс-системная команда `vsp copy --from source -s target`

---

## 9. Здоровье проекта

| Метрика | Значение | Тренд |
|---------|----------|-------|
| Открытые issue | 13 | ↓ (было 19) |
| Открытые PR | 12 | Стабильно |
| Контрибьюторы (активные) | 8 | ↑ |
| Количество тестов | 244 unit + 34 интеграционных | Стабильно |
| Количество инструментов | 122 (granular) / 1 (universal) | ↑ Новый режим |
| CLI команды | 12 | ↑ (было 5) |
| Размер кода | ~15K строк Go | Стабильно (декомпозировано, не выросло) |

Проект здоров. Сообщество активно и вносит качественные контрибуции. Работа по декомпозиции сохраняет поддерживаемость кодовой базы по мере роста.
