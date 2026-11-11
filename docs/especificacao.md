# Especificação Funcional e Técnica — SRCOff

> **SRCOff** — Sistema de Roteirização Contábil Offshore.
> Documento normativo (fonte da verdade) para desenvolvimento orientado a especificação (SDD).

| | |
|---|---|
| **Versão da spec** | 1.1.0 |
| **Data** | 2026-11-11 |
| **Status** | Vigente |
| **Escopo** | POC funcional (backends file / sqlite / sqlserver) |
| **Idioma** | Português (BR) — alinhado ao código |

> **Changelog 1.1.0 (2026-11-11).** Documentadas as funcionalidades implementadas após a 1.0.0 e ausentes da spec: cadastro/governança de regras (RF-071 clonar, RF-125 status rascunho/ativa/excluída, RF-129 lint, RF-133 histórico de versões, RF-134 catálogo), simulação e análise (RF-072 preview, RF-120 dashboard, RF-122 what-if, RF-124 diff de versões, RF-127 balancete, RF-128 cobertura, RF-130 série de saldo, RF-131 conciliação posição×movimento, RF-135 backtesting, RF-070 razonete), operação e processamento (RF-121 jobs, RF-123 expurgo/retenção, RF-126 geração agendada, RF-132 fechamento de competência, RF-137 reprocessamento seletivo) e auditoria/IA (RF-136 trilha de auditoria, RF-138 copiloto conversacional, RF-139 diagnóstico de dupla contabilização e RF-140 linhagem de lançamento). Todo o detalhamento SDD está na **§15**; os endpoints correspondentes, na **§15** junto de cada RF (a §11 permanece como contrato das funcionalidades do núcleo RF-010..RF-092). Novas decisões: ADR-017..ADR-022.

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
tipo_lancamento             string   (reverte | nao_reverte | incremental | incremental_reverte)   ← RN-024
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
descricao_regra_contabil, descricao_condicao_contabil, id_regra_contabil,
origem (automatico | manual; vazio = automatico)   ← RN-033.3
veiculo (veículo da operação, copiado da posição)  ← RN-180
```
- **RN-030.1 (Origem).** Todo lançamento tem `origem`: **`automatico`** (gerado pelo motor) ou **`manual`** (inserido pelo usuário — RF-033). Vazio é tratado como `automatico`. É apenas marcação/exibição; não altera nenhuma lógica de geração. (ADR-012)
- **RN-180 (Veículo da operação — RF-034).** Todo lançamento carrega `veiculo`, **copiado da posição** na geração. A coluna da posição que contém o veículo é definida no padrão de arquivo (`coluna_veiculo`), normalizada em snake_case para casar com os campos da posição. O estorno **herda** o veículo da linha de origem; a baixa do `incremental_reverte` herda o veículo da origem do saldo. É **metadado descritivo**: **não** entra na chave canônica de net/saldo/"sem cancelados" (como a moeda), logo **não altera nenhum resultado contábil**. No lançamento manual o veículo é informado pelo usuário. Reflete em consulta (coluna + filtro exato), extração CSV e lançamento manual. (ADR-013)
- **RN-181 (De/Para de veículo → código — RF-035).** Cadastro em Parametrizações (`veiculo_de_para`: nome → código). **Nome e código obrigatórios**; **não** pode haver mais de um cadastro para o mesmo nome (comparação **case-insensitive**, ignorando espaços nas pontas). É aplicado **exclusivamente na extração CSV** do movimento: para cada lançamento, se existir De/Para para o nome do veículo (case-insensitive), o CSV recebe o **código**; senão, o **nome**. **Não** afeta nenhuma outra funcionalidade — nem a geração, nem a consulta/filtro, nem a extração **TXT**. (ADR-014)
- **RN-182 (Valor na moeda base do veículo — RF-036).** Todo lançamento carrega `valor_moeda_base_veiculo` e `moeda_base_veiculo`. O De/Para do veículo ganha **`moeda_base` (obrigatória)**. A **condição** define a origem desses dados (**obrigatório em toda condição**): **`base_na_posicao=true`** → lê das colunas `campo_valor_moeda_base` e `campo_moeda_base` da posição (data de conversão não usada); **`false`** → **converte** na geração usando a data de `campo_data_conversao`. Na conversão: se a moeda base do veículo (De/Para) **=** a moeda do lançamento, usa o próprio valor/moeda; senão, busca a **cotação** (RN-183) para a data base e aplica a **direção**. Quando não é possível obter a base (campo ausente, veículo sem moeda base, data inválida ou cotação não encontrada) → **inconsistência `MOEDA_BASE`, lançamento não gerado**. O estorno **herda** os dois campos da origem. (ADR-015)
- **RN-183 (Cotação — RF-037).** Novo menu **Cotações** (`cotacao`: Data, Moeda1=veículo, Moeda2=lançamento, Direção, Taxa). **Todos os campos obrigatórios** (direção ∈ {multiplicar, dividir}; taxa > 0). Alimentada por **tela e CSV**. Lookup na geração por (Data, Moeda1, Moeda2) case-insensitive; a conversão é `valor × taxa` (multiplicar) ou `valor ÷ taxa` (dividir). (ADR-016)

### 4.5 PadraoArquivo — RF-040
```
id, padrao (máscara do nome do arquivo), produto, dominio,
delimitador (";"/","/auto), separador_decimal, separador_milhar,
formato_data (rótulo, ex. "DD/MM/AAAA"; obrigatório no cadastro novo),
coluna_data   (coluna que define a data-base; obrigatória no cadastro novo),
coluna_boleto (coluna do boleto, importada como texto; obrigatória no cadastro novo),
coluna_veiculo (coluna do veículo da operação, importada como texto; obrigatória — RF-034/RN-180),
obrigatorio_mov_d1 *bool (nil=true)   ← RN-053
```
- **RN-040.1** `ExigeMovimentoD1()` = `obrigatorio_mov_d1 == nil || *obrigatorio_mov_d1` (default **true**).
- **RN-040.2 (Coluna do veículo — RF-034).** `coluna_veiculo` é **obrigatória** no cadastro do padrão. Na importação a coluna precisa existir no arquivo (senão critica) e é **persistida como texto** (preserva zeros à esquerda/precisão), como o boleto. Na geração do contábil, o valor dessa coluna é copiado para `movimento_contabil.veiculo` (RN-180).

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
tipo (PRE_CONDICAO | CONDICAO | CAMPO_VALOR | CAMPO_MOEDA), expressao, campos_faltantes, detalhe, criado_em
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

### 4.10 SaldoConta (projeção materializada) — RF-065 / RN-170
```
produto, dominio, codigo_identificador_boleto string
id_regra_contabil int64
conta_debito, conta_credito string
saldo float64   (net postado vigente: Σ lançamentos − Σ estornos)
```
- **RN-170.1 (Projeção derivada).** `saldo_conta` é um **cache** do net vigente do `movimento_contabil` por chave `(produto, domínio, boleto, id_regra, conta_deb, conta_cred)`. A **fonte da verdade continua sendo `movimento_contabil`**; `saldo_conta` é reconstruível a partir dele (`ReconstruirSaldos`).
- **RN-170.2 (Manutenção aditiva, fora dos repositórios de movimento).** É atualizada pelo **serviço**, após cada geração, aplicando `delta(chave) = net(novas linhas) − net(versão vigente anterior da mesma data/combo)`. Os métodos `BulkInsert`/`ExcluirPorDataEVersao` do movimento **não são alterados** (nenhum resultado existente muda). (ADR-010)
- **RN-170.3 (Exclusão).** Ao excluir movimento, o serviço **recomputa** os saldos das combinações afetadas a partir do vigente.
- **RN-170.4 (Escopo de leitura).** É **lida** apenas: (a) pela baixa do `incremental_reverte` (RN-121) e (b) pela consulta de saldo (RF-065). **Não** participa do cálculo diário do incremental (que segue usando a posição de D-N — ADR-002).

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
   - **RN-110.5 (Validação do campo_moeda).** O `campo_moeda` da condição também é validado antes de gerar o lançamento: (1) se referenciar campo **ausente** na posição → **inconsistência `CAMPO_MOEDA`**, sem lançamento; (2) se o campo existir mas resultar **em branco** → **inconsistência `CAMPO_MOEDA`**, sem lançamento. Evita lançamentos com moeda vazia.
   - Caso contrário gera **um lançamento** com: `valor = EvaluateValue(campo_valor)`, `moeda = CampoString(campo_moeda)`, `boleto = CampoString(campo_boleto|padrão)`, contas da condição, produto/domínio da posição, `indicador_reversao=false`.

### 5.3 Tipo de lançamento — RN-024 / ADR-001

`tipo_lancamento` (combo **obrigatório**) define a estratégia. Normalização canônica única: `model.NormalizaTipoLancamento` (trim + lower; vazio/desconhecido ⇒ `reverte`). Helpers: `EhReverte()` (inclui vazio), `EhIncremental()`, `EhIncrementalReverte()`, `UsaCalculoIncremental()` (= incremental **ou** incremental_reverte).

| Tipo (valor canônico) | Rótulo na UI | Estorno (RN-130)? | Valor do lançamento em D0 |
|------|------|-------------------|---------------------------|
| **`reverte`** | Posta/Reverte | **Sim** (D-1) | Valor cheio de `campo_valor`. |
| **`nao_reverte`** | Posta/Não Reverte | Não | Valor cheio de `campo_valor`. |
| **`incremental`** | Incremental/Não Reverte | Não | `\| \|valor_D0\| − \|valor_D-N\| \|` (RN-120). |
| **`incremental_reverte`** | Incremental/Reverte | Não (D-1); **sim na baixa** (RN-121) | `\| \|valor_D0\| − \|valor_D-N\| \|` (RN-120), igual ao incremental. |

- **RN-024.1** Normalização aplicada em **toda escrita** (API/CSV/3 backends) e na exportação; o valor persistido é sempre canônico, independentemente de maiúsculas/espaços. (ADR-001)
- **RN-024.4 (Rótulos vs. valores).** Os rótulos "Incremental/Não Reverte" e "Incremental/Reverte" são apenas de **exibição**; os valores canônicos armazenados permanecem `incremental` e `incremental_reverte` (rename sem migração de dados).

### 5.4 Cálculo Incremental — RN-120

Aplica-se aos tipos `incremental` **e** `incremental_reverte` (ambos `UsaCalculoIncremental()`).
Para essas regras, o valor lançado em D0 é a **diferença absoluta** entre o valor de D0 e o valor de D-N:

```
valor_lançado = | |valor_D0| − |valor_D-N| |
```

- **RN-120.1 (Chave da série).** A série incremental é identificada por **(produto, domínio, boleto, id_regra, conta_débito, conta_crédito)**.
- **RN-120.2 (Data-base D-N).** D-N é a **mesma data-base do estorno** (RN-131): D-1 útil se o padrão exige D-1; senão a **maior data anterior com movimento**.
- **RN-120.3 (Origem do valor_D-N).** `valor_D-N` é obtido **relendo a posição de D-N e reavaliando `campo_valor`** (não é a soma de lançamentos postados). Por isso reflete "Posição D0 − Posição D-N". (ADR-002)
- **RN-120.4 (Primeira ocorrência).** Sem D-N (nenhuma execução anterior) → base 0 → lança o **valor cheio**.
- **RN-120.5 (Queda / absoluto).** O valor lançado é **absoluto** (sem sinal), inclusive quando o valor cai (ex.: D-N 1300, D0 900 → lança 400). A persistência mantém o valor como calculado.
- **RN-120.6 (Reprocessamento).** D-N é sempre estritamente anterior a D0, então reprocessar D0 **desconsidera** D0.
- **RN-120.7 (Baixa por desaparecimento).** Se um boleto existe em D-N mas **não** em D0:
  - Tipo `incremental` (Não Reverte): nenhum lançamento é gerado e o saldo contábil **permanece** (sem baixa). (ADR-003)
  - Tipo `incremental_reverte` (Reverte): gera **baixa** zerando o saldo do boleto (RN-121).
- **RN-120.8 (Unicidade de posição).** Assume-se **no máximo uma posição por (boleto, dia, versão)**; portanto a acumulação da base soma exatamente uma vez por chave. (ADR-004)
- **RN-120.9** As regras `reverte` e `nao_reverte` **não** têm sua lógica alterada pelo incremental. (ADR-005)

Implementação: bloco "2b" de `GerarMovimentoEscopo` + `acumularBaseIncremental` + `chaveIncremental`.

### 5.5 Baixa por desaparecimento (Incremental/Reverte) — RN-121 / RF-031 / ADR-007

Exclusivo do tipo `incremental_reverte`. Em cada processamento de D0, para cada boleto que **tinha saldo** em movimento contábil de alguma regra `incremental_reverte` e que **não existe mais na posição de D0**, gera-se um **estorno de baixa** que zera o saldo daquele boleto para a regra.

- **RN-121.1 (Gatilho).** Dispara quando o boleto está **ausente da posição de D0** (verificação por existência na **posição**, não por geração de lançamento — um boleto presente em D0 cujas condições deram `false` **não** é baixado).
- **RN-121.2 (Valor = saldo líquido real).** O valor da baixa é o **saldo líquido efetivamente postado** para a chave (produto, domínio, boleto, id_regra, conta_débito, conta_crédito): `Σ(lançamentos) − Σ(estornos)` vigente até D-N. Isso **zera de fato** a conta, inclusive quando houve quedas (onde o valor absoluto do incremental faz o saldo postado divergir do valor de posição). (ADR-007 — difere deliberadamente do ADR-002: aqui soma-se o postado, não se relê a posição.)
  - **RN-121.2a (Fonte do saldo).** Quando há `saldo_conta` (RN-170) injetado, a baixa **lê o saldo materializado** (`saldo − net do vigente de D0`), com **resultado idêntico** à varredura. Sem ele (ex.: testes legados), usa o caminho de varredura do histórico vigente. (ADR-011)
- **RN-121.3 (Lançamento da baixa).** Estorno com **contas invertidas** em relação à orientação canônica do saldo, `indicador_reversao = true`, `data_lote_contabil = D0`, mesma versão do combo (RN-150).
- **RN-121.4 (Idempotência).** Como o valor vem do saldo líquido vigente, um boleto já baixado tem saldo 0 e **não** é baixado de novo em reprocessamentos ou dias seguintes.
- **RN-121.5 (Escopo e bloqueio).** Respeita o filtro de escopo (produto/domínio) e **não** processa baixas de combinações bloqueadas por D-1 (RN-140). O combo de uma baixa é incluído no versionamento/execução (RN-150/RN-160) mesmo que não tenha lançamentos de D0.
- **RN-121.6 (Chave por regra, todas as contas).** "Todo o saldo ... para a Regra" cobre **todos os pares de contas** (condições) da regra para aquele boleto.

Implementação: bloco "4b" de `GerarMovimentoEscopo` (lê o movimento vigente histórico via `ConsultarPaginadoFiltrado` com `versao_modo=vigente`, agrega o saldo líquido por chave e emite as baixas).

### 5.6 Obrigatoriedade de movimento de D-1 — RN-140

Quando o padrão da combinação **exige D-1** (RN-040.1) e há movimento contábil em alguma data anterior:
- **RN-140.1** Se **não** houver movimento no **D-1 útil**, a combinação é **bloqueada** (não processa lançamentos nem inconsistências naquela data).
- **RN-140.2** Se não há movimento anterior nenhum (primeira vez) → **não** bloqueia.
- **RN-140.3** A base é o **movimento real** (`DatasComMovimento`, `qtd_lancamentos > 0`), não o mero log de execução. (RN-060.1)

### 5.7 Estorno — RN-130 / RN-131

- **RN-130.1** Apenas regras `EhReverte()` geram estorno. `nao_reverte` e `incremental` **nunca** estornam.
- **RN-130.2** O estorno copia o lançamento de origem **invertendo** débito↔crédito, com `indicador_reversao = true` e valor igual.
- **RN-131 (Data de origem do estorno por combinação).** `dataEstornoPorCombo`:
  - Exige D-1 → **D-1 útil**.
  - Não exige D-1 → **maior data anterior com movimento** (`DatasComMovimento`).
  - Sem data de origem (nenhuma execução anterior) → **não estorna**.

### 5.8 Versionamento por combinação — RN-150

- **RN-150.1** A versão (`codigo_versao_conteudo`) é calculada **por (produto, domínio)**: `max(versão vigente da combinação) + 1`.
- **RN-150.2** Cada combinação versiona de forma **independente**: a 1ª execução de um novo combo na data começa em **v1**, mesmo que outro combo já esteja em versão maior.
- **RN-150.3** Lançamentos e estornos da mesma execução recebem a mesma versão do combo.

### 5.9 Persistência e registro — RN-160

- **RN-160.1** Lançamentos + estornos são persistidos em um único `BulkInsert`.
- **RN-160.2** Inconsistências são persistidas com `SubstituirPorEscopo` (substitui apenas as combinações do escopo na data).
- **RN-160.3** Para cada combinação é registrada uma **execução** (RF-060) com `qtd_lancamentos`/`qtd_estornos` **visíveis** (= consulta sem cancelados) e `qtd_movimento` **bruto**.
- **RN-160.4** Retorna a lista de combinações **bloqueadas** por D-1 (não processadas); as demais são processadas normalmente.

#### Critérios de aceitação (núcleo)
- **CA-incremental** (`internal/service/incremental_test.go`): posições 1000/1300/900 em dias consecutivos, padrão sem D-1 obrigatório → lança **1000 → 300 → 400**, todos **sem estorno**.
- **CA-execucao-consulta** (`execucao_alinhada_test.go`): a contagem gravada na execução bate com a consulta (pares de saldo zero ocultos).
- **CA-D1** (`obrigatoriedade_d1_test.go`): bloqueio quando falta D-1 útil e já há contábil anterior.

---

## 5b. Lançamento manual — RF-033 / RN-033

Permite ao usuário inserir **um ou N** lançamentos contábeis manualmente, **em tela** ou **via CSV**.

- **RN-033.1 (Campos).** Por lançamento: `data` (dia útil), `produto`, `dominio`, `id_regra`, `id_condicao`, `boleto`, `valor` (≠ 0), `moeda`, `indicador_reversao` (opcional; default lançamento). As **contas débito/crédito** e as descrições de regra/condição **vêm da condição referenciada** (não são digitadas) — a condição **vai** no lançamento (`descricao_condicao_contabil`).
- **RN-033.2 (Validação tudo-ou-nada).** Se qualquer linha for inválida (data não útil, regra/condição inexistente, valor zero, campos faltando), **nada** é inserido.
- **RN-033.3 (Origem).** Lançamentos manuais recebem `origem = manual` (RN-030.1); os gerados pelo motor, `automatico`.
- **RN-033.4 (Versionamento — anexa à vigente).** Cada lançamento manual é **anexado à versão vigente** do seu (produto, domínio, data) — ou **versão 1** se não houver movimento. Assim aparece junto do movimento existente na consulta. Se depois o contábil for **gerado/reprocessado** para essa data/combo, cria-se uma versão nova que **supersede** (comportamento normal de versionamento). (ADR-013)
- **RN-033.5 (Saldo).** A inserção manual atualiza a projeção de saldo (RN-170) somando o net das linhas.
- **RN-033.6 (Sem execução).** Não registra execução (RF-060) nem inconsistências — é inserção direta, não geração.

Implementação: `InserirLancamentosManuais`; handler `POST /lancamentos-manuais` (JSON) e `/import` (CSV).

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
  - **RN-024.2** Coluna `tipo_lancamento` aceita `reverte | nao_reverte | incremental | incremental_reverte` (vazio = `reverte`); também aceita rótulos amigáveis ("não reverte", "incremental/reverte", "incremental/não reverte"). Normalização via `model.NormalizaTipoLancamento`.
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
  - **RN-043.1 (Sem seed padrão).** Um banco/instalação novo nasce **sem** opções de combos; o usuário cadastra Produto/Domínio manualmente. Vale para os 3 backends (file, sqlite, SQL Server). (ADR-009)
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

Base: `/api/v1`. Rotas registradas em `cmd/api/main.go`. Esta seção cobre o **núcleo (RF-010..RF-092)**; os endpoints das funcionalidades RF-070..RF-140 (análise, governança, fechamento, auditoria e IA) estão documentados junto de cada requisito na **§15**. Convenções gerais:

- **Datas**: sempre `YYYY-MM-DD`.
- **Erro**: corpo `{"erro": "<mensagem>"}`. Observação: alguns fluxos de negócio (gerar movimento/estorno) respondem **HTTP 200** com `{"mensagem": "..."}` mesmo quando a operação não produz efeito (ex.: data não útil, bloqueio D-1). O 4xx é reservado a entrada malformada.
- **Resposta de sucesso simples**: `{"mensagem": "..."}`.
- **Paginação** (`PaginaLancamentos`): `{ total, pagina, tamanho, lancamentos[] }`.

> Qualquer alteração de contrato DEVE atualizar esta seção e o RF correspondente (processo §0).

### 11.1 Movimento contábil

#### `POST /movimento-contabil` — RF-030 (gerar) · RF-031 (baixa por desaparecimento)
- **Body**: `{ "data": "YYYY-MM-DD", "produto": "<opcional>", "dominio": "<opcional>" }` (`produto`/`dominio` vazios = todas as combinações).
- **200**: `{"mensagem":"movimento contábil gerado com sucesso"}`; ou, se houver combinações bloqueadas por D-1 (RN-140), `{"mensagem":"O contábil de <p/d, ...> deve ser executado para datas anteriores..."}`; ou a mensagem de erro de negócio (ex.: data não útil, sem posição) — sempre 200.
- **400**: data ausente/malformada.
- **RF-031**: no mesmo processamento, regras `incremental_reverte` geram as baixas de boletos que sumiram da posição (RN-121).

#### `GET /movimento-contabil` — RF-050 (consultar)
- **Query** (modo filtrado, quando ao menos um de `data_inicio|data_fim|boleto|produto|dominio` vier):
  `data_inicio` (default `2000-01-01`), `data_fim` (default `2999-12-31`), `boleto`, `produto`, `dominio`, `versao_modo` (`vigente`|`todas`|`especifica`; default `vigente`), `versao` (quando `especifica`), `pagina` (default 1), `tamanho` (default 100).
- **Query** (modo compatível data única): `data`, `pagina`, `tamanho`.
- **200**: `PaginaLancamentos` (sem pares de saldo zero — RN-050.1).
- **400**: datas malformadas.

#### `POST /lancamentos-manuais` — RF-033 (inserir em tela)
- **Body**: `{ "lancamentos": [ { data, produto, dominio, id_regra, id_condicao, boleto, valor, moeda, indicador_reversao } ] }`.
- **201**: `{ inseridos: N, mensagem }`. **400**: validação (mensagem indica a linha).

#### `POST /lancamentos-manuais/import` — RF-033 (inserir via CSV)
- **multipart/form-data**: campo `arquivo` (CSV `;`), cabeçalho: `data;produto;dominio;id_regra;id_condicao;boleto;valor;moeda;indicador_reversao`.
- **201**: `{ inseridos: N, mensagem }`. **400**: CSV vazio/validação.

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

### 11.5b Saldo de contas

#### `GET /saldos` — RF-065
- **Query** (todos opcionais, filtram): `produto`, `dominio`, `boleto`, `id_regra`, `conta_debito`, `conta_credito` (par da **condição**), `conta` (nº que casa em **débito OU crédito**, sem informar D/C), `com_zero=1` (inclui saldo 0; default oculta), `pagina` (default 1), `tamanho` (default 50).
- **200**: `SaldoPagina` `{ total, pagina, tamanho, saldos[] }`. A tela consulta **apenas sob comando** do usuário (não carrega automático).
- **RN-065.1 (Filtro de condição).** A condição é identificada pelo seu **par de contas** (débito/crédito) dentro da regra (não há `id_condicao` persistido). A UI oferece Regra → Condição (combos dependentes) e Conta por número.
- **200**: `[SaldoConta...]` (§4.10) ordenado por produto/domínio/boleto. Projeção derivada do movimento (RN-170).

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
| **ADR-007** | Novo tipo `incremental_reverte`: baixa por desaparecimento usa o **saldo líquido real postado** (`Σ lançamentos − Σ estornos` do movimento vigente), **não** a releitura da posição D-N. | Decisão do usuário (2026-10-11): "estornar todo o saldo" deve **zerar a conta**, o que só a soma do postado garante quando houve quedas (valor absoluto). Difere do ADR-002, que vale só para o cálculo diário. | Vigente |
| **ADR-008** | Rename de rótulos: `incremental` → "Incremental/Não Reverte"; novo `incremental_reverte` → "Incremental/Reverte". Valores canônicos inalterados (sem migração). | Pedido do usuário; evita migração de dados e mantém retrocompatibilidade. | Vigente |
| **ADR-010** | Tabela `saldo_conta` como projeção materializada do net vigente, mantida pelo **serviço** (aditiva), sem alterar `BulkInsert`/`ExcluirPorDataEVersao`. Fonte da verdade continua o `movimento_contabil`; reconstruível. | Performance da baixa `incremental_reverte` em escala (500k/dia) sem varrer o histórico; e consulta direta de saldo. | Implementado |
| **ADR-012** | Campo `origem` (automatico\|manual) no `movimento_contabil` para diferenciar lançamentos gerados dos inseridos manualmente. Default `automatico`; não altera lógica de geração. | Pedido do usuário (2026-10-16): diferenciar gerado × manual. | Implementado |
| **ADR-013** | Lançamento manual **anexa à versão vigente** do combo/data (ou v1). Reprocesso posterior supersede. | Mostrar o manual junto do movimento existente sem esconder o gerado. | Implementado |
| **ADR-011** | A baixa do `incremental_reverte` passa a **ler** o `saldo_conta` quando presente (resultado idêntico à varredura); mantém o caminho de varredura como fallback. Nenhum outro tipo de regra é afetado. | Atende "nada do que funciona hoje muda, exceto o estorno do incremental com reversão". | Implementado |
| **ADR-009** | Remoção do seed de opções de combos (Produto NDF/SWAP/FXO, Domínio Posição/Liquidação) na criação do banco/instalação, nos 3 backends. Para bases SQL Server já existentes, a `migration 015` remove explicitamente esses 5 pares (robusta a code page; não toca opções cadastradas pelo usuário). | Pedido do usuário (2026-10-11): instalação nova deve nascer sem combos padrão. | Implementado |
| **ADR-017** | Integração de IA (copiloto RF-138, conciliação-IA RF-081, NL Query RF-073) feita por **HTTP cru** (`net/http`), **não** por SDK de fornecedor; provedores plugáveis (Claude/Gemini/Ollama) atrás de uma interface única; **chaves sempre em variável de ambiente**, nunca no código ou no banco. | Preservar o build _vendored_ offline (`go mod vendor`) e não acoplar o produto a um fornecedor; segurança (não versionar segredos). Pedido do usuário (2026-11): chave em env, e IA utilizável de graça (Gemini free / Ollama local). | Implementado |
| **ADR-018** | Modelo Gemini padrão = `gemini-3.5-flash` (sobreponível por `GEMINI_MODEL`). `gemini-2.0-flash` foi descontinuado; `gemini-3.8-flash`/`gemini-flash-latest` **não respondem** pelo endpoint `v1beta/generateContent` (direcionados à nova Interactions API). | Erro em produção do usuário (2026-11): modelo antigo removido; os "mais novos" travam no generateContent. `3.5-flash` responde normalmente. | Implementado |
| **ADR-019** | Histórico de versões do cadastro (RF-133) é **append-only** e no **nível da regra** (uma alteração só de condição aparece na linha do tempo da regra). Restaurar uma versão antiga **cria uma nova versão** com o conteúdo restaurado (não apaga o histórico). sqlite-first. | Rastreabilidade/auditoria: nunca perder histórico; unificar a visão no objeto que o usuário enxerga (a regra). | Implementado |
| **ADR-020** | Trilha de auditoria (RF-136) **sempre** emite a linha de log JSON estruturada (`[auditoria] {...}`); a **persistência/consulta** só liga quando há repositório (sqlite-first). O serviço é nil-safe (nunca quebra a operação de negócio por falha de auditoria). | Garantir trilha mesmo em backends sem persistência; não acoplar o sucesso da operação à gravação do log. | Implementado |
| **ADR-021** | Controle de fechamento de competência (RF-132) por **MODO** (off / todos / selecionados), **não por data**: sob controle, uma competência (mês/ano) posterior a outra já processada só gera após **liberação manual** do mês/ano. sqlite-first (tipo concreto; nil nos demais backends = sem trava). | Pedido do usuário: travar a virada de competência sem exigir cadastro de datas; degradar com segurança nos backends sem a feature. | Implementado |
| **ADR-022** | Features **[LAB]** experimentais (RF-139 diagnóstico de dupla contabilização, RF-140 linhagem) atrás da flag de ambiente **`FEATURES_LAB`** (ligada por padrão; `0/false/off/nao` desliga sem recompilar → menu some, endpoints 501). Isoladas em arquivos próprios (`*lab*`), reusando helpers do motor sem alterá-lo; removíveis por `grep "[LAB]"`. | Pedido do usuário (2026-11): implementar para testar e poder **desabilitar/remover** depois do teste com baixo custo. | Vigente |

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

---

## 15. Funcionalidades adicionais — detalhamento SDD (RF-070..RF-140)

Requisitos implementados após a spec 1.0.0. Cada entrada traz **Descrição**, **Regras (RN)**, **Endpoint(s)**, **Critérios de aceitação** e **Rastreabilidade** (código/teste). Convenção de disponibilidade por backend (**sqlite-first**): várias features só persistem/consultam no backend `sqlite` (tipo concreto no `main`; `nil` nos demais → a feature degrada para *off*, retornando **501** ou ocultando-se na UI). A UI é servida embutida (`internal/webui/app.html`, `go:embed`).

### 15.1 Cadastro e governança de regras

#### RF-071 — Clonar regra / condição
**Descrição.** Duplica uma regra inteira (com suas condições) ou uma condição dentro da mesma regra; a cópia recebe a descrição sufixada com " (cópia)".
**Regras.** RN-071.1: a regra clonada nasce como **rascunho** (RF-125), não entra na geração até ser ativada. RN-071.2: a condição clonada é anexada à mesma regra.
**Endpoint.** `POST /api/v1/regras/{id}/clonar` · `POST /api/v1/condicoes/{id}/clonar`.
**Rastreabilidade.** Código: `internal/service/regra_contabil_service.go` (`ClonarRegra`, `ClonarCondicao`), `internal/handler/regra_contabil_handler.go`.

#### RF-125 — Status rascunho / ativa / excluída
**Descrição.** O ciclo de vida de regras e condições passa a ter estado explícito (`status`), substituindo o booleano `ativo` como fonte da verdade (derivado: `ativo ⟺ status==ativa`).
**Regras.**
- RN-125.1: estados — **`rascunho`** (existe e é editável, mas NÃO vale na geração), **`ativa`** (vale na geração), **`excluida`** (removido logicamente, oculto por padrão).
- RN-125.2: a **geração** usa apenas o que está `ativa` (`ListarRegrasAtivas`); a **UI** lista `ativa + rascunho` (`ListarRegras`/`ListarCondicoesVisiveis`), nunca `excluida`.
- RN-125.3 (compat.): `NormalizaStatus(status, ativo)` deriva o estado em bancos antigos (sem a coluna) a partir do booleano `ativo` (ativo→ativa, senão excluida). sqlite-first; nos demais backends o status é derivado.
**Critérios de aceitação.** Dado uma condição em `rascunho`, Quando o contábil é gerado, Então ela **não** produz lançamento; Quando ativada, Então passa a produzir. (`internal/repository/sqlite/sqlite_test.go` → `TestStatusRegraCondicao`.)
**Rastreabilidade.** Código: `internal/model/regra_contabil.go` (`StatusRascunho|Ativa|Excluida`, `NormalizaStatus`, `AtivoDoStatus`), `internal/repository/sqlite/*`, `internal/repository/interfaces.go`.

#### RF-129 — Lint (verificação estática) de regras
**Descrição.** Analisa **estaticamente** as regras/condições ativas e devolve achados (erro/aviso) sem executar contra posições.
**Regras.** RN-129.1: detecta — regra ativa **sem condição**; expressão (pré-condição/condição/`campo_valor`) que **não compila**; **conta débito == crédito**; **condições duplicadas** no mesmo escopo (mesmo produto/domínio + expressão + contas). Severidade `erro`/`aviso`.
**Endpoint.** `GET /api/v1/regras/lint` → `LintResultado { total, achados[] }`.
**Rastreabilidade.** Código: `internal/service/regra_contabil_service.go` (`Lint`), `internal/handler/analises_handler.go`, `internal/model/analises.go`. Teste: `internal/service/analises_test.go`.
> **Complemento data-driven:** o lint estático **não** detecta sobreposição entre expressões **diferentes** que casam o mesmo boleto — isso é o RF-139 (§15.4).

#### RF-133 — Histórico de versões do cadastro de regras
**Descrição.** Ao alterar uma regra **ou** uma de suas condições, registra-se um snapshot versionado da regra (com condições). Permite consultar a linha do tempo, **comparar** duas versões (diff) e **restaurar** uma versão antiga como vigente. A unidade de versão é a **regra** (uma mudança só de condição aparece na linha do tempo da regra).
**Regras.**
- RN-133.1 (Captura): cada mutação relevante (criar/editar/excluir regra ou condição) captura, _best-effort_, um snapshot com número incremental, resumo da alteração e origem; "Sem alterações" não gera versão.
- RN-133.2 (Diff): compara campos da regra e condições (adicionada/removida/alterada) entre duas versões.
- RN-133.3 (Restaurar — append-only, ADR-019): restaura campos e condições da versão escolhida criando **uma nova versão vigente**; condições excluídas são ressuscitadas e condições mais novas, excluídas, conforme o alvo; o histórico **nunca** é apagado.
- RN-133.4: sqlite-first; sem o repositório de versões o cadastro funciona normalmente e a feature fica *off*.
**Endpoints.** `GET /api/v1/regra-historico` (resumo quando sem `id`; linha do tempo quando `?id=<regra>`) · `GET /api/v1/regra-historico/diff?id=&a=&b=` · `POST /api/v1/regra-historico/restaurar`.
**Critérios de aceitação.** Dado uma regra editada N vezes, Quando consulto o histórico, Então vejo N versões em ordem decrescente com a vigente marcada; Quando restauro a versão k, Então o estado vigente volta ao conteúdo de k numa versão nova. (`internal/service/regra_versao_service_test.go` → `TestHistoricoRegraFluxo`; `internal/model/regra_versao_test.go`.)
**Rastreabilidade.** Código: `internal/model/regra_versao.go`, `internal/repository/sqlite/regra_versao.go`, `internal/service/regra_contabil_service.go` (`Historico`, `ResumoHistorico`, `DiffVersoes`, `RestaurarVersao`, `capturarVersao*`), `internal/handler/regra_historico_handler.go`. UI: modal "Histórico" + selo por regra em `app.html`.

#### RF-134 — Catálogo de regras (documento imprimível)
**Descrição.** Gera um documento **HTML imprimível** com as regras **ativas** e suas condições — relatório legível para contábil/auditoria.
**Regras.** RN-134.1: lista apenas regras ativas; rótulos amigáveis para tipo de lançamento e moeda base; "todos" quando produto/domínio vazios.
**Endpoint.** `GET /api/v1/catalogo-regras` → documento HTML (abre em nova aba).
**Rastreabilidade.** Código: `internal/handler/catalogo_regras_handler.go` (`html/template`), `internal/service/regra_contabil_service.go` (`RegrasAtivas`). Teste: `internal/handler/catalogo_regras_test.go`.

### 15.2 Simulação e análise

#### RF-072 — Preview de expressão sobre a posição
**Descrição.** Pré-visualiza o resultado de uma expressão (condição ou valor) avaliada sobre a posição real de uma data/escopo, antes de salvar a regra.
**Endpoint.** `POST /api/v1/preview-expressao`.
**Rastreabilidade.** Código: `internal/service/movimento_contabil_service.go` (`PreviewExpressao`), `internal/model/preview_expressao.go`, `internal/handler/validar_expressao_handler.go`/`simulacao_whatif_handler.go`.

#### RF-120 — Dashboard
**Descrição.** Visão consolidada (execuções, inconsistências, saldo/câmbio) no menu Dashboard.
**Endpoint.** `GET /api/v1/dashboard`.
**Rastreabilidade.** Código: `internal/model/dashboard.go`, `internal/service/dashboard_service.go`, `internal/handler/dashboard_handler.go`. O dashboard só lê a projeção de saldo (RN-170) quando ela está habilitada.

#### RF-122 — Simulação What-if de regra candidata
**Descrição.** Avalia uma regra **hipotética** (não persistida) sobre a posição vigente de uma data e projeta os lançamentos que ela geraria, sem gravar nada.
**Regras.** RN-122.1: reusa a mesma avaliação da geração (`EvaluateCondition`/`EvaluateValue`, `boletoDaPosicao`); é uma projeção dos lançamentos **normais** (sem estorno nem conversão de moeda base).
**Endpoint.** `POST /api/v1/simulacao-whatif`.
**Critérios de aceitação.** Dado uma regra candidata e uma posição, Quando simulo, Então recebo os lançamentos projetados sem persistir. (`internal/service/simulacao_whatif_test.go` → `TestSimularRegra_CandidataSobrePosicao`.)
**Rastreabilidade.** Código: `internal/service/movimento_contabil_service.go` (`SimularRegra`), `internal/model/simulacao_whatif.go`, `internal/handler/simulacao_whatif_handler.go`.

#### RF-135 — Backtesting de regra
**Descrição.** Roda uma regra candidata (What-if) sobre **N dias históricos** com posição, caminhando para trás a partir da última data com posição, e **agrega o impacto** (por dia e por conta).
**Regras.** RN-135.1: percorre dias úteis para trás coletando N datas que tenham posição (com guarda de iteração máxima); RN-135.2: cada dia reusa o laço de avaliação do What-if e agrega totais.
**Endpoint.** `POST /api/v1/backtest-regra` → `BacktestResultado { dias[], contas[] }`.
**Critérios de aceitação.** Dado uma regra candidata e N dias, Quando rodo o backtest, Então recebo o impacto agregado por dia/conta das datas com posição. (`internal/service/backtest_service_test.go`.)
**Rastreabilidade.** Código: `internal/model/backtest.go`, `internal/service/backtest_service.go` (`BacktestRegra`), `internal/handler/backtest_handler.go`. Dependência de repositório: `posicaoRepo.UltimaDataPosicao`.

#### RF-124 — Diff de versões do movimento contábil
**Descrição.** Compara **duas versões** de um mesmo (data, produto, domínio) de movimento e mostra lançamentos adicionados, removidos e alterados (inclusive mudança de valor/moeda). Sem versões informadas, usa "vigente × anterior".
**Endpoint.** `GET /api/v1/movimento-contabil/diff`.
**Critérios de aceitação.** `internal/service/diff_versoes_test.go` → `TestDiffVersoes_VigenteXAnterior`.
**Rastreabilidade.** Código: `internal/service/movimento_contabil_service.go` (`DiffVersoes`, `somaPorChaveVersao`), `internal/model/diff_versoes.go`, `internal/handler` (diffVersoesHandler). Repo: `VersoesMovimento`.

#### RF-127 — Balancete de verificação
**Descrição.** Σ débitos × Σ créditos por conta no período (confere a partida dobrada).
**Endpoint.** `GET /api/v1/balancete` (`data_inicio`, `data_fim`, `produto?`, `dominio?`, `versao_modo?`).
**Rastreabilidade.** Código: `internal/service/movimento_contabil_service.go` (`Balancete`), `internal/model/analises.go`, `internal/handler/analises_handler.go`.

#### RF-128 — Cobertura de posições
**Descrição.** Para uma data/escopo, classifica cada posição em **coberta** (alguma regra gera), **órfã** (nenhuma regra gera) ou **inconsistente** (campo ausente/erro), com listas (truncadas por limite).
**Endpoint.** `GET /api/v1/cobertura` (`data`, `produto?`, `dominio?`).
**Rastreabilidade.** Código: `internal/service/movimento_contabil_service.go` (`CoberturaPosicoes`), `internal/model/analises.go`, `internal/handler/analises_handler.go`.

#### RF-130 — Série temporal de saldo por conta
**Descrição.** Evolução do saldo de uma conta ao longo do período (para gráfico/tendência).
**Endpoint.** `GET /api/v1/serie-saldo` (`conta` obrigatória, `data_inicio`, `data_fim`, `produto?`, `dominio?`).
**Rastreabilidade.** Código: `internal/service/movimento_contabil_service.go` (`SerieSaldoConta`), `internal/model/analises.go`, `internal/handler/analises_handler.go`.

#### RF-131 — Conciliação posição × movimento (saldo×posição + órfãos)
**Descrição.** Em uma execução, concilia a posição contra o movimento da data/escopo (presença na posição, órfãos, divergências de saldo).
**Endpoint.** `GET /api/v1/conciliacao` (`data`, `produto?`, `dominio?`).
**Rastreabilidade.** Código: `internal/service/movimento_contabil_service.go` (`Conciliar`, `valorEsperadoPorBoleto`), `internal/model/analises.go`, `internal/handler/analises_handler.go`.

#### RF-070 — Razonete (livro razão em T)
**Descrição.** Monta o razonete por conta a partir dos lançamentos vigentes do período (cada lançamento contribui com débito na `conta_debito` e crédito na `conta_credito`; estornos no lado oposto), com saldo acumulado e natureza (D/C). Filtros: boleto, conta, produto, domínio.
**Endpoint.** `GET /api/v1/razonete`.
**Rastreabilidade.** Código: `internal/service/movimento_contabil_service.go` (`Razonete`), `internal/model/razonete.go`, `internal/handler/razonete_handler.go`.
> **Nota de rastreabilidade:** o código marca o razonete como `RF-070`, número que a spec 1.0.0 usara para o modelo `Notificacao` (§4.8) e `RF-071` para endpoints de notificação / clonar. Colisão herdada da 1.0.0; mantida como **dívida de documentação** (IDs imutáveis — §0.4) a ser reconciliada numa revisão futura, sem renumerar o que já está no código.

### 15.3 Operação e processamento

#### RF-132 — Controle de fechamento de competência
**Descrição.** Trava a virada de competência: sob controle, uma competência (mês/ano) **posterior** a outra já processada só gera após **liberação manual** do mês/ano para o escopo.
**Regras.**
- RN-132.1 (Modo, ADR-021): `off` (sem controle), `todos` (todas as combinações, inclusive novas) ou `selecionados` (apenas os escopos cadastrados; campo vazio = curinga). `FechamentoConfig.Controla(produto,dominio)`.
- RN-132.2 (Liberação): o OK é por (ano, mês, produto, domínio) — produto/domínio vazios = "todos". `Liberado(ano,mes,produto,dominio)`.
- RN-132.3 (Trava na geração): para uma combinação sob controle e **não liberada**, a geração de uma competência posterior a outra já processada é **bloqueada** (HTTP 200 com mensagem orientando liberar no menu Fechamento).
- RN-132.4: sqlite-first (tipo concreto; `nil` nos demais backends = sem trava).
**Endpoints.** `GET /api/v1/fechamento` (estado) · `POST /fechamento/modo` · `POST /fechamento/escopo` + `DELETE /fechamento/escopo/{id}` · `POST /fechamento/liberar` `{ano,mes,produto,dominio}` + `DELETE /fechamento/liberar/{id}`.
**Rastreabilidade.** Código: `internal/model/controle_fechamento.go`, `internal/repository/sqlite/controle_fechamento.go`, `internal/service/fechamento_service.go`, `internal/service/movimento_contabil_service.go` (trava na geração), `internal/handler` (FechamentoHandler).

#### RF-137 — Reprocessamento seletivo
**Descrição.** Reprocessa o contábil de um escopo restrito (produto e/ou domínio e, opcionalmente, um **boleto**) para a data, **sem refazer o dia inteiro**.
**Regras.** RN-137.1: reusa `GerarMovimentoEscopo` por combinação. RN-137.2 (por boleto): como a versão é **por combo** (produto, domínio) e a identidade do boleto depende da regra, não há isolamento real "por boleto"; o serviço **resolve e reprocessa o(s) combo(s) que contêm o boleto** (match por `codigo_identificador_boleto`). RN-137.3: emite evento de auditoria `movimento.reprocessado` (RF-136).
**Endpoint.** `POST /api/v1/reprocessar` `{data, produto?, dominio?, boleto?}`.
**Critérios de aceitação.** Dado um boleto informado, Quando reprocesso, Então apenas o(s) combo(s) que o contêm são regerados. (`internal/service/reprocessamento_service_test.go` → `TestReprocessarSeletivo`.)
**Rastreabilidade.** Código: `internal/model/reprocessamento.go`, `internal/service/reprocessamento_service.go`, `internal/handler/reprocessamento_handler.go`.

#### RF-121 — Fila de jobs assíncronos
**Descrição.** Fila de jobs em segundo plano (ex.: geração de movimento, expurgo), com worker, estados e log por job.
**Endpoints.** `GET /api/v1/jobs` (listar/enfileirar) · `/api/v1/jobs/{id}` (item + log).
**Regras.** RN-121.1: disponível quando há repositório de jobs (sqlite); o worker roda em segundo plano.
**Rastreabilidade.** Código: `internal/model/job.go`, `internal/repository/sqlite/job.go`, `internal/service/job_service.go`, `internal/handler` (JobHandler); registro em `cmd/api/main.go`.

#### RF-123 — Expurgo / retenção de movimentos
**Descrição.** Job recorrente que remove movimentos (e inconsistências/execuções correspondentes) além da retenção configurada (N dias).
**Regras.** RN-123.1: retenção N≤0 = desativado; RN-123.2: disponível quando os repositórios sabem expurgar por data (interface `ExpurgavelPorData`, sqlite).
**Configuração.** Via `/api/v1/configuracoes` (dias de retenção, intervalo). Sem endpoint HTTP próprio (roda como job RF-121).
**Critérios de aceitação.** `internal/repository/sqlite/sqlite_test.go` → `TestExpurgarAteData`.
**Rastreabilidade.** Código: `internal/service` (`ExpurgoService`), `internal/repository/sqlite/*` (`ExpurgarAteData`), `cmd/api/main.go` (`agendarExpurgo`).

#### RF-126 — Geração agendada do contábil
**Descrição.** Enfileira a geração do contábil 1×/dia no horário configurado (HH:MM), para o escopo (produto+domínio) configurado, na **última data com posição** importada.
**Regras.** RN-126.1: desativado se horário vazio/inválido ou escopo não configurado; RN-126.2: não regera data já executada para o escopo; RN-126.3: não enfileira duplicado (mesma data/produto/domínio pendente).
**Configuração.** `/api/v1/configuracoes` (horário, produto, domínio). Roda como job (RF-121).
**Critérios de aceitação.** `UltimaDataPosicao` validada em `internal/repository/sqlite/sqlite_test.go` → `TestUltimaDataPosicao`.
**Rastreabilidade.** Código: `cmd/api/main.go` (`agendarGeracao`, `geracaoPendente`), `internal/repository/*/posicao*.go` (`UltimaDataPosicao`).

### 15.4 Auditoria e IA

#### RF-136 — Trilha de auditoria global
**Descrição.** Log unificado e **estruturado (JSON)** de todas as ações do sistema (cadastro, geração, fechamento, liberação, conciliação, reprocessamento), com consulta filtrável e exportação.
**Regras.**
- RN-136.1 (Sempre-log, ADR-020): `Registrar` **sempre** emite `log.Printf("[auditoria] %s", json)`; é **nil-safe** (nunca quebra a operação de negócio).
- RN-136.2 (Persistência sqlite-first): grava/consulta só quando há repositório (sqlite); `Disponivel()` reflete isso.
- RN-136.3 (Evento): `categoria`, `ação`, `entidade`, `origem`, `detalhe`, `dados` (map serializado em JSON).
- RN-136.4 (Hooks): emitido por cadastro de regras (regra/condição criada/editada/excluída, regra.restaurada), fechamento (modo/escopo, competência liberada), geração/conciliação/reprocessamento (movimento.gerado, conciliacao.executada, movimento.reprocessado).
**Endpoints.** `GET /api/v1/auditoria` (filtrável) · `GET /api/v1/auditoria/export` (NDJSON default; `?formato=csv`). Backend sem repositório → **501**.
**Critérios de aceitação.** `internal/service/auditoria_service_test.go`; `internal/repository/sqlite/auditoria_test.go`.
**Rastreabilidade.** Código: `internal/model/auditoria.go`, `internal/repository/sqlite/auditoria.go`, `internal/service/auditoria_service.go`, `internal/handler/auditoria_handler.go`.

#### RF-138 — Copiloto conversacional (chat IA)
**Descrição.** Chat que responde sobre o sistema e, **com confirmação do usuário**, executa ações (criar regra como rascunho, reprocessar, liberar competência). Ferramentas de **leitura** rodam de imediato; ações de **escrita** exigem confirmação na UI.
**Regras.**
- RN-138.1 (Provedores plugáveis, ADR-017): Claude / Gemini / Ollama atrás de uma interface única; seleção por `AI_PROVIDER` ou auto (primeiro disponível). Chamada por **HTTP cru**; **chaves em variável de ambiente** (`ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `OLLAMA_URL`), nunca no código.
- RN-138.2 (Modelo Gemini, ADR-018): padrão `gemini-3.5-flash` (sobreponível por `GEMINI_MODEL`).
- RN-138.3 (Habilitação): `GET /copilot/status` informa `{disponivel, modelo, provedor}`; sem chave, `disponivel=false` e a UI mostra aviso com as opções gratuitas (Gemini free / Ollama local).
- RN-138.4 (Loop de agente): planejamento via JSON (`acao_tipo` responder/ferramenta/confirmar), máx. passos; resultados de ferramenta são dobrados no prompt (alternância robusta). Ações de escrita só executam via `POST /copilot/acao` após confirmação.
- RN-138.5 (Segurança): criar_regra cria como **rascunho** (RF-125); nenhuma ação de escrita é executada sem passo de confirmação explícito.
**Endpoints.** `POST /api/v1/copilot` (chat) · `GET /api/v1/copilot/status` · `POST /api/v1/copilot/acao` (executar ação confirmada).
**Critérios de aceitação.** Indisponível sem IA; resposta direta; ferramenta→resposta; confirmação; executar reprocessar; executar criar_regra (como rascunho). (`internal/service/copilot_service_test.go`.)
**Rastreabilidade.** Código: `internal/ai/{cliente,claude,gemini,ollama}.go`, `internal/model/copilot.go`, `internal/service/copilot_service.go`, `internal/handler/copilot_handler.go`. UI: menu "Assistente IA" em `app.html`.

#### RF-139 — [LAB] Diagnóstico de dupla contabilização
**Descrição.** Avalia as regras **ativas** contra a posição de uma data (igual à geração) e aponta **boletos atingidos por mais de uma regra distinta** — risco de lançamento em dobro que o lint estático (RF-129) **não** pega, porque as expressões são diferentes. Também lista condições que **não casaram nenhuma posição** (regra possivelmente morta na data) e condições que referenciam **campos ausentes**.
**Regras.**
- RN-139.1 (Data-driven): para cada posição, avalia pré-condição + condições reusando os helpers do motor (`regraAplicaAPosicao`, `camposFaltantes`, `boletoDaPosicao`, `campoBoletoOuPadrao`); registra os disparos por boleto.
- RN-139.2 (Conflito): boleto com disparos de **≥2 regras distintas** (`id_regra` diferentes) = possível dupla contabilização.
- RN-139.3 (Data default): sem `data`, usa a última data com posição (`UltimaDataPosicao`). Listas truncadas por limite (500).
- RN-139.4 (Flag, ADR-022): atrás de `FEATURES_LAB` (default ligado); desligado → **501** e menu oculto.
**Endpoints.** `GET /api/v1/lab/status` → `{ativo}` · `GET /api/v1/lab/diagnostico` (`data?`, `produto?`, `dominio?`) → `DiagnosticoRegras { conflitos[], sem_correspondencia[], campos_ausentes[] }`.
**Critérios de aceitação.** Dado um boleto que satisfaz condições de duas regras ativas diferentes numa data, Quando diagnostico a data, Então o boleto aparece em `conflitos` com as duas regras. _(Verificado manualmente de ponta a ponta; **teste automatizado pendente** — dívida de §0.3.)_
**Rastreabilidade.** Código: `internal/model/lab.go`, `internal/service/lab_service.go` (`Diagnosticar`), `internal/handler/lab_handler.go`. UI: menu "Diagnóstico" em `app.html`. Marcação de remoção: `grep "[LAB]"`.

#### RF-140 — [LAB] Linhagem de lançamento ("por que esse lançamento?")
**Descrição.** Para um lançamento gerado, reconstrói a **regra/condição** que o originou e os **campos da posição vigente** que casaram, reavaliando a condição e o `campo_valor`. É honesta quando a regra/posição **mudou desde a geração**.
**Regras.**
- RN-140.1 (Entrada): identificação pela própria linha da consulta de movimentos (`data`, `boleto`, `id_regra`, `descricao_condicao`, `produto`, `dominio`).
- RN-140.2 (Reconstrução): localiza a regra ativa por `id_regra` e a condição pela expressão armazenada; acha a posição do boleto (versão vigente) por `campoBoletoOuPadrao`; lista os campos referenciados (pré-condição + condição + `campo_valor`) com seus valores; reavalia e sinaliza `condicao_ok`.
- RN-140.3 (Honestidade): se a regra não está mais ativa, a posição não foi encontrada, ou a condição **não casa mais**, retorna observação explicativa (ex.: "a regra pode ter sido alterada após a geração").
- RN-140.4 (Flag, ADR-022): atrás de `FEATURES_LAB`; na UI é um botão por linha na **Consulta** (só aparece quando ativo).
**Endpoint.** `POST /api/v1/lab/linhagem` (corpo = `LinhagemInput`) → `Linhagem { encontrada, regra, condicao, condicao_ok, valor, campos[] , observacao }`.
**Critérios de aceitação.** Dado um lançamento cuja regra ativa ainda casa a posição, Quando peço a linhagem, Então recebo a condição, o valor calculado e os campos usados; Dado que a regra mudou e não casa mais, Então recebo a observação de divergência. _(Verificado manualmente; **teste automatizado pendente**.)_
**Rastreabilidade.** Código: `internal/model/lab.go`, `internal/service/lab_service.go` (`Linhagem`), `internal/handler/lab_handler.go`. UI: modal "Por que esse lançamento?" + botão na grade de Consulta (`app.html`).
