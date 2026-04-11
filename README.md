# SRCOff — Sistema de Roteirização Contábil Offshore

Sistema desenvolvido em Go para geração, estorno, conciliação e consulta do movimento contábil diário da Tesouraria Offshore.

---

## Sumário

- [Visão Geral](#visão-geral)
- [Tecnologias](#tecnologias)
- [Estrutura do Projeto](#estrutura-do-projeto)
- [Configuração e Execução](#configuração-e-execução)
- [Estrutura do Banco de Dados](#estrutura-do-banco-de-dados)
- [Páginas do Sistema](#páginas-do-sistema)
- [API REST](#api-rest)
- [Regras Contábeis — Expressões](#regras-contábeis--expressões)
- [Formato do Arquivo TXT](#formato-do-arquivo-txt)

---

## Visão Geral

O SRCOff processa em lote os registros de posição de carteira offshore para uma data específica, aplica regras contábeis parametrizadas dinamicamente e persiste os lançamentos contábeis resultantes. O sistema também realiza estorno do lote anterior, conciliação e exportação dos dados.

**Fluxo principal:**
1. Operador insere/importa posição de carteira no banco
2. Aciona geração do movimento contábil para uma data
3. Aciona geração do estorno (reverte lançamentos de D-1)
4. Consulta e exporta o lote contábil consolidado

---

## Tecnologias

| Componente | Tecnologia |
|-----------|-----------|
| Linguagem | Go 1.21 |
| Banco de dados | Microsoft SQL Server Express |
| Driver SQL | `github.com/denisenkom/go-mssqldb` |
| Avaliador de expressões | `github.com/expr-lang/expr` |
| Testes de propriedade | `github.com/leanovate/gopter` |
| Frontend | HTML/template (stdlib Go) |

---

## Estrutura do Projeto

```
srcoff/
├── cmd/
│   ├── api/                    → Ponto de entrada da API REST
│   │   └── main.go
│   └── frontend/               → Ponto de entrada do Frontend
│       ├── main.go
│       └── templates/
│           ├── operacao.html
│           ├── consulta.html
│           ├── posicao.html
│           ├── conciliacao.html
│           └── regras.html
├── internal/
│   ├── db/                     → Conexão com banco de dados
│   ├── evaluator/              → Avaliador de expressões dinâmicas
│   ├── handler/                → Handlers HTTP da API
│   ├── model/                  → Structs de domínio
│   ├── repository/             → Repositórios SQL Server
│   │   └── file/               → Repositórios baseados em arquivo JSON
│   └── service/                → Lógica de negócio
├── migrations/
│   ├── 001_create_tables.sql   → DDL das tabelas
│   ├── seed_regras_condicoes.sql
│   └── seed_posicao_carteira_20260409.sql
└── data/                       → Arquivos JSON (backend file)
```

---

## Configuração e Execução

### Variáveis de Ambiente

| Variável | Padrão | Descrição |
|----------|--------|-----------|
| `STORAGE_BACKEND` | `sqlserver` | Backend de persistência: `sqlserver` ou `file` |
| `DB_SERVER` | `DESKTOP-B1QQIIN\SQLEXPRESS` | Servidor SQL Server |
| `DB_NAME` | `srcoff` | Nome do banco de dados |
| `API_PORT` | `8080` | Porta da API |
| `FRONTEND_PORT` | `9090` | Porta do Frontend |
| `API_URL` | `http://localhost:8080` | URL da API consumida pelo frontend |
| `FILE_STORAGE_DIR` | `./data` | Diretório dos arquivos JSON (backend file) |

### Executar com SQL Server

```bash
# Configurar servidor (se diferente do padrão)
set DB_SERVER=localhost\SQLEXPRESS

# Compilar
go build -o api.exe ./cmd/api
go build -o frontend.exe ./cmd/frontend

# Executar (dois terminais separados)
.\api.exe
.\frontend.exe
```

### Executar com Backend de Arquivo (sem SQL Server)

```bash
set STORAGE_BACKEND=file
set FILE_STORAGE_DIR=./data

.\api.exe
.\frontend.exe
```

### Acessar o sistema

- Frontend: http://localhost:9090
- API: http://localhost:8080

---

## Estrutura do Banco de Dados

### posicao_carteira

Armazena a posição diária de carteira offshore. Suporta múltiplas versões por data.

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `id` | BIGINT IDENTITY PK | Identificador único |
| `data_posicao_carteira` | DATE | Data da posição |
| `codigo_versao_conteudo` | INT | Versão do conteúdo (maior = mais recente) |
| `codigo_identificador_boleto` | VARCHAR(50) | Identificador único da operação |
| `descricao_veiculo` | VARCHAR(100) | Veículo da operação (ex: NASSAU) |
| `indicador_contraparte_afiliada` | BIT | Se a contraparte é afiliada |
| `valor_mtm` | DECIMAL(18,6) | Valor Mark-to-Market |
| `principal_remanescente` | DECIMAL(18,6) | Principal remanescente |
| `moeda_principal_remanescente` | VARCHAR(10) | Moeda (BRL, USD, EUR) |

> Colunas adicionais podem ser incluídas livremente — o sistema as carrega automaticamente via `SELECT *` e as disponibiliza nas expressões das regras.

### regra_contabil

Define as regras de roteamento contábil.

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `id` | BIGINT IDENTITY PK | Identificador único |
| `descricao` | VARCHAR(255) | Nome da regra |
| `codigo_produto_corporativo` | VARCHAR(50) | Código do produto (ex: NDF) |
| `ativo` | BIT | Se a regra está ativa |

### condicao_regra

Define as condições e contas de cada regra.

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `id` | BIGINT IDENTITY PK | Identificador único |
| `id_regra` | BIGINT FK | Regra pai |
| `condicao` | VARCHAR(1000) | Expressão booleana (ex: `valor_mtm > 0 && descricao_veiculo == "NASSAU"`) |
| `conta_debito` | VARCHAR(20) | Conta de débito do lançamento |
| `conta_credito` | VARCHAR(20) | Conta de crédito do lançamento |
| `campo_valor` | VARCHAR(500) | Expressão para calcular o valor (ex: `principal_remanescente + valor_mtm`) |
| `campo_moeda` | VARCHAR(100) | Campo da posição que contém a moeda |
| `ativo` | BIT | Se a condição está ativa |

### movimento_contabil

Armazena os lançamentos contábeis gerados.

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `id` | BIGINT IDENTITY PK | Identificador único |
| `data_lote_contabil` | DATE | Data do lote |
| `codigo_versao_conteudo` | INT | Versão do lote |
| `codigo_identificador_boleto` | VARCHAR(50) | Boleto de origem |
| `valor_lancamento_contabil` | DECIMAL(18,6) | Valor do lançamento |
| `moeda_lancamento_contabil` | VARCHAR(10) | Moeda |
| `conta_debito` | VARCHAR(20) | Conta de débito |
| `conta_credito` | VARCHAR(20) | Conta de crédito |
| `indicador_reversao` | BIT | `0` = lançamento normal, `1` = estorno |
| `descricao_regra_contabil` | VARCHAR(255) | Nome da regra que originou |
| `descricao_condicao_contabil` | VARCHAR(1000) | Expressão da condição |
| `id_regra_contabil` | BIGINT FK | Regra de origem |

---

## Páginas do Sistema

### Operação (`/operacao`)

Página principal para acionar o processamento diário.

**Gerar Movimento Contábil:**
- Informe uma data e clique em "Processar"
- O sistema busca a posição de carteira (versão máxima) para a data
- Avalia todas as regras e condições ativas
- Persiste os lançamentos gerados em lote
- Exibe mensagem de confirmação ou erro

**Gerar Estorno:**
- Informe uma data D e clique em "Estornar"
- O sistema busca todos os lançamentos de D-1
- Gera estornos com contas invertidas e `indicador_reversao = true`
- Os estornos são persistidos com a mesma versão do lote de D

---

### Consulta de Movimento Contábil (`/consulta`)

Página para consultar, exportar e excluir lançamentos.

**Filtros disponíveis:**
- Data início / Data fim
- Número do boleto (busca parcial)
- Versão: Vigente (maior por data), Todas, Específica
- Registros por página: 10, 50, 100

**Grid de resultados:**
- Exibe: Data, Versão, Boleto, Conta Débito, Conta Crédito, Valor, Moeda, Reversão, Regra
- Lançamentos com saldo líquido zero (par lançamento + estorno) são automaticamente ocultados
- Paginação com navegação anterior/próxima

**Exportar Excel (CSV):**
- Botão "⬇ Excel" ao lado do "Consultar"
- Usa os mesmos filtros da pesquisa atual
- Gera arquivo `.csv` com BOM UTF-8, separador `;`

**Exportar TXT:**
- Seção separada com campo de data única
- Gera arquivo `.txt` no formato estruturado (ver seção abaixo)
- Exibe mensagem se não há dados para a data

**Excluir Movimento:**
- Informe data (obrigatória) e versão (opcional)
- Se versão omitida, exclui todos os lançamentos da data
- Confirmação via dialog antes de excluir

---

### Posição de Carteira (`/posicao`)

Página para gerenciar os registros de posição.

**Consultar:**
- Informe uma data e clique em "Consultar"
- Exibe grid dinâmico com todas as colunas da tabela (incluindo colunas adicionais)
- Botão "Excluir" em cada linha

**Inserir:**
- Formulário com campos: Data, Versão, Boleto, Veículo, Contraparte Afiliada, Valor MTM, Principal Remanescente, Moeda
- Versão padrão: 1

> Para inserir colunas adicionais (ex: `accrual_ativo`), use INSERT direto no banco ou ajuste o formulário.

---

### Conciliação (`/conciliacao`)

Página para verificar inconsistências entre posição e movimento contábil.

**Como usar:**
- Informe uma data e clique em "Conciliar"
- O sistema compara a posição (versão máxima) com o movimento (versão vigente)

**Inconsistências detectadas:**

| Tipo | Descrição |
|------|-----------|
| `POSICAO_SEM_MOVIMENTO` | Boleto presente na posição sem lançamento no movimento |
| `LANCAMENTO_DUPLICADO` | Mais de um lançamento para mesmo boleto + regra + indicador de reversão |

**Resultado:**
- Resumo: total de posições, total de movimentos, quantidade de inconsistências
- Grid com tipo (badge colorido), boleto, regra, reversão e detalhe
- Mensagem verde quando não há inconsistências

---

### Regras Contábeis (`/regras`)

Página para cadastrar e manter as regras de roteamento contábil.

**Regras:**
- Lista todas as regras ativas com ID, descrição e código do produto
- Botão "Ver Condições" para expandir as condições de cada regra
- Formulário para criar nova regra (descrição + código produto)

**Condições:**
- Grid com ID, expressão de condição, contas débito/crédito, campo valor e campo moeda
- Botão "Editar" para alterar cada condição
- Formulário para adicionar nova condição à regra selecionada

**Regras NDF Nassau (pré-cadastradas):**

| Condição | Débito | Crédito | Valor |
|----------|--------|---------|-------|
| Nassau + Afiliada + MTM > 0 | 111111111 | 222222222 | `principal_remanescente + valor_mtm` |
| Nassau + Afiliada + MTM < 0 | 333333333 | 444444444 | `principal_remanescente` |
| Nassau + Não Afiliada + MTM > 0 | 555555555 | 666666666 | `principal_remanescente + valor_mtm` |
| Nassau + Não Afiliada + MTM < 0 | 777777777 | 888888888 | `principal_remanescente` |

---

## API REST

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| POST | `/api/v1/movimento-contabil` | Gera movimento contábil |
| GET | `/api/v1/movimento-contabil` | Consulta lançamentos paginados |
| DELETE | `/api/v1/movimento-contabil` | Exclui lançamentos por data/versão |
| GET | `/api/v1/movimento-contabil/export` | Exporta CSV |
| GET | `/api/v1/movimento-contabil/export-txt` | Exporta TXT estruturado |
| POST | `/api/v1/estorno` | Gera estorno |
| GET | `/api/v1/conciliacao` | Executa conciliação |
| GET | `/api/v1/posicao` | Lista posições por data |
| POST | `/api/v1/posicao` | Insere posição |
| DELETE | `/api/v1/posicao` | Exclui posição por ID |
| GET | `/api/v1/regras` | Lista regras |
| POST | `/api/v1/regras` | Cria regra |
| PUT | `/api/v1/regras/{id}` | Edita regra |
| GET | `/api/v1/regras/{id}/condicoes` | Lista condições |
| POST | `/api/v1/regras/{id}/condicoes` | Cria condição |
| PUT | `/api/v1/condicoes/{id}` | Edita condição |

---

## Regras Contábeis — Expressões

O sistema usa a biblioteca `expr-lang` para avaliar expressões. A sintaxe é similar a Go/JavaScript.

**Operadores suportados:**

| Operação | Operador |
|----------|----------|
| Igual | `==` |
| Diferente | `!=` |
| Maior / Menor | `>`, `<`, `>=`, `<=` |
| E lógico | `&&` |
| Ou lógico | `\|\|` |
| Negação | `!` |

**Exemplos de condições:**
```
descricao_veiculo == "NASSAU" && valor_mtm > 0
accrual_ativo != 0 && indicador_contraparte_afiliada == true
valor_mtm < 0 || principal_remanescente > 1000000
```

**Exemplos de campo_valor:**
```
principal_remanescente + valor_mtm
principal_remanescente
valor_mtm * 1.1
accrual_ativo
```

**Campos disponíveis (padrão):**
- `id`, `data_posicao_carteira`, `codigo_versao_conteudo`
- `codigo_identificador_boleto`, `descricao_veiculo`
- `indicador_contraparte_afiliada`, `valor_mtm`
- `principal_remanescente`, `moeda_principal_remanescente`
- Qualquer coluna adicional adicionada à tabela `posicao_carteira`

---

## Formato do Arquivo TXT

```
C;20260409
D;111111111;D;USD;NDF Nassau - Afiliada MTM+;BOL-A-001;N;101500.000000
D;222222222;C;USD;NDF Nassau - Afiliada MTM+;BOL-A-001;N;101500.000000
D;333333333;D;BRL;NDF Nassau - Afiliada MTM-;BOL-B-001;N;50000.000000
D;444444444;C;BRL;NDF Nassau - Afiliada MTM-;BOL-B-001;N;50000.000000
T;151500.000000
```

**Estrutura:**
- `C;AAAAMMDD` — Cabeçalho com data
- `D;{conta};{D/C};{moeda};{regra};{boleto};{S/N};{valor}` — Linha de detalhe
  - `D` = conta débito, `C` = conta crédito
  - `S` = reversão, `N` = lançamento normal
- `T;{soma}` — Totalizador com soma de todos os valores

---

## Scripts SQL

```bash
# Criar tabelas
sqlcmd -S localhost\SQLEXPRESS -d srcoff -i migrations/001_create_tables.sql

# Inserir regras NDF Nassau
sqlcmd -S localhost\SQLEXPRESS -d srcoff -i migrations/seed_regras_condicoes.sql

# Inserir massa de teste de posição (09/04/2026)
sqlcmd -S localhost\SQLEXPRESS -d srcoff -i migrations/seed_posicao_carteira_20260409.sql
```
