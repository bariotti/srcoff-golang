# Especificação Funcional e Técnica — SRCOff

> **SRCOff** — Sistema de Roteirização Contábil Offshore.
> Documento normativo (fonte da verdade) para desenvolvimento orientado a especificação (SDD).

| | |
|---|---|
| **Versão da spec** | 1.0.0 |
| **Data** | 2026-10-09 |
| **Status** | Vigente |
| **Escopo** | POC funcional (backends file / sqlite / sqlserver) |
| **Idioma** | Português (BR) — alinhado ao código |

---

## 0. Como usar esta especificação (processo SDD)

Esta especificação é a **referência primária** do comportamento do sistema. A partir dela:

1. **Mudança começa na spec.** Qualquer nova funcionalidade ou alteração de comportamento é primeiro descrita aqui (requisito novo ou alteração de requisito existente) e só então implementada.
2. **Rastreabilidade.** Todo requisito tem um identificador estável:
   - `RF-NNN` — Requisito Funcional (o que o sistema faz).
   - `RN-NNN` — Regra de Negócio (como a lógica decide).
   - `RNF-NNN` — Requisito Não-Funcional (qualidade, restrição técnica).
   - `ADR-NNN` — Decisão de arquitetura/negócio registrada (seção 12).
   Commits, PRs e testes devem citar o(s) ID(s) que implementam/validam.
3. **Critérios de aceitação.** Requisitos com comportamento observável trazem critérios em formato _Dado/Quando/Então_, que devem ter teste automatizado correspondente.
4. **IDs são imutáveis.** Não reaproveite um ID aposentado; marque como `DEPRECADO` e crie um novo.
5. **Divergência spec × código é bug.** Se o código divergir da spec, decida qual está certo: ou corrige o código, ou atualiza a spec (com entrada em ADR). Nunca deixe os dois em conflito silencioso.

> Template para novos requisitos no final do documento (seção 13).

---

## 1. Visão geral

O SRCOff transforma **posições de carteira** (importadas de arquivos) em **lançamentos contábeis**, aplicando **regras configuráveis** por Produto e Domínio. O processamento é diário, só ocorre em **dias úteis**, é **versionado por combinação (produto, domínio)** e suporta três estratégias de lançamento: **Posta/Reverte**, **Posta/Não Reverte** e **Incremental**.

Além da geração, o sistema oferece consulta/extração do movimento, conciliação (inclusive assistida por IA), gestão de regras, padrões de arquivo, parametrizações, importação manual/automática (pasta monitorada) e notificações.

### 1.1 Atores

| Ator | Descrição |
|------|-----------|
| **Operador contábil** | Importa posições, dispara o contábil, consulta e extrai resultados, concilia. |
| **Administrador de regras** | Cadastra/edita regras, condições, padrões de arquivo e parametrizações. |
| **Watcher (sistema)** | Processo em segundo plano que varre pastas monitoradas, importa e dispara o contábil automaticamente. |

---

## 2. Glossário

| Termo | Definição |
|-------|-----------|
| **Posição de carteira** | Registro de uma operação/boleto em uma data-base, com campos dinâmicos (`map[string]interface{}`). |
| **Boleto** | Identificador da operação (campo configurável na condição; persistido como texto). |
| **Produto** | Classificação corporativa da operação (ex.: NDF, SWAP). |
| **Domínio** | Natureza do processamento (ex.: Posição, Liquidação). |
| **Combinação / combo** | Par (Produto, Domínio). Unidade de versionamento e de processamento. |
| **Regra contábil** | Conjunto de condições que, aplicadas a uma posição, geram lançamentos. |
| **Condição** | Dentro de uma regra: expressão + contas (débito/crédito) + campos (valor, moeda, boleto). |
| **Lançamento** | Linha contábil gerada (`indicador_reversao = false`). |
| **Estorno** | Reversão de um lançamento (contas invertidas, `indicador_reversao = true`). |
| **D0** | Data de processamento. |
| **D-1 útil** | Dia útil imediatamente anterior a D0. |
| **D-N** | Data-base de referência para estorno/incremental (D-1 útil ou a maior data anterior com movimento). |
| **Versão (`codigo_versao_conteudo`)** | Número incremental por (data, produto, domínio) a cada reprocessamento. |
| **Vigente** | Maior versão existente de uma combinação em uma data. |
| **Par de saldo zero** | Lançamento + estorno de mesmo valor/contas invertidas que se cancelam e ficam **ocultos** na consulta. |
| **Inconsistência** | Falha de avaliação (campo ausente/erro) que impede a geração de um lançamento específico. |

---

## 3. Arquitetura

### 3.1 Componentes

Arquitetura em camadas: **model → repository → service → handler → frontend**.

- **API** (`cmd/api`): servidor HTTP, injeção de dependências, roteamento.
- **Frontend** (`cmd/frontend`): SPA servida por templates Go (`templates/*.html`), consome a API.
- **Evaluator** (`internal/evaluator`): avaliação de expressões com [expr-lang](https://github.com/expr-lang/expr).
- **Services** (`internal/service`): regra de negócio.
- **Repositories** (`internal/repository`): persistência, com 3 implementações intercambiáveis.

### 3.2 Backends de armazenamento — RNF-001

O backend é selecionado por `STORAGE_BACKEND`:

| Valor | Implementação | Detalhe |
|-------|---------------|---------|
| `file` (**default**) | `internal/repository/file` | JSON em `FILE_STORAGE_DIR` (default `./data`). |
| `sqlite` | `internal/repository/sqlite` | `modernc.org/sqlite` (puro Go), arquivo em `SQLITE_PATH` (default `./srcoff.db`). Datas como TEXT, placeholders `?`. |
| `sqlserver` | `internal/repository` | `go-mssqldb`, driver `sqlserver`, placeholders `@pN`. |

**RNF-001.1** As três implementações DEVEM satisfazer os mesmos contratos (`internal/repository/interfaces.go`) e produzir **comportamento observável equivalente**. Qualquer teste de comportamento de repositório deve valer para as três.

**RNF-001.2** O NL Query (RF-073) depende de `*sql.DB` cru e só está disponível no backend `sqlserver`; nos demais o handler recebe `nil`.

### 3.3 Variáveis de ambiente — RNF-002

| Variável | Default | Efeito |
|----------|---------|--------|
| `STORAGE_BACKEND` | `file` | Seleciona o backend. |
| `FILE_STORAGE_DIR` | `./data` | Diretório do backend file. |
| `SQLITE_PATH` | `./srcoff.db` | Arquivo do backend sqlite. |
| `API_PORT` | `8080` | Porta HTTP da API. |

---

## 4. Modelo de dados (contrato)

Entidades em `internal/model`. Os nomes JSON são o contrato de API.

### 4.1 PosicaoCarteira — RF-010
```
id                     int64
data_posicao_carteira  time   (data-base)
codigo_versao_conteudo int    (versão do import na data)
campos                 map[string]interface{}  (campos dinâmicos do arquivo)
```
- **RN-010.1** Os campos são **dinâmicos**: dependem do arquivo importado. Nomes normalizados para snake_case.
- **RN-010.2** O número do boleto é sempre persistido como **texto** (preserva zeros à esquerda e precisão), conforme `coluna_boleto` do padrão.

### 4.2 RegraContabil — RF-020
```
id                          int64
descricao                   string   (obrigatório)
codigo_produto_corporativo  string   (lista separada por vírgula: "NDF,SWAP"; vazio = todos)
dominio                     string   (lista separada por vírgula; vazio = todos)
campo_produto               string   (nome do campo da posição comparado ao produto)
pre_condicao                string   (expressão opcional, combinada com AND a cada condição)
ativo                       bool
tipo_lancamento             string   (reverte | nao_reverte | incremental)   ← RN-024
condicoes                   []CondicaoRegra
```

### 4.3 CondicaoRegra — RF-021
```
id            int64
id_regra      int64
condicao      string   (expressão booleana; obrigatória)
conta_debito  string   (obrigatória)
conta_credito string   (obrigatória)
campo_valor   string   (expressão/campo de valor; obrigatório)
campo_moeda   string   (campo de moeda; obrigatório)
campo_boleto  string   (campo usado como identificador do boleto no lançamento)
ativo         bool
```

### 4.4 LancamentoContabil — RF-030
```
id, data_lote_contabil, codigo_versao_conteudo,
codigo_identificador_boleto, valor_lancamento_contabil, moeda_lancamento_contabil,
conta_debito, conta_credito, produto, dominio,
indicador_reversao (false=lançamento, true=estorno),
descricao_regra_contabil, descricao_condicao_contabil, id_regra_contabil
```

### 4.5 PadraoArquivo — RF-040
```
id, padrao (máscara do nome do arquivo), produto, dominio,
delimitador (";"/","/auto), separador_decimal, separador_milhar,
formato_data (rótulo, ex. "DD/MM/AAAA"; obrigatório no cadastro novo),
coluna_data   (coluna que define a data-base; obrigatória no cadastro novo),
coluna_boleto (coluna do boleto, importada como texto; obrigatória no cadastro novo),
obrigatorio_mov_d1 *bool (nil=true)   ← RN-053
```
- **RN-040.1** `ExigeMovimentoD1()` = `obrigatorio_mov_d1 == nil || *obrigatorio_mov_d1` (default **true**).

### 4.6 MovimentoExecucao — RF-060
```
id, data_lote, produto, dominio,
qtd_lancamentos (VISÍVEL, sem cancelados),
qtd_estornos    (VISÍVEL),
qtd_movimento   (BRUTO: nº de lançamentos não-estorno gerados),
criado_em
```
- **RN-060.1** `qtd_movimento` é a base para saber se a data "tem contábil", independentemente de pares de saldo zero zerarem `qtd_lancamentos`.

### 4.7 InconsistenciaProcessamento — RF-061
```
id, data_lote_contabil, codigo_identificador_boleto, produto, dominio,
id_regra_contabil, descricao_regra_contabil,
tipo (PRE_CONDICAO | CONDICAO | CAMPO_VALOR), expressao, campos_faltantes, detalhe, criado_em
```

### 4.8 Notificacao — RF-070
```
id, tipo (POSICAO_IMPORTADA | CONTABIL_EXECUTADO), data_lote, produto, dominio, mensagem, lida, criado_em
```

### 4.9 Outras
- **Parametrização**: opções de combos por categoria (ex.: `produto`, `dominio`).
- **Configuracao**: pares chave→valor (ex.: pastas monitoradas, intervalo do watcher).
- **Conciliacao / Inconsistencia (conciliação)**: ver seção 7.

---

## 5. Motor de geração do movimento contábil (núcleo)

Serviço: `MovimentoContabilService.GerarMovimentoEscopo(ctx, data, produtoFiltro, dominioFiltro)`.
`GerarMovimento` = escopo vazio (todas as combinações).

### 5.1 Pré-condições

- **RN-100 (Dia útil).** O contábil só processa em **dia útil**. Dia **não** útil = sábado, domingo, 25/12 (Natal) e 01/01 (Ano Novo), independentemente do ano. Processar data não útil → erro, nada é gerado.
  - `EhDiaUtil`, `DiaUtilAnterior` em `internal/service/dias_uteis.go`.
- **RN-101 (Posição vigente).** Usa sempre a **versão máxima** da posição para a data (`BuscarPorDataEVersaoMaxima`). Sem posição no escopo → erro.
- **RN-102 (Escopo).** Filtros de produto/domínio (quando informados) restringem as posições processadas.

### 5.2 Aplicação de regras — RN-110

Para cada posição e cada regra ativa:
1. **RN-110.1** A regra só se aplica se **Produto E Domínio** da posição casarem com a regra (`regraAplicaAPosicao`). Lista vazia na regra = casa com todos.
2. **RN-110.2** `pre_condicao` (se houver) é avaliada uma vez por posição/regra; se referenciar campo ausente → **inconsistência `PRE_CONDICAO`** e a regra inteira é pulada para aquela posição; se avaliar `false` → regra pulada (sem inconsistência).
3. Para cada **condição ativa**:
   - **RN-110.3** Campo ausente em `condicao` → **inconsistência `CONDICAO`**, sem lançamento.
   - Condição `false` → ignora.
   - **RN-110.4** Campo ausente em `campo_valor` → **inconsistência `CAMPO_VALOR`**, sem lançamento.
   - Caso contrário gera **um lançamento** com: `valor = EvaluateValue(campo_valor)`, `moeda = CampoString(campo_moeda)`, `boleto = CampoString(campo_boleto|padrão)`, contas da condição, produto/domínio da posição, `indicador_reversao=false`.

### 5.3 Tipo de lançamento — RN-024 / ADR-001

`tipo_lancamento` (combo **obrigatório**) define a estratégia. Normalização canônica única: `model.NormalizaTipoLancamento` (trim + lower; vazio/desconhecido ⇒ `reverte`). Helpers: `EhReverte()` (inclui vazio), `EhIncremental()`.

| Tipo | Estorno (RN-130)? | Valor do lançamento em D0 |
|------|-------------------|---------------------------|
| **`reverte`** (Posta/Reverte) | **Sim** | Valor cheio de `campo_valor`. |
| **`nao_reverte`** (Posta/Não Reverte) | Não | Valor cheio de `campo_valor`. |
| **`incremental`** | Não | `\| \|valor_D0\| − \|valor_D-N\| \|` (RN-120). |

- **RN-024.1** Normalização aplicada em **toda escrita** (API/CSV/3 backends) e na exportação; o valor persistido é sempre canônico, independentemente de maiúsculas/espaços. (ADR-001)

### 5.4 Cálculo Incremental — RN-120

Para regras `incremental`, o valor lançado em D0 é a **diferença absoluta** entre o valor de D0 e o valor de D-N:

```
valor_lançado = | |valor_D0| − |valor_D-N| |
```

- **RN-120.1 (Chave da série).** A série incremental é identificada por **(produto, domínio, boleto, id_regra, conta_débito, conta_crédito)**.
- **RN-120.2 (Data-base D-N).** D-N é a **mesma data-base do estorno** (RN-131): D-1 útil se o padrão exige D-1; senão a **maior data anterior com movimento**.
- **RN-120.3 (Origem do valor_D-N).** `valor_D-N` é obtido **relendo a posição de D-N e reavaliando `campo_valor`** (não é a soma de lançamentos postados). Por isso reflete "Posição D0 − Posição D-N". (ADR-002)
- **RN-120.4 (Primeira ocorrência).** Sem D-N (nenhuma execução anterior) → base 0 → lança o **valor cheio**.
- **RN-120.5 (Queda / absoluto).** O valor lançado é **absoluto** (sem sinal), inclusive quando o valor cai (ex.: D-N 1300, D0 900 → lança 400). A persistência mantém o valor como calculado.
- **RN-120.6 (Reprocessamento).** D-N é sempre estritamente anterior a D0, então reprocessar D0 **desconsidera** D0.
- **RN-120.7 (Baixa por desaparecimento — fora de escopo).** Se um boleto existe em D-N mas **não** em D0, nenhum lançamento é gerado e o saldo contábil **permanece** (sem baixa automática). Comportamento correto por decisão de negócio. (ADR-003)
- **RN-120.8 (Unicidade de posição).** Assume-se **no máximo uma posição por (boleto, dia, versão)**; portanto a acumulação da base soma exatamente uma vez por chave. (ADR-004)
- **RN-120.9** As regras `reverte` e `nao_reverte` **não** têm sua lógica alterada pelo incremental. (ADR-005)

Implementação: bloco "2b" de `GerarMovimentoEscopo` + `acumularBaseIncremental` + `chaveIncremental`.

### 5.5 Obrigatoriedade de movimento de D-1 — RN-140

Quando o padrão da combinação **exige D-1** (RN-040.1) e há movimento contábil em alguma data anterior:
- **RN-140.1** Se **não** houver movimento no **D-1 útil**, a combinação é **bloqueada** (não processa lançamentos nem inconsistências naquela data).
- **RN-140.2** Se não há movimento anterior nenhum (primeira vez) → **não** bloqueia.
- **RN-140.3** A base é o **movimento real** (`DatasComMovimento`, `qtd_lancamentos > 0`), não o mero log de execução. (RN-060.1)

### 5.6 Estorno — RN-130 / RN-131

- **RN-130.1** Apenas regras `EhReverte()` geram estorno. `nao_reverte` e `incremental` **nunca** estornam.
- **RN-130.2** O estorno copia o lançamento de origem **invertendo** débito↔crédito, com `indicador_reversao = true` e valor igual.
- **RN-131 (Data de origem do estorno por combinação).** `dataEstornoPorCombo`:
  - Exige D-1 → **D-1 útil**.
  - Não exige D-1 → **maior data anterior com movimento** (`DatasComMovimento`).
  - Sem data de origem (nenhuma execução anterior) → **não estorna**.

### 5.7 Versionamento por combinação — RN-150

- **RN-150.1** A versão (`codigo_versao_conteudo`) é calculada **por (produto, domínio)**: `max(versão vigente da combinação) + 1`.
- **RN-150.2** Cada combinação versiona de forma **independente**: a 1ª execução de um novo combo na data começa em **v1**, mesmo que outro combo já esteja em versão maior.
- **RN-150.3** Lançamentos e estornos da mesma execução recebem a mesma versão do combo.

### 5.8 Persistência e registro — RN-160

- **RN-160.1** Lançamentos + estornos são persistidos em um único `BulkInsert`.
- **RN-160.2** Inconsistências são persistidas com `SubstituirPorEscopo` (substitui apenas as combinações do escopo na data).
- **RN-160.3** Para cada combinação é registrada uma **execução** (RF-060) com `qtd_lancamentos`/`qtd_estornos` **visíveis** (= consulta sem cancelados) e `qtd_movimento` **bruto**.
- **RN-160.4** Retorna a lista de combinações **bloqueadas** por D-1 (não processadas); as demais são processadas normalmente.

#### Critérios de aceitação (núcleo)
- **CA-incremental** (`internal/service/incremental_test.go`): posições 1000/1300/900 em dias consecutivos, padrão sem D-1 obrigatório → lança **1000 → 300 → 400**, todos **sem estorno**.
- **CA-execucao-consulta** (`execucao_alinhada_test.go`): a contagem gravada na execução bate com a consulta (pares de saldo zero ocultos).
- **CA-D1** (`obrigatoriedade_d1_test.go`): bloqueio quando falta D-1 útil e já há contábil anterior.

---

## 6. Consulta e extração do movimento

### 6.1 Consulta — RF-050
- **RF-050.1** Consulta paginada com filtros: intervalo de datas, boleto, versão, modo de versão (`vigente` ou versão específica), e escopo produto/domínio.
- **RN-050.1 (Ocultar cancelados).** A consulta "fonte da verdade" (`ConsultarPaginadoFiltradoSemCancelados`) **oculta** pares lançamento+estorno de saldo zero.
- **RN-050.2 (Vigente).** Modo `vigente` mostra apenas a maior versão por combinação/data.

### 6.2 Extrações — RF-051 / RF-052
- **RF-051** Exportação CSV do movimento (`/api/v1/movimento-contabil/export`).
- **RF-052** Exportação TXT de layout fixo (`/export-txt`). Formato descrito no README (seção "Formato do Arquivo TXT").

### 6.3 Exclusão — RF-053
- **RF-053** Exclusão de movimento por data e versão (`DELETE /api/v1/movimento-contabil`).

---

## 7. Conciliação

### 7.1 Conciliação clássica — RF-080
Compara posição × movimento e aponta inconsistências:
- `POSICAO_SEM_MOVIMENTO` — posição sem lançamento correspondente.
- `LANCAMENTO_DUPLICADO` — lançamento duplicado.

### 7.2 Conciliação com IA (Gemini) — RF-081
- **RF-081.1** Analisa divergências e sugere ajustes (`/api/v1/conciliacao-ia`).
- **RF-081.2** Aplicação de ajuste gera lançamentos de ajuste em nova versão (`/conciliacao-ia/ajuste`, `BulkInsertAjuste`).
- **RNF-081** Requer chave/credenciais do provedor (ver README). Integração externa — detalhe de payload no código.

---

## 8. Regras, padrões e parametrizações (administração)

### 8.1 Regras e condições — RF-020..RF-023
- **RF-022** CRUD de regras (`/api/v1/regras`, `/api/v1/regras/{id}`).
- **RF-023** CRUD de condições (`/api/v1/regras/{id}/condicoes`, `/api/v1/condicoes/{id}`).
- **RF-024 (Import/Export CSV).** Export (`/regras/export`) e import (`/regras/import`) de regras+condições em um único CSV.
  - **RN-024.2** Coluna `tipo_lancamento` aceita `reverte | nao_reverte | incremental` (vazio = `reverte`); também aceita rótulos amigáveis de "não reverte". Normalização via `model.NormalizaTipoLancamento`.
  - **RN-024.3** Casamento por `id`; linhas idênticas não são alteradas; import **nunca exclui**; linhas de criação (rótulo/`regra_id` vazio) criam a cada importação (risco de duplicata — reexportar após criar).

### 8.2 Expressões — RF-025 / RN-025
- **RN-025.1** Condições e `campo_valor` usam expr-lang sobre os campos da posição (`PosicaoToEnv`).
- **RN-025.2** Campo referenciado e ausente na posição ⇒ inconsistência (RN-110), não erro fatal.
- **RF-025** Validação de expressão sob demanda (`/api/v1/validar-expressao`).

### 8.3 Padrões de arquivo — RF-040..RF-042
- **RF-041** CRUD de padrões (`/api/v1/parametrizacoes/padroes`).
- **RN-041.1** Campos obrigatórios no cadastro novo: `formato_data`, `coluna_data`, `coluna_boleto` (legados podem estar vazios = comportamento automático).

### 8.4 Parametrizações e configurações — RF-043 / RF-044
- **RF-043** Opções de combos (produto, domínio) (`/api/v1/parametrizacoes/opcoes`).
- **RF-044** Configurações chave→valor, incl. pastas monitoradas (`/api/v1/configuracoes`).

---

## 9. Importação de posição

### 9.1 Manual — RF-090
- **RF-090.1** Upload de um arquivo (`/api/v1/posicao/upload`) e em lote (`/upload-lote`).
- **RN-090.1 (Versionamento do import).** Cada importação na mesma data-base cria uma nova `codigo_versao_conteudo`; a geração usa a máxima (RN-101).
- **RN-090.2 (Data-base).** Definida pela `coluna_data` do padrão; parsing conforme `formato_data`/separadores do padrão.
- **RF-091** Consulta de campos disponíveis (`/api/v1/posicao/campos`) e listagem (`/api/v1/posicao`).

### 9.2 Automática (watcher) — RF-092
- **RF-092.1** Varredura periódica de pastas monitoradas (intervalo parametrizável; sem parametrização, **não** varre).
- **RF-092.2** Importação automática **dispara o contábil** e **gera notificações** (`POSICAO_IMPORTADA`, `CONTABIL_EXECUTADO`).
- **RF-092.3** Status e scan sob demanda (`/api/v1/posicao/scan-pasta`).

---

## 10. Status, calendário e notificações

- **RF-062** Status de execução por data (`/api/v1/movimento-contabil/status`) — Processados × Pendentes por combinação.
- **RF-063** Calendário de execução (`/api/v1/movimento-contabil/calendario`).
- **RF-064** Listagem de inconsistências e export (`/api/v1/inconsistencias`, `/inconsistencias/export`).
- **RF-071** Notificações (`/api/v1/notificacoes`): listar, contar não lidas, marcar lidas.

---

## 11. Contrato de API REST (completo)

Base: `/api/v1`. Rotas registradas em `cmd/api/main.go`. Convenções gerais:

- **Datas**: sempre `YYYY-MM-DD`.
- **Erro**: corpo `{"erro": "<mensagem>"}`. Observação: alguns fluxos de negócio (gerar movimento/estorno) respondem **HTTP 200** com `{"mensagem": "..."}` mesmo quando a operação não produz efeito (ex.: data não útil, bloqueio D-1). O 4xx é reservado a entrada malformada.
- **Resposta de sucesso simples**: `{"mensagem": "..."}`.
- **Paginação** (`PaginaLancamentos`): `{ total, pagina, tamanho, lancamentos[] }`.

> Qualquer alteração de contrato DEVE atualizar esta seção e o RF correspondente (processo §0).

### 11.1 Movimento contábil

#### `POST /movimento-contabil` — RF-030 (gerar)
- **Body**: `{ "data": "YYYY-MM-DD", "produto": "<opcional>", "dominio": "<opcional>" }` (`produto`/`dominio` vazios = todas as combinações).
- **200**: `{"mensagem":"movimento contábil gerado com sucesso"}`; ou, se houver combinações bloqueadas por D-1 (RN-140), `{"mensagem":"O contábil de <p/d, ...> deve ser executado para datas anteriores..."}`; ou a mensagem de erro de negócio (ex.: data não útil, sem posição) — sempre 200.
- **400**: data ausente/malformada.

#### `GET /movimento-contabil` — RF-050 (consultar)
- **Query** (modo filtrado, quando ao menos um de `data_inicio|data_fim|boleto|produto|dominio` vier):
  `data_inicio` (default `2000-01-01`), `data_fim` (default `2999-12-31`), `boleto`, `produto`, `dominio`, `versao_modo` (`vigente`|`todas`|`especifica`; default `vigente`), `versao` (quando `especifica`), `pagina` (default 1), `tamanho` (default 100).
- **Query** (modo compatível data única): `data`, `pagina`, `tamanho`.
- **200**: `PaginaLancamentos` (sem pares de saldo zero — RN-050.1).
- **400**: datas malformadas.

#### `DELETE /movimento-contabil` — RF-053 (excluir)
- **Query**: `data` (obrigatória), `versao` (opcional; 0 = conforme serviço).
- **200**: `{"mensagem":"movimento excluído com sucesso"}`.

#### `POST /estorno` — RN-130 (gerar estorno avulso)
- **Body**: `{ "data": "YYYY-MM-DD" }` (campos `produto`/`dominio` aceitos pelo payload, não usados).
- **200**: `{"mensagem":"estorno gerado com sucesso"}` ou mensagem de erro de negócio.

#### `GET /movimento-contabil/export` — RF-051 (CSV)
- **Query**: `data_inicio`, `data_fim`, `boleto`, `produto`, `dominio`, `versao_modo` (default `vigente`), `versao`. Defaults de data iguais à consulta.
- **200**: arquivo CSV (todos os registros do filtro, sem paginação).

#### `GET /movimento-contabil/export-txt` — RF-052 (TXT)
- **Query**: `data` (obrigatória).
- **200**: arquivo TXT (layout fixo: cabeçalho, detalhes, totalizador) da versão **vigente** sem cancelados; ou `{"sem_dados":"..."}` se vazio.

#### `GET /movimento-contabil/status` — RF-062
- **Query**: `data` (obrigatória).
- **200**: status de execução por combinação (Processados × Pendentes).

#### `GET /movimento-contabil/calendario` — RF-063
- **Query**: `ano` (1900–3000), `mes` (1–12).
- **200**: `{ "ano", "mes", "dias": [DiaCalendario...] }` — status agregado por dia (completo/parcial/nenhum/não útil/futuro), considerando todas as combinações dos padrões.

### 11.2 Regras e condições

#### `GET /regras` — RF-022
- **200**: `[RegraContabil...]` (com condições).

#### `POST /regras` — RF-022
- **Body**: `RegraContabil` (JSON; ver §4.2). `tipo_lancamento` normalizado (RN-024.1).
- **201**: `{"id": <int64>}`. **400**: JSON inválido / erro de serviço.

#### `PUT /regras/{id}` — RF-022
- **Body**: `RegraContabil`. **200**/erro. `{id}` no path.

#### `DELETE /regras/{id}` — RF-022
- **200**/erro.

#### `GET /regras/{id}/condicoes` — RF-023
- **200**: `[CondicaoRegra...]`.

#### `POST /regras/{id}/condicoes` — RF-023
- **Body**: `CondicaoRegra` (§4.3). **201**: `{"id": ...}`.

#### `PUT /condicoes/{id}` — RF-023 · `DELETE /condicoes/{id}` — RF-023
- **Body** (PUT): `CondicaoRegra`. **200**/erro.

#### `GET /regras/export` — RF-024
- **200**: CSV de regras+condições (cabeçalho em `cabecalhoRegrasCSV`; coluna `tipo_lancamento`).

#### `POST /regras/import` — RF-024
- **multipart/form-data**: campo `arquivo` (CSV). Regras de casamento/criação: RN-024.3.
- **200**: resumo da importação (atualizadas/criadas/erros).

### 11.3 Posição de carteira

#### `GET /posicao` — RF-091
- **Query**: `produto` (**obrigatório**), `dominio` (**obrigatório**), `data_inicio` (ou `data` por retrocompat.), `data_fim`, `pagina`, `tamanho`.
- **200**: `PaginaPosicoes`. **400**: faltando produto/domínio/data_inicio.

#### `GET /posicao/campos` — RF-091
- **Query**: `data`.
- **200**: `[nomes_de_campos...]` (vazio se `data` ausente).

#### `POST /posicao/upload` — RF-090
- **multipart/form-data**: `arquivo` (obrigatório), `produto`, `dominio`, `preview` (`"1"` = pré-visualização).
- **200 (preview)**: `{ preview:true, colunas[], coluna_data, total, amostra[≤10] }`.
- **200 (import)**: `{ mensagem, produto, ... , total }`. **400**: sem produto/domínio (fora do preview), arquivo ausente/sem dados.

#### `POST /posicao/upload-lote` — RF-090
- **multipart/form-data**: campo `arquivos` (vários). Produto resolvido pelo **nome do arquivo** via padrões; casando com N padrões, importa para cada um.
- **200**: `{ "resultados": [ResultadoImportacaoArquivo...] }`.

#### `GET /posicao/scan-pasta` — RF-092 · `POST /posicao/scan-pasta` — RF-092
- **GET**: resumo da última varredura. **POST**: dispara varredura imediata e retorna o resumo.

### 11.4 Parametrizações, padrões e configurações

#### `GET /parametrizacoes/opcoes` — RF-043
- **Query**: `categoria` (ex.: `produto`, `dominio`). **200**: `[valores...]`.

#### `POST /parametrizacoes/opcoes` — RF-043
- **Body**: `{ "categoria": "...", "valor": "..." }`. **201**: `{"mensagem":"opção adicionada"}`.

#### `DELETE /parametrizacoes/opcoes` — RF-043
- **Query**: `categoria`, `valor`. **200**: `{"mensagem":"opção removida"}`.

#### `GET /parametrizacoes/padroes` — RF-041
- **200**: `[PadraoArquivo...]` (§4.5).

#### `POST /parametrizacoes/padroes` — RF-041
- **Body**: `PadraoArquivo`. **201**: `{"id": ...}`. (Obrigatórios no cadastro novo: RN-041.1.)

#### `DELETE /parametrizacoes/padroes` — RF-041
- **Query**: `id`. **200**: `{"mensagem":"padrão removido"}`.

#### `GET /configuracoes` — RF-044 · `PUT /configuracoes` — RF-044
- **GET**: `{ chave: valor, ... }`. **PUT Body**: `{ "chave": "...", "valor": "..." }` → `{"mensagem":"configuração salva"}`.

### 11.5 Inconsistências e notificações

#### `GET /inconsistencias` — RF-064
- **Query**: `data`. **200**: `[InconsistenciaProcessamento...]`.

#### `GET /inconsistencias/export` — RF-064
- **Query**: `data`. **200**: CSV.

#### `GET /notificacoes` — RF-071 · `POST /notificacoes` — RF-071
- **GET Query**: `limite` (default 30). **200**: `{ "nao_lidas": N, "itens": [Notificacao...] }`.
- **POST**: marca todas como lidas → `{"mensagem":"notificações marcadas como lidas"}`.

### 11.6 Validação, conciliação e IA

#### `POST /validar-expressao` — RF-025
- **Body**: `{ "expressao": "...", "tipo": "condicao"|"valor" }` (valida **sintaxe** via compilação; não executa contra env).
- **200**: `{ valido:true }` ou `{ valido:false, erro, sugestao }`.

#### `POST /conciliacao` — RF-080
- **Query**: `data`. **200**: `ResultadoConciliacao` (inconsistências `POSICAO_SEM_MOVIMENTO`/`LANCAMENTO_DUPLICADO`).

#### `POST /conciliacao-ia` — RF-081
- **Body**: `{ "pergunta": "...", "data": "YYYY-MM-DD" }`. Requer `GEMINI_API_KEY`.
- **200**: `ConciliacaoIAResponse` `{ diagnostico, sugestao?{descricao, lancamentos[]} }`. **400**: faltando pergunta/data. **500**: sem API key / erro upstream.

#### `POST /conciliacao-ia/ajuste` — RF-081
- **Body**: `{ "data": "...", "lancamentos": [LancamentoContabil...] }`.
- **200**: `{"mensagem":"N lançamento(s) de ajuste aplicado(s)..."}` (nova versão via `BulkInsertAjuste`).

#### `POST /nlquery` — RF-073
- **Body**: `{ "pergunta": "..." }`. Requer `GEMINI_API_KEY` e backend **sqlserver** (RNF-001.2).
- **200**: resultado da consulta gerada. **400**: sem pergunta. **500**: sem API key.

### RF-073 — Consulta em linguagem natural (NL Query)
**Descrição.** Traduz uma pergunta em linguagem natural para consulta sobre os dados e retorna o resultado. Recurso auxiliar, dependente de IA externa (Gemini) e exclusivo do backend `sqlserver`.

---

## 12. Decisões registradas (ADR)

| ID | Decisão | Motivação | Status |
|----|---------|-----------|--------|
| **ADR-001** | Normalização de `tipo_lancamento` centralizada em `model.NormalizaTipoLancamento` (trim+lower; desconhecido⇒reverte), aplicada em todas as escritas e na exportação. | Evitar coerção silenciosa divergente entre backends e caminho API vs CSV. | Implementado |
| **ADR-002** | `valor_D-N` do incremental vem de **reler a posição de D-N e reavaliar `campo_valor`**, não de somar lançamentos postados. | Alinha com a semântica "Posição D0 − Posição D-N" e evita coluna extra. | Implementado |
| **ADR-003** | Boleto que some em D0 **mantém** o saldo contábil (sem baixa automática). | Decisão de negócio do usuário (2026-10-09). | Vigente |
| **ADR-004** | Assumir **no máximo 1 posição por (boleto, dia, versão)**; acumulação incremental soma uma vez por chave. | Confirmado pelo usuário (2026-10-09): não há boleto duplicado por dia/versão. | Vigente |
| **ADR-005** | `reverte` e `nao_reverte` não têm a lógica alterada pela introdução do incremental. | Restrição explícita do usuário. | Implementado |
| **ADR-006** | Memoização de `exigeMovimentoD1` por combinação dentro de `GerarMovimentoEscopo`. | Evitar releitura repetida dos padrões; sem mudança de comportamento. | Implementado |

---

## 13. Template para novos requisitos

```
### RF-NNN — <título>
**Descrição.** <o que o sistema faz>
**Regras.** RN-NNN: <regra de negócio>
**Critérios de aceitação.**
- Dado <contexto>, Quando <ação>, Então <resultado observável>.
**Rastreabilidade.** Código: <arquivos>. Teste: <arquivo_test.go>. ADR: <se houver>.
```

---

## 14. Referências

- `README.md` — guia de uso, passo a passo, telas, execução e proposta AWS.
- `internal/service/*_test.go` — critérios de aceitação executáveis (PBT incluído: `*_pbt_test.go`).
- `migrations/*.sql` — evolução do schema (SQL Server); `internal/repository/sqlite/db.go` — DDL do SQLite.
