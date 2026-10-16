# Cenários de Teste Funcional — SRCOff

> Catálogo de cenários derivado de [`especificacao.md`](especificacao.md). Cada cenário cita o requisito (`RN`/`RF`) e a cobertura atual.
> **Cobertura**: `Auto` = teste automatizado existente (arquivo citado) · `Parcial` = coberto em parte · `Manual` = validar manualmente · `Gap` = sem teste.

| | |
|---|---|
| **Versão** | 1.0.0 |
| **Data** | 2026-10-11 |
| **Base** | especificacao.md §4–§11 |

Convenção de ID: `CT-<área><nº>`. Formato do cenário: *pré-condição → ação → resultado esperado*.

---

## A. Calendário e dias úteis (RN-100)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-A01 | Gerar movimento em sábado/domingo | Erro de negócio; nada é gerado | RN-100 | Auto (`dias_uteis_test`) |
| CT-A02 | Gerar movimento em 25/12 ou 01/01 (qualquer ano) | Erro de negócio; nada é gerado | RN-100 | Auto (`dias_uteis_test`) |
| CT-A03 | Gerar movimento em dia útil normal | Processa | RN-100 | Auto |
| CT-A04 | `DiaUtilAnterior` de segunda-feira | Retorna sexta anterior (pula fim de semana/feriado) | RN-100 | Auto (`dias_uteis_test`) |
| CT-A05 | Calendário de um mês com dias úteis, não úteis, futuros e processados | Cada dia classificado completo/parcial/nenhum/não útil/futuro | RF-063 | Auto (`calendario_test`) |
| CT-A06 | Calendário com `ano`/`mes` inválidos | HTTP 400 | RF-063 | Manual |

## B. Importação de posição (RF-090, RF-092)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-B01 | Upload com `preview=1` | Retorna `colunas`, `coluna_data`, `total`, amostra ≤10; não persiste | RF-090 | Manual |
| CT-B02 | Upload sem `produto`/`dominio` (fora do preview) | HTTP 400 | RF-090 | Manual |
| CT-B03 | Upload sem campo `arquivo` | HTTP 400 | RF-090 | Manual |
| CT-B04 | Upload de arquivo sem linhas de dados | HTTP 400 | RF-090 | Manual |
| CT-B05 | Upload válido | Persiste; retorna total; campos dinâmicos preservados | RF-090/RN-010.1 | Manual |
| CT-B06 | Reimportar mesma data-base | Cria nova `codigo_versao_conteudo`; geração usa a máxima | RN-090.1/RN-101 | Parcial |
| CT-B07 | Boleto com zeros à esquerda / id longo | Persistido como texto, sem perda de precisão | RN-010.2/RF-040 | Auto (`coluna_boleto_test`) |
| CT-B08 | Data-base definida pela `coluna_data` do padrão | Lote recebe a data correta | RN-090.2 | Parcial (`padrao_formato_test`) |
| CT-B09 | Upload-lote: nome do arquivo casa 1 padrão | Importa para o produto resolvido | RF-090 | Manual |
| CT-B10 | Upload-lote: nome casa N padrões | Importa para cada produto | RF-090 | Manual |
| CT-B11 | Upload-lote: arquivo ilegível | `resultados` registra erro por arquivo, sem abortar os demais | RF-090 | Manual |
| CT-B12 | Watcher sem intervalo configurado | Não varre | RF-092.1 | Manual |
| CT-B13 | Watcher com intervalo: arquivo novo na pasta | Importa, dispara contábil e gera notificações | RF-092.2 | Manual |
| CT-B14 | `GET /posicao/scan-pasta` | Retorna resumo da última varredura | RF-092.3 | Manual |
| CT-B15 | `POST /posicao/scan-pasta` | Dispara varredura imediata e retorna resumo | RF-092.3 | Manual |
| CT-B16 | Separadores por padrão (decimal/milhar/delimitador) | Valores numéricos parseados corretamente | RF-040 | Parcial (`padrao_formato_test`) |

## C. Padrões de arquivo (RF-040, RF-041)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-C01 | Listar padrões | `[PadraoArquivo...]` | RF-041 | Manual |
| CT-C02 | Criar padrão com `formato_data`/`coluna_data`/`coluna_boleto` | 201 `{id}` | RF-041/RN-041.1 | Manual |
| CT-C03 | Excluir padrão por `id` | 200; `id` inválido → 400 | RF-041 | Manual |
| CT-C04 | `ExigeMovimentoD1` com `obrigatorio_mov_d1 = nil` | Default `true` | RN-040.1 | Parcial (`obrigatoriedade_d1_test`) |
| CT-C05 | `ExigeMovimentoD1 = false` | Não exige D-1 | RN-040.1 | Auto (`obrigatoriedade_d1_test`) |
| CT-C06 | Formatos de data distintos (DD/MM vs MM/DD) | Interpretação conforme rótulo do padrão | RF-040 | Auto (`padrao_formato_test`) |

## D. Parametrizações e configurações (RF-043, RF-044)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-D01 | Listar opções de uma categoria (produto/dominio) | `[valores...]` | RF-043 | Manual |
| CT-D02 | Adicionar opção | 201 | RF-043 | Manual |
| CT-D03 | Remover opção por `categoria`+`valor` | 200 | RF-043 | Manual |
| CT-D04 | `GET /configuracoes` | Mapa chave→valor | RF-044 | Manual |
| CT-D05 | `PUT /configuracoes` sem `chave` | HTTP 400 | RF-044 | Manual |
| CT-D06 | `PUT /configuracoes` válido | Persiste; "configuração salva" | RF-044 | Manual |

## E. Regras, condições e expressões (RF-022, RF-023, RF-025)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-E01 | Criar regra (JSON) | 201 `{id}`; `tipo_lancamento` normalizado | RF-022/RN-024.1 | Parcial |
| CT-E02 | Editar regra | 200; campos atualizados | RF-022 | Manual |
| CT-E03 | Excluir regra | 200 | RF-022 | Manual |
| CT-E04 | Listar regras | `[RegraContabil...]` com condições | RF-022 | Manual |
| CT-E05 | Criar/editar/excluir condição | 201/200 | RF-023 | Parcial (`sqlite_test`) |
| CT-E06 | Validar expressão de condição (sintaxe ok) | `{valido:true}` | RF-025 | Parcial (`evaluator_test`) |
| CT-E07 | Validar expressão inválida | `{valido:false, erro, sugestao}` | RF-025 | Manual |
| CT-E08 | Campo dinâmico referenciado existe na posição | Expressão avalia normalmente | RN-025.1 | Auto (`evaluator_test`) |
| CT-E09 | `GET /posicao/campos` | Lista campos disponíveis para sugerir no cadastro | RF-091 | Manual |

## F. Import/Export CSV de regras e normalização de tipo (RF-024, RN-024)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-F01 | Exportar regras CSV | Cabeçalho com `tipo_lancamento`; valores canônicos | RF-024 | Auto (`regra_io_test`) |
| CT-F02 | Importar CSV atualizando por `id` | Atualiza; linhas idênticas não mudam | RF-024/RN-024.3 | Auto (`regra_io_http_test`) |
| CT-F03 | Importar linha com `id` inexistente | Erro na linha, sem interromper o resto | RN-024.3 | Auto (`regra_io_http_test`) |
| CT-F04 | Importar linha de criação (rótulo/`regra_id` vazio) | Cria (reimportar duplica) | RN-024.3 | Auto (`regra_io_test`) |
| CT-F05 | `tipo_lancamento` vazio no CSV | Normaliza para `reverte` | RN-024.2 | Auto (`regra_contabil_test`) |
| CT-F06 | `tipo_lancamento` com maiúsculas/espaços ("Incremental", " incremental ") | Normaliza para canônico | RN-024.1 | Auto (`regra_contabil_test`) |
| CT-F07 | `tipo_lancamento` desconhecido | Normaliza para `reverte` | RN-024.1 | Auto (`regra_contabil_test`) |
| CT-F08 | `incremental_reverte` e rótulos amigáveis ("Incremental/Reverte") no CSV | Normaliza para `incremental_reverte` | RN-024.2 | Auto (`regra_contabil_test`) |
| CT-F09 | Importar via API com `tipo_lancamento` mal formatado (maiúsculo) | Persistido canônico (não vira `reverte` silenciosamente) | RN-024.1/ADR-001 | Parcial |

## G. Geração do movimento — núcleo (RF-030, RN-110)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-G01 | Sem posição no escopo | Erro de negócio; nada gerado | RN-101 | Parcial |
| CT-G02 | Regra casa Produto **e** Domínio da posição | Aplica | RN-110.1 | Auto (`ndf_test`,`integracao_test`) |
| CT-G03 | Regra com produto/domínio vazio (todos) | Aplica a qualquer combo | RN-110.1 | Parcial |
| CT-G04 | Regra não casa produto ou domínio | Não aplica | RN-110.1 | Parcial |
| CT-G05 | Pré-condição referencia campo ausente | Inconsistência `PRE_CONDICAO`; regra inteira pulada | RN-110.2 | Parcial |
| CT-G06 | Pré-condição avalia `false` | Regra pulada sem inconsistência | RN-110.2 | Parcial |
| CT-G07 | Condição referencia campo ausente | Inconsistência `CONDICAO`; sem lançamento | RN-110.3 | Parcial |
| CT-G08 | Condição avalia `false` | Ignora condição | RN-110.3 | Auto |
| CT-G09 | `campo_valor` referencia campo ausente | Inconsistência `CAMPO_VALOR`; sem lançamento | RN-110.4 | Parcial |
| CT-G10 | Múltiplas condições ativas satisfeitas na mesma regra | Um lançamento por condição (contas distintas) | RN-110.3 | Auto (`movimento_pbt_test`) |
| CT-G11 | Condição inativa | Não gera lançamento | RN-110.3 | Parcial |
| CT-G12 | Escopo por produto/domínio no POST | Processa só o escopo | RN-102 | Parcial |
| CT-G13 | POST com data malformada | HTTP 400 | RF-030 | Manual |
| CT-G14 | Erro de negócio (data não útil/sem posição) via POST | HTTP 200 com `{mensagem}` | RF-030/§11 | Manual |

## H. Tipos de lançamento (RN-024, RN-120, RN-121)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-H01 | `reverte`: valor cheio + estorno de D-N | Lança valor cheio e gera estorno (contas invertidas) | RN-130 | Auto (`estorno_pbt_test`) |
| CT-H02 | `nao_reverte`: valor cheio, sem estorno | Lança; nenhum estorno | RN-024 | Parcial |
| CT-H03 | `incremental` 1ª ocorrência (sem D-N) | Lança valor cheio | RN-120.4 | Auto (`incremental_test`) |
| CT-H04 | `incremental` subida (1000→1300) | Lança 300 (`|1300-1000|`), sem estorno | RN-120 | Auto (`incremental_test`) |
| CT-H05 | `incremental` queda (1300→900) | Lança 400 (absoluto), sem estorno | RN-120.5 | Auto (`incremental_test`) |
| CT-H06 | `incremental` reprocesso de D0 | Desconsidera D0 (D-N < D0) | RN-120.6 | Parcial |
| CT-H07 | `incremental` D-N com padrão "exige D-1" | D-N = D-1 útil | RN-120.2 | Parcial |
| CT-H08 | `incremental` D-N com padrão "não exige D-1" | D-N = última data com movimento | RN-120.2 | Auto (`incremental_test`) |
| CT-H09 | `incremental` boleto some da posição | Saldo **permanece**, sem baixa | RN-120.7/ADR-003 | Parcial |
| CT-H10 | `incremental_reverte` cálculo diário | Igual ao `incremental` (`|D0-D-N|`) | RN-120 | Auto (`incremental_reverte_test`) |
| CT-H11 | `incremental_reverte` boleto some (subida only) | Baixa = soma líquida postada | RN-121.2 | Auto (`incremental_reverte_test`) |
| CT-H12 | `incremental_reverte` boleto some após queda (1000→1300→900) | Baixa = **1700** (soma postada), não 900 da posição | RN-121.2/ADR-007 | Auto (`incremental_reverte_test`) |
| CT-H13 | `incremental_reverte` reprocesso após baixa | Idempotente: não gera baixa nova (saldo já 0) | RN-121.4 | Auto (`incremental_reverte_test`) |
| CT-H14 | `incremental_reverte` boleto presente em D0 mas condição `false` | **Não** baixa (existe na posição) | RN-121.1 | Gap |
| CT-H15 | `incremental_reverte` combo inteiro some (sem posições D0 do combo) | Combo incluído no versionamento/execução da baixa | RN-121.5 | Gap |
| CT-H16 | `incremental_reverte` boleto reaparece após baixa | Nova série incremental (saldo anterior zerado) | RN-121 | Gap |
| CT-H17 | `incremental_reverte` combo bloqueado por D-1 | Baixa **não** processada para o combo bloqueado | RN-121.5 | Gap |
| CT-H18 | `incremental_reverte` "todo o saldo" por regra | Baixa cobre todos os pares de contas (condições) da regra | RN-121.6 | Gap |
| CT-H19 | `reverte`/`nao_reverte` inalterados pela introdução do incremental | Comportamento idêntico ao anterior | RN-120.9/ADR-005 | Auto |

## I. Obrigatoriedade de movimento D-1 (RN-140)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-I01 | Padrão exige D-1; há contábil anterior mas falta no D-1 útil | Combo **bloqueado** (não processa) | RN-140.1 | Auto (`obrigatoriedade_d1_test`) |
| CT-I02 | Padrão exige D-1; não há contábil anterior (1ª vez) | **Não** bloqueia | RN-140.2 | Auto |
| CT-I03 | Padrão exige D-1; há contábil em D-1 útil | Processa | RN-140.1 | Auto |
| CT-I04 | Padrão não exige D-1 | Nunca bloqueia por D-1 | RN-140 | Parcial |
| CT-I05 | Execução gerou 0 lançamentos (barrada por inconsistências) | Não conta como "tem contábil" | RN-140.3/RN-060.1 | Parcial |
| CT-I06 | Combo bloqueado | Lançamentos e inconsistências do combo removidos; retornado na lista de bloqueios | RN-160.4 | Auto |

## J. Estorno clássico (RN-130, RN-131)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-J01 | Estorno inverte débito/crédito, mesmo valor, `indicador_reversao=true` | Linha de estorno correta | RN-130.2 | Auto (`estorno_pbt_test`) |
| CT-J02 | Data de origem: padrão exige D-1 → D-1 útil | Estorna lançamentos do D-1 útil | RN-131 | Parcial |
| CT-J03 | Data de origem: não exige D-1 → maior data anterior com movimento | Estorna dessa data | RN-131 | Parcial |
| CT-J04 | Sem execução anterior (sem data de origem) | **Não** estorna | RN-131 | Auto |
| CT-J05 | Regra `nao_reverte`/`incremental`/`incremental_reverte` | Não gera estorno clássico de D-1 | RN-130.1 | Parcial |
| CT-J06 | `POST /estorno` avulso | "estorno gerado com sucesso" | RN-130 | Manual |

## K. Versionamento e reprocesso (RN-150)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-K01 | 1ª execução de um combo na data | Versão = 1 | RN-150.2 | Parcial |
| CT-K02 | Reprocessar a mesma data/combo | Nova versão = max+1 | RN-150.1 | Parcial |
| CT-K03 | Dois combos na mesma data com versões diferentes | Cada combo versiona independente | RN-150.2 | Parcial |
| CT-K04 | Lançamentos e estornos da execução | Mesma versão do combo | RN-150.3 | Auto |

## L. Consulta de movimento (RF-050, RN-050)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-L01 | Consulta por data única | Página de lançamentos | RF-050 | Parcial |
| CT-L02 | Consulta por período (data_inicio/fim) | Filtra pelo intervalo | RF-050 | Auto (`consulta_pbt_test`) |
| CT-L03 | Filtro por boleto | Só o boleto | RF-050 | Parcial |
| CT-L04 | Filtro por produto/domínio | Só o escopo | RF-050 | Parcial |
| CT-L05 | `versao_modo=vigente` | Só a maior versão por combo/data | RN-050.2 | Auto |
| CT-L06 | `versao_modo=especifica` + `versao` | Só a versão pedida | RF-050 | Parcial |
| CT-L07 | Par lançamento+estorno de saldo zero | **Oculto** na consulta | RN-050.1 | Auto (`execucao_alinhada_test`) |
| CT-L08 | Paginação (pagina/tamanho) | Total e página corretos | RF-050 | Parcial |
| CT-L09 | Datas malformadas | HTTP 400 | RF-050 | Manual |

## M. Exportações (RF-051, RF-052)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-M01 | Export CSV do filtro | CSV com todos os registros (sem paginação) | RF-051 | Manual |
| CT-M02 | Export TXT por data | Layout fixo (cabeçalho/detalhe/totalizador), vigente sem cancelados | RF-052 | Manual |
| CT-M03 | Export TXT sem dados | `{sem_dados:...}` | RF-052 | Manual |
| CT-M04 | Export respeita `versao_modo` | Reflete versão escolhida | RF-051 | Manual |

## N. Exclusão de movimento (RF-053)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-N01 | Excluir por data+versão | Remove a versão; "movimento excluído" | RF-053 | Parcial |
| CT-N02 | Excluir data sem versão | Comportamento conforme serviço (versão 0) | RF-053 | Manual |
| CT-N03 | Data malformada | HTTP 400 | RF-053 | Manual |

## O. Status e execução (RF-062, RN-160)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-O01 | Status por data | Processados × Pendentes por combo | RF-062 | Manual |
| CT-O02 | Contagem da execução bate com a consulta | `qtd_lancamentos/qtd_estornos` = visíveis (sem cancelados) | RN-160.3 | Auto (`execucao_alinhada_test`) |
| CT-O03 | `qtd_movimento` bruto | Conta lançamentos não-estorno gerados | RN-060.1 | Parcial |
| CT-O04 | Combo só com baixa (incremental_reverte) | Execução registrada mesmo sem lançamento de D0 | RN-121.5 | Gap |

## P. Inconsistências (RF-064, RN-160.2)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-P01 | Listar inconsistências por data | `[InconsistenciaProcessamento...]` | RF-064 | Manual |
| CT-P02 | Export CSV de inconsistências | CSV | RF-064 | Manual |
| CT-P03 | Reprocesso substitui inconsistências só do escopo | `SubstituirPorEscopo` não afeta outros combos | RN-160.2 | Parcial |

## Q. Conciliação clássica (RF-080)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-Q01 | Posição sem movimento correspondente | Inconsistência `POSICAO_SEM_MOVIMENTO` | RF-080 | Manual |
| CT-Q02 | Lançamento duplicado | Inconsistência `LANCAMENTO_DUPLICADO` | RF-080 | Manual |
| CT-Q03 | Conciliação sem divergências | Resultado limpo | RF-080 | Manual |
| CT-Q04 | `data` malformada | HTTP 400 | RF-080 | Manual |

## R. Conciliação IA e NL Query (RF-081, RF-073)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-R01 | Conciliação IA sem `GEMINI_API_KEY` | HTTP 500 "não configurada" | RF-081 | Manual |
| CT-R02 | Conciliação IA sem pergunta/data | HTTP 400 | RF-081 | Manual |
| CT-R03 | Conciliação IA válida | `{diagnostico, sugestao?}` | RF-081 | Manual |
| CT-R04 | Aplicar ajuste | Lançamentos de ajuste em nova versão | RF-081 | Manual |
| CT-R05 | NL Query em backend não-sqlserver | Indisponível (handler sem `*sql.DB`) | RF-073/RNF-001.2 | Manual |
| CT-R06 | NL Query sem pergunta / sem key | HTTP 400 / 500 | RF-073 | Manual |

## S. Notificações (RF-071)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-S01 | `GET /notificacoes` | `{nao_lidas, itens}` (respeita `limite`) | RF-071 | Manual |
| CT-S02 | `POST /notificacoes` | Marca todas como lidas | RF-071 | Manual |
| CT-S03 | Importação automática | Gera `POSICAO_IMPORTADA` e `CONTABIL_EXECUTADO` | RF-092.2/RF-070 | Manual |

## T. Backends e equivalência (RNF-001)

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-T01 | CRUD de regra no SQLite | Round-trip correto; `tipo_lancamento` preservado | RNF-001.1 | Auto (`sqlite_test`) |
| CT-T02 | Mesmo cenário de geração em file/sqlite/sqlserver | Resultado observável equivalente | RNF-001.1 | Gap (file/sqlserver) |
| CT-T03 | Normalização de `tipo_lancamento` nos 3 backends (escrita) | Valor canônico em todos | RN-024.1 | Parcial |
| CT-T04 | `STORAGE_BACKEND` ausente | Default `file` | RNF-002 | Manual |
| CT-T05 | Migração 014 (posta_reverte → tipo_lancamento) no SQL Server | Backfill correto (1→reverte, 0→nao_reverte) | §12/migr. | Manual |

## U. Bordas gerais

| ID | Cenário | Resultado esperado | Req. | Cobertura |
|----|---------|--------------------|------|-----------|
| CT-U01 | Boleto único por (dia, versão) | Base incremental soma uma vez por chave | RN-120.8/ADR-004 | Auto (implícito) |
| CT-U02 | Valores grandes / casas decimais | Sem perda relevante de precisão | — | Parcial (`borda_test`) |
| CT-U03 | Expressões com campos inexistentes vs. válidas (PBT) | Comportamento estável | RN-025 | Auto (`pbt_test`) |
| CT-U04 | Múltiplos combos numa única geração | Versionamento/execução por combo corretos | RN-150 | Auto (`movimento_pbt_test`) |

---

## Gaps prioritários (sem automação hoje)

1. **CT-H14** — `incremental_reverte`: boleto presente na posição mas condição `false` **não** deve baixar (gatilho por posição, não por lançamento). *(RN-121.1)*
2. **CT-H17** — baixa não deve ocorrer em combo bloqueado por D-1. *(RN-121.5)*
3. **CT-H15 / CT-O04** — combo que some inteiro: baixa + versionamento + execução. *(RN-121.5)*
4. **CT-H18** — baixa cobrindo múltiplos pares de contas (regra com várias condições). *(RN-121.6)*
5. **CT-H16** — reaparecimento do boleto após baixa (nova série). *(RN-121)*
6. **CT-T02** — testes de equivalência entre os 3 backends para o mesmo cenário de geração.

> Convenção: ao fechar um gap, criar o teste citando o `CT-xx` e o `RN/RF`, e atualizar a coluna Cobertura.
