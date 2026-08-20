# Auditoria e correção dos bugs do VSP

Data da auditoria: 2026-08-15

## Limites e proteção de dados

- A investigação e os testes foram executados somente no repositório local, com mocks, nomes sintéticos e endpoints de exemplo.
- Nenhuma chamada foi feita a um sistema SAP, nenhuma credencial foi usada e nenhuma alteração SAP foi executada.
- Os documentos fornecidos serviram apenas como evidência funcional. Seu conteúdo e quaisquer dados de cliente não foram copiados para código, testes, commits ou este relatório.
- A publicação posterior foi limitada à branch `codex/vsp-bug-audit` no fork `Augusto42/vibing-steampunk` e ao draft PR [#156](https://github.com/oisee/vibing-steampunk/pull/156) contra o projeto original.

## Baseline reproduzível

| Item | Valor |
|---|---|
| Repositório | `oisee/vibing-steampunk` |
| Branch local | `codex/vsp-bug-audit` |
| Base | `83b9699bbeb42afa35baf0650d055c83c64a8eb0` |
| Descrição da base | `v2.38.1-76-g83b9699` |
| Versão reportada pelo binário sem `ldflags` | `dev (commit: unknown, built: unknown)` |
| Plataforma | Windows/amd64 |
| Go | `go1.25.0` |
| CGO | desabilitado no runtime de auditoria |
| Commit de código auditado | `064ad714f5b8c7facae70e5cc691c8acb09f0147` |
| Árvore Git do código auditado | `dddb6ed63ba1d19e1080b60a04cde2a69796e81e` |
| Descrição do código auditado | `v2.38.1-90-g064ad71` |

## Matriz de diagnóstico e correção

| Frente | Causa confirmada | Correção aplicada | Evidência automatizada | Status |
|---|---|---|---|---|
| A. Segurança da CLI | Flags de segurança pertenciam ao comando raiz não persistente e a configuração resolvida não chegava aos clientes criados pelos subcomandos. | Flags tornadas persistentes; resolução central executada antes de CLI/MCP; perfil, ambiente e flags são mesclados; todos os campos de segurança são aceitos no arquivo de sistemas. | Testes de herança, precedência e construção do cliente. | Corrigido |
| B. `writeSource` em Lua | O binding descartava opções relevantes e reduzia falhas lógicas a retorno insuficiente. | Compatibilidade com três argumentos preservada; tabela opcional encaminha modo, transporte, pacote, descrição, teste e método; falhas retornam `false, mensagem`; erros de existência deixam de converter falhas de rede/autenticação em ausência. | Testes do binding e dos modos create/update/upsert. | Corrigido |
| C. `ExecuteABAP` | Ativação sem sucesso e alertas ABAP Unit reais podiam terminar como sucesso; camadas CLI/MCP não tratavam `Success=false` como erro. | Ativação bem-sucedida passou a ser obrigatória; analisador separa o marcador de resultado de assertions/exceções reais e exige marcador de conclusão; CLI/MCP propagam falha lógica. | Testes puros para sucesso, assertion, exceção, ausência de marcador e falha de ativação. | Corrigido |
| D. Instaladores | A existência do pacote era inferida por endpoint inadequado, opções não eram integralmente repassadas e o resultado lógico do deploy não era verificado. | Consulta direta do pacote; criação idempotente com verificação; deploy compartilhado valida erro, `Success`, sintaxe, ativação e leitura posterior; objeto preexistente nunca é excluído por reconciliação automática. | Fakes para pacote existente/ausente, falha lógica, sintaxe, ativação, leitura e conflito de existência. | Corrigido |
| E. Lock, sessão, CSRF e transporte | Descoberta CSRF podia abrir contexto stateless; havia busca de pacote entre lock e escrita; `NoModification` era interpretado como proibição; `corrNr` do lock era descartado. | CSRF usa o mesmo modo stateful, com fallback HEAD→GET; pacote é validado antes do lock; somente a busca redundante é suprimida, mantendo gates de operação/transporte; `NoModification` é preservado como metadado; `corrNr` é reutilizado com whitelist e preferência ao transporte explícito; erros 400/409/423 não recebem retry cego. | Mocks de cookie, renovação 403, ordem LOCK→PUT→UNLOCK, unlock após falha, fallback/override de transporte e bloqueio fail-closed. | Corrigido |
| F. IDs de gravação | A resolução do relógio no Windows permitia IDs idênticos em gravações criadas em rajada, sobrescrevendo arquivo e índice. | O timestamp usado no ID avança atomicamente quando o relógio não progride, preservando ordenação e formato. | Teste concorrente de 4.000 IDs e 50 repetições da suíte de histórico. | Corrigido |
| G. Oracle JavaScript no Windows | O teste gravava o script em `/tmp`, caminho não portável, e ignorava erro de escrita. | `t.TempDir`, escrita verificada, `CommandContext` e skip explícito quando Node.js não está disponível. | `pkg/jseval` e `TestOracleComparison` passam no Windows. | Corrigido |
| H. Falsos sucessos em `copy` e MCP | `copy` ignorava `Success=false` em cinco tipos, terminava com código zero após falhas, criava pacote após qualquer erro de consulta e anunciava deploy WebSocket/includes ainda não implementado; o MCP serializava falha lógica de `WriteSource` como resposta normal. | Resultado nulo/lógico agora falha; erro agregado gera saída não zero; pacote só é criado após ausência conclusiva; objetos incompletos são pulados antes da escrita; MCP retorna erro de ferramenta. | Testes de resultado, resumo, pacote existente/ausente/inconclusivo, matriz de suporte e validação MCP. | Corrigido |
| I. Ativação e rename destrutivo | Vários workflows verificavam apenas erro HTTP de `Activate`; `RenameObject` escrevia no endpoint do objeto, ignorava unlock e podia excluir o objeto antigo após `Success=false`. | Validador comum converte falha lógica em erro; deploy, tabela, lote, DSL e MCP o utilizam; rename escreve em `/source/main`, exige unlock/ativação e não toca no objeto antigo após falha. | Teste sintético confirma diagnóstico, classificação em lote, endpoint correto e interrupção antes do lock/delete antigo, repetido 20 vezes. | Corrigido |
| J. Existência inconclusiva em deploy de arquivo | `DeployFromFile` tratava somente 404 como ausência, mas seguia para update após erro de autenticação, rede ou servidor. | Falhas inconclusivas encerram o fluxo antes de lock ou escrita. | Teste com erro 500 confirma uma única requisição de leitura e nenhuma mutação. | Corrigido |

## Histórico e correlação pública

Em 2026-08-15, as issues [#88](https://github.com/oisee/vibing-steampunk/issues/88), [#92](https://github.com/oisee/vibing-steampunk/issues/92), [#117](https://github.com/oisee/vibing-steampunk/issues/117), [#141](https://github.com/oisee/vibing-steampunk/issues/141), [#143](https://github.com/oisee/vibing-steampunk/issues/143) e [#144](https://github.com/oisee/vibing-steampunk/issues/144) estavam abertas. As PRs [#106](https://github.com/oisee/vibing-steampunk/pull/106) e [#138](https://github.com/oisee/vibing-steampunk/pull/138) cobriam partes do instalador.

O histórico da branch principal já continha correções parciais de sessão (`27f4d7c`, `22517d4`), mas mantinha caminhos incompletos e a interpretação incorreta de `MODIFICATION_SUPPORT`. As PRs abertas [#108](https://github.com/oisee/vibing-steampunk/pull/108), [#120](https://github.com/oisee/vibing-steampunk/pull/120) e [#125](https://github.com/oisee/vibing-steampunk/pull/125) confirmaram os mesmos pontos de ordem, CSRF e busca redundante. A implementação local incorpora esses princípios sem ignorar os gates de operação e transporte.

## Commits locais

1. `41630704550bf16ade46da76644ac7b226b8616e` — `fix(cli): propagate safety policy to subcommands`
2. `e34ba9e1f25a3ebfd6ea869211800e7375a30138` — `fix(lua): make writeSource failures explicit`
3. `d30614f4cf52064acdfee299ced9e9e4ced4919e` — `fix(execute): fail on ABAP Unit errors`
4. `057c3c66cbf3a264017f74fc6f5f51f11959f889` — `fix(install): verify package and deployed objects`
5. `60ec08e86e070fc116d812b30ac9feb110a88fba` — `fix(adt): preserve lock session and transport context`
6. `8bd5c63c470ef6b688880071e4cd8ac9d4362d5e` — `style(go): format audited files`
7. `cc9690bdf3c846f98fa7aec0437268b2856178bc` — `fix(adt): guarantee unique recording IDs`
8. `596b8cce0a10da847b877f1677e4e1c026cf669a` — `test(jseval): use portable oracle temp files`
9. `c2bb35c2fd875e6f8706ff8b78e16ad0738ccbcc` — `fix(copy): reject partial and inconclusive deployments`
10. `e9fe44d99a37be938c9bc1cd7203994e6879bcf9` — `fix(mcp): surface logical write failures`
11. `064ad714f5b8c7facae70e5cc691c8acb09f0147` — `fix(adt): stop workflows on activation failure`

## Arquivos modificados

- CLI e configuração: `cmd/vsp/main.go`, `cmd/vsp/cli.go`, `cmd/vsp/cli_extra.go`, `cmd/vsp/cli_safety_test.go`, `cmd/vsp/devops.go`, `pkg/config/systems.go`.
- Lua e source workflows: `pkg/scripting/lua.go`, `pkg/scripting/bindings.go`, `pkg/scripting/write_source_test.go`, `pkg/adt/workflows_source.go`, `pkg/adt/workflows_test.go`.
- Execução: `pkg/adt/workflows_execute.go`, `pkg/adt/workflows_execute_test.go`, `internal/mcp/handlers_help.go`, `internal/mcp/handlers_transport.go`, `internal/mcp/tools_register.go`.
- Instaladores: `internal/install/install.go`, `internal/install/install_test.go`, `internal/mcp/handlers_install.go`, `pkg/adt/client.go`, `pkg/adt/crud.go`, `pkg/adt/crud_reconcile_test.go`.
- Sessão e mutações: `pkg/adt/http.go`, `pkg/adt/http_test.go`, `pkg/adt/mutation_gate.go`, `pkg/adt/saml_auth_test.go`, `pkg/adt/workflows.go`, `pkg/adt/workflows_deploy.go`, `pkg/adt/workflows_deploy_order_test.go`, `pkg/adt/workflows_edit.go`, `pkg/adt/workflows_fileio.go`.
- Portabilidade e histórico: `pkg/adt/recorder.go`, `pkg/adt/recorder_test.go`, `pkg/jseval/oracle_test.go`.
- Segunda passada fail-closed: `cmd/vsp/copy_cmd.go`, `cmd/vsp/copy_cmd_test.go`, `internal/mcp/handlers_source.go`, `internal/mcp/handlers_source_test.go`, `internal/mcp/handlers_devtools.go`, `pkg/adt/devtools.go`, `pkg/adt/devtools_activation_test.go`, `pkg/adt/workflows_deploy_exists_test.go`, `pkg/dsl/workflow.go`.

## Verificação

Passaram:

- testes focados de `pkg/config`, `pkg/scripting`, `internal/install`, `internal/mcp`, `cmd/vsp` e `pkg/adt`;
- `go vet ./...`;
- `gofmt` em todos os arquivos Go do delta;
- `golangci-lint v1.64.8 run --new-from-rev=origin/main ./...`;
- `git diff origin/main..HEAD --check`;
- inspeção do delta para dados de cliente, usando somente fixtures sintéticas.
- 50 repetições dos testes de histórico que falhavam intermitentemente;
- teste oracle de `pkg/jseval` com diretório temporário nativo do sistema.
- 20 repetições dos cenários de ativação lógica, rename protegido, existência inconclusiva e contratos de `copy`/MCP.

A suíte `go test ./...` agora passa em todos os pacotes que não dependem de SQLite com CGO. No ambiente atual, restam somente falhas já presentes na base:

- testes SQLite de `cmd/vsp` e `pkg/cache` requerem CGO;

O problema de `/tmp` em `pkg/jseval` e a intermitência do histórico foram corrigidos, não apenas ignorados. A limitação restante de CGO não intercepta os cenários corrigidos e não foi mascarada.

## Riscos residuais

- As diferenças entre versões e topologias reais de ADT ainda exigem uma canária manual em sandbox; mocks provam a lógica do cliente, não o comportamento de cada backend.
- O fallback GET de CSRF adiciona uma requisição apenas quando HEAD falha ou não devolve token.
- O transporte herdado do lock continua sujeito ao opt-in de edição transportável e à whitelist; por projeto, ele falha fechado.
- A associação lock→transporte existe somente na memória do processo. Reiniciar o processo invalida essa associação, assim como o próprio contexto de sessão.
- `ExecuteABAP` continua sendo uma operação sensível e não deve ser usado como executor genérico em ambiente produtivo.
- O comando `copy` pula classes com includes e tipos fora da implementação ADT atual; o caminho WebSocket permanece explicitamente não implementado, sem anunciar sucesso parcial.

## Plano manual seguro em SAP

Executar apenas em sandbox descartável e autorizada, nunca com dados de cliente:

1. Criar usuário técnico dedicado, sem acesso produtivo, e confirmar logs sem corpo de source, cookies ou credenciais.
2. Rodar descoberta/autenticação somente leitura e confirmar que o cliente não imprime token nem cookie.
3. Usar um único objeto local `$TMP`, com nome e source sintéticos, sem leitura de tabelas ou chamadas externas.
4. Enviar primeiro source deliberadamente inválido e confirmar que não ocorre lock nem escrita.
5. Enviar source mínimo válido e observar uma única sequência `LOCK → PUT → UNLOCK`; conferir que não há busca stateless no intervalo e que não sobra lock.
6. Simular falhas 400/409/423 no proxy de teste e confirmar propagação sem segunda escrita.
7. Validar transporte somente em sistema de desenvolvimento dedicado, com request sintético explicitamente permitido; confirmar que transporte explícito prevalece e que fallback fora da whitelist é bloqueado antes do PUT.
8. Para `ExecuteABAP`, usar apenas constantes em memória, sem SELECT/UPDATE, exigir resultado e verificar a remoção do programa temporário.
9. Para o instalador, usar sistema descartável: testar pacote ausente e preexistente, validar ativação e leitura de source, e confirmar que uma falha nunca exclui objeto preexistente.
10. Encerrar verificando locks e objetos sintéticos e registrar somente metadados técnicos sanitizados.

## Rascunhos de pull request

Os oito blocos abaixo foram preservados como unidades lógicas de revisão. Para reduzir conflito e manter as dependências entre os fixes, eles foram publicados juntos no draft PR [#156](https://github.com/oisee/vibing-steampunk/pull/156).

### PR 1 — Propagação de segurança na CLI

**Título:** `fix(cli): propagate safety policy to subcommands`

**Resumo:** transforma flags de segurança em persistentes, resolve configuração antes de qualquer subcomando e encaminha a política ao cliente ADT. Inclui testes de herança e precedência. Relaciona-se a #117.

### PR 2 — Contrato Lua de `writeSource`

**Título:** `fix(lua): make writeSource failures explicit`

**Resumo:** preserva a chamada legada, aceita opções completas e devolve diagnóstico em falhas lógicas. A existência deixa de tratar erros indeterminados como objeto ausente.

### PR 3 — Resultado confiável de `ExecuteABAP`

**Título:** `fix(execute): fail on ABAP Unit errors`

**Resumo:** exige ativação, marcador de conclusão e ausência de assertions/exceções reais; CLI e MCP passam a falhar quando o workflow reporta `Success=false`.

### PR 4 — Instaladores verificáveis e idempotentes

**Título:** `fix(install): verify package and deployed objects`

**Resumo:** verifica pacote diretamente, preserva descrição/opções, valida resultado, sintaxe, ativação e leitura posterior, e protege objetos preexistentes. Relaciona-se às PRs #106 e #138.

### PR 5 — Sessão stateful e contexto de transporte

**Título:** `fix(adt): preserve lock session and transport context`

**Resumo:** mantém CSRF/cookie stateful, move validações de pacote para antes do lock, preserva gates locais, corrige o uso de `MODIFICATION_SUPPORT` e reutiliza `corrNr` com whitelist. Relaciona-se a #88, #92, #141, #143 e #144 e às PRs #108, #120 e #125.

### PR 6 — `copy` fail-closed

**Título:** `fix(copy): reject partial and inconclusive deployments`

**Resumo:** cria pacote somente após ausência conclusiva, propaga falhas lógicas e agregadas, e não anuncia ou executa parcialmente tipos/includes ainda sem implementação.

### PR 7 — Falhas lógicas no MCP

**Título:** `fix(mcp): surface logical write failures`

**Resumo:** converte resultados nulos ou `Success=false` de `WriteSource` em erro MCP com diagnóstico.

### PR 8 — Ativação e rename verificáveis

**Título:** `fix(adt): stop workflows on activation failure`

**Resumo:** propaga falha lógica de ativação e impede que rename/deploy/tabela/lote avancem após resultado inconclusivo ou sem sucesso.

O PR consolidado permanece em rascunho para revisão dos mantenedores. A branch publicada permite modificações por mantenedores.
