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
| Leitura de XLSX | `github.com/xuri/excelize/v2` |
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
| `STORAGE_BACKEND` | `file` | Backend de persistência: `sqlserver` ou `file` |
| `DB_SERVER` | `LOCALHOST\SQLEXPRESS` | Servidor SQL Server |
| `DB_NAME` | `srcoff` | Nome do banco de dados |
| `API_PORT` | `8080` | Porta da API |
| `FRONTEND_PORT` | `9090` | Porta do Frontend |
| `API_URL` | `http://localhost:8080` | URL da API consumida pelo frontend |
| `FILE_STORAGE_DIR` | `./data` | Diretório dos arquivos JSON (backend file) |
| `CAMPO_BOLETO_PADRAO` | `codigo_identificador_boleto` | Campo da posição usado como identificador do boleto na conciliação/exportação e como fallback quando a condição não define `campo_boleto` |

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

Armazena a posição diária de carteira offshore, de forma **totalmente dinâmica**.
Apenas os metadados do lote são colunas fixas; todos os campos de negócio ficam
serializados em JSON na coluna `campos`. Suporta múltiplas versões por data.

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `id` | BIGINT IDENTITY PK | Identificador único |
| `data_posicao_carteira` | DATE | Data da posição (informada no upload) |
| `codigo_versao_conteudo` | INT | Versão do conteúdo (maior = mais recente) |
| `campos` | NVARCHAR(MAX) | JSON com todos os campos de negócio do arquivo importado |

> **Não há acoplamento** entre a estrutura da posição e o código. Qualquer coluna
> do arquivo importado (CSV/XLSX) fica disponível nas expressões das regras pelo
> seu nome normalizado (snake_case, sem acento). Backend `file`: cada registro é
> um mapa JSON flat. Aplique a migration `002_posicao_dinamica.sql` no SQL Server.

### regra_contabil

Define as regras de roteamento contábil.

| Coluna | Tipo | Descrição |
|--------|------|-----------|
| `id` | BIGINT IDENTITY PK | Identificador único |
| `descricao` | VARCHAR(255) | Nome da regra |
| `codigo_produto_corporativo` | VARCHAR(50) | Código(s) do produto, separados por vírgula (ex: `NDF,SWAP`) — opcional |
| `campo_produto` | VARCHAR(100) | Campo da posição comparado com o produto (default: `produto`) |
| `campo_data` | VARCHAR(100) | Nome da coluna do arquivo que contém a data da posição (usado na importação) |
| `pre_condicao` | VARCHAR(1000) | Expressão opcional combinada com **E** a cada condição da regra |
| `natureza` | VARCHAR(255) | Texto informativo (combo parametrizável) sobre a natureza da regra |
| `ativo` | BIT | Se a regra está ativa |

> A regra só é aplicada às posições cujo **produto** (informado no upload e gravado no campo `produto`) coincide com `codigo_produto_corporativo`. Ex.: ao processar uma posição de NDF, apenas as regras de NDF são aplicadas. Regra sem produto aplica-se a todas as posições.
>
> **Pré-condição:** quando preenchida, é avaliada uma vez por posição/regra; se falsa, nenhuma condição da regra é aplicada. Útil para fatorar uma condição comum a várias condições da regra (ex: `descricao_veiculo == "NASSAU"`), evitando repeti-la em cada condição.
>
> **Campo Data:** na importação, a coluna de data é determinada pelo `campo_data` da regra do produto informado; se não configurado, o sistema tenta detectar automaticamente uma coluna cujo nome comece com `data`.

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
| `campo_boleto` | VARCHAR(100) | Campo da posição usado como identificador do boleto no lançamento (fallback: `CAMPO_BOLETO_PADRAO`) |
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
- **Inconsistências:** se a pré-condição, condição ou campo_valor de uma regra referenciar um campo que **não existe** na posição, o lançamento **não é gerado** e uma inconsistência é registrada. Após o processamento, as inconsistências da data são listadas na tela.

**Inconsistências do Processamento:**
- As inconsistências são persistidas por data do movimento e podem ser consultadas informando a data
- Cada registro traz boleto, produto, regra, tipo (`PRE_CONDICAO` / `CONDICAO` / `CAMPO_VALOR`), a expressão que falhou, os campos faltantes e o detalhe
- Botão **Exportar CSV** gera um arquivo com esses detalhes (BOM UTF-8, separador `;`)
- Reprocessar a mesma data **substitui** as inconsistências anteriores

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

Página **somente leitura + importação** — não há mais inserção nem exclusão manual de registros.

**Consultar:**
- **Selecione o produto (obrigatório)** e informe uma data (ou período), depois clique em "Consultar"
- Exibe grid dinâmico com todas as colunas do arquivo importado (inclusive colunas adicionais), filtrado pelo produto

**Importar (individual — CSV / XLSX):**
- Selecione o **produto** da posição (combo com opções parametrizadas) e um arquivo `.csv` ou `.xlsx`
- A **data** de cada registro vem de uma **coluna do próprio arquivo**, cujo nome é definido no `campo_data` da regra do produto (fallback: coluna cujo nome comece com `data`)

**Importar em lote (produto pelo nome do arquivo):**
- Selecione **vários arquivos** de uma vez; o produto de cada um é identificado por **padrões de nome** parametrizados (ex: `posicao_ndf*.csv` → NDF)
- Um arquivo que casa com **mais de um padrão** é importado para **cada produto** correspondente
- A data continua vindo de uma coluna do conteúdo (mesma lógica do import individual)

**Pasta monitorada (automático):**
- Uma **pasta monitorada** parametrizada é varrida periodicamente e também sob demanda ("Escanear agora")
- O **intervalo de varredura é parametrizável em minutos**; se estiver vazio ou 0, o monitoramento automático fica **desativado** (o botão "Escanear agora" continua funcionando)
- Arquivos que casam com um padrão são importados e **movidos** para a **pasta de processados** (também parametrizada)
- Arquivos **sem padrão** correspondente **permanecem intactos** na pasta monitorada

> A importação (individual, em lote ou por varredura) apenas **persiste a posição** — não avalia as regras contábeis. As regras (condições, pré-condições) só são aplicadas ao **Gerar Movimento Contábil**. O único uso das regras na importação é ler o `campo_data` para localizar a coluna de data.
- O produto informado é gravado no campo `produto` de cada registro e determina quais regras serão aplicadas
- A primeira linha do arquivo deve conter os nomes das colunas; qualquer coluna é aceita e fica disponível nas regras
- "Pré-visualizar" mostra as primeiras linhas e a coluna de data detectada antes de confirmar
- "Importar" persiste os registros; a versão é atribuída automaticamente por **(data, produto)**, de modo que produtos distintos da mesma data coexistem no snapshot vigente
- Números com vírgula decimal (formato BR) e valores `true/false`/`sim/não` são convertidos automaticamente

---

### Parametrizações (`/parametrizacoes`)

Menu para manter as opções disponíveis nos combos do sistema (extensível para novas funcionalidades futuras). Atualmente:

- **Produtos** — lista de produtos usada nos combos de Produto (upload de posição e cadastro de regra). Adicione/remova opções livremente.
- **Naturezas da Regra** — lista usada no combo de Natureza da regra contábil.

Os campos Produto (upload e regra) e Natureza (regra) deixam de ser texto livre e passam a ser selecionados a partir dessas listas.

- **Padrões de Arquivo → Produto** — mapeia um padrão de nome de arquivo (glob, ex: `posicao_ndf*.csv`) a um produto. Usado na importação em lote e no monitoramento de pasta. Um arquivo pode casar com mais de um padrão (importa para cada produto).
- **Pastas de Importação** — configura a **pasta monitorada** (varrida automaticamente) e a **pasta de processados** (para onde os arquivos importados são movidos).

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

| Método | Endpoint                                | Descrição                          |
| --------| -----------------------------------------| ------------------------------------|
| POST   | `/api/v1/movimento-contabil`            | Gera movimento contábil            |
| GET    | `/api/v1/movimento-contabil`            | Consulta lançamentos paginados     |
| DELETE | `/api/v1/movimento-contabil`            | Exclui lançamentos por data/versão |
| GET    | `/api/v1/movimento-contabil/export`     | Exporta CSV                        |
| GET    | `/api/v1/movimento-contabil/export-txt` | Exporta TXT estruturado            |
| POST   | `/api/v1/estorno`                       | Gera estorno                       |
| GET    | `/api/v1/conciliacao`                   | Executa conciliação                |
| GET    | `/api/v1/posicao`                       | Lista posições por **produto** (obrigatório) e data |
| GET    | `/api/v1/inconsistencias`               | Lista inconsistências do processamento (`?data=YYYY-MM-DD`) |
| GET    | `/api/v1/inconsistencias/export`        | Exporta inconsistências em CSV (`?data=YYYY-MM-DD`) |
| POST   | `/api/v1/posicao/upload`                | Importa posição de CSV/XLSX (multipart: `produto` + `arquivo`; data vem de coluna do arquivo; `preview=1` só pré-visualiza) |
| POST   | `/api/v1/posicao/upload-lote`           | Importa vários arquivos (multipart `arquivos`); produto por padrão de nome |
| GET/POST | `/api/v1/posicao/scan-pasta`          | Status (GET) / varredura imediata (POST) da pasta monitorada |
| GET/POST/DELETE | `/api/v1/parametrizacoes/padroes` | CRUD dos padrões nome-de-arquivo → produto |
| GET/PUT | `/api/v1/configuracoes`                | Lê todas / define uma configuração (`{chave, valor}`) — ex: pastas |
| GET    | `/api/v1/posicao/campos`                | Lista os campos disponíveis na posição da data |
| GET    | `/api/v1/parametrizacoes/opcoes`        | Lista opções de uma categoria (`?categoria=produto\|natureza`) |
| POST   | `/api/v1/parametrizacoes/opcoes`        | Adiciona opção (`{categoria, valor}`) |
| DELETE | `/api/v1/parametrizacoes/opcoes`        | Remove opção (`?categoria=...&valor=...`) |
| GET    | `/api/v1/regras`                        | Lista regras                       |
| POST   | `/api/v1/regras`                        | Cria regra                         |
| PUT    | `/api/v1/regras/{id}`                   | Edita regra                        |
| GET    | `/api/v1/regras/{id}/condicoes`         | Lista condições                    |
| POST   | `/api/v1/regras/{id}/condicoes`         | Cria condição                      |
| PUT    | `/api/v1/condicoes/{id}`                | Edita condição                     |

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

# Migrar para posição dinâmica + campo_boleto/campo_produto
sqlcmd -S localhost\SQLEXPRESS -d srcoff -i migrations/002_posicao_dinamica.sql

# Campos extras da regra (campo_data, pre_condicao, natureza)
sqlcmd -S localhost\SQLEXPRESS -d srcoff -i migrations/003_regra_campos_extras.sql

# Parametrizações (opções de combos: produto, natureza)
sqlcmd -S localhost\SQLEXPRESS -d srcoff -i migrations/004_parametrizacao.sql

# Inconsistências de processamento
sqlcmd -S localhost\SQLEXPRESS -d srcoff -i migrations/005_inconsistencia.sql

# Padrões de arquivo → produto e configurações (pastas)
sqlcmd -S localhost\SQLEXPRESS -d srcoff -i migrations/006_padrao_arquivo.sql
```


---

## Atualizações Recentes

### Flag Posta/Reverte nas Regras Contábeis

Ao cadastrar uma regra contábil, é possível marcar se ela é **Posta/Reverte**:

- **Marcada (padrão):** os lançamentos gerados por essa regra serão estornados normalmente no processo de estorno de D-1
- **Desmarcada:** os lançamentos **não serão estornados**, útil para regras de lançamentos definitivos (ex: tarifas, ajustes permanentes)

Para SQL Server, execute o migration:
```sql
ALTER TABLE regra_contabil ADD posta_reverte BIT NOT NULL DEFAULT 1;
```

---

### Campo Produto na Posição de Carteira

Cada registro de posição pode ter um campo `produto` (ex: `NDF`, `SWAP`). As regras contábeis só são aplicadas às posições cujo produto coincide com o `codigo_produto_corporativo` da regra.

- Se a posição não tiver produto → regra se aplica a todas
- Se a regra não tiver produto → aplica a todas as posições

Para SQL Server:
```sql
ALTER TABLE posicao_carteira ADD produto VARCHAR(50) NULL;
```

---

### Conciliação Inteligente com IA (Gemini)

Nova seção na página de **Conciliação** que permite descrever em linguagem natural o que deseja verificar. O sistema analisa a posição e o movimento contábil da data e retorna:

1. **Diagnóstico** — explicação sobre a coerência entre posição e movimento
2. **Sugestão de ajuste** — lançamentos contábeis recomendados (se houver inconsistência)
3. **Confirmação** — o usuário decide se aplica ou descarta os ajustes sugeridos

**Configuração necessária:**
```bash
# Windows CMD
set GEMINI_API_KEY=SUA_CHAVE_AQUI

# PowerShell
$env:GEMINI_API_KEY="SUA_CHAVE_AQUI"
```

Obtenha sua chave em [aistudio.google.com](https://aistudio.google.com).

**Exemplo de pergunta:**
> "As contas 111111111 e 222222222 são contas de Principal para o produto NDF. O saldo de principal_remanescente na posição está coerente com os saldos dessas contas no movimento?"

---

### Estrutura de Navegação (Frontend SPA)

O frontend foi reescrito como SPA (Single Page Application) com Bootstrap 5:

| Página | Rota | Descrição |
|--------|------|-----------|
| Operação | `/` | Gerar movimento contábil (estorno automático incluso) |
| Consulta | `/consulta` | Consultar, exportar (CSV/TXT) e excluir movimentos |
| Posição | `/posicao` | Inserir, consultar e excluir registros de posição |
| Conciliação | `/conciliacao` | Conciliação simples e inteligente com IA |
| Regras | `/regras` | Cadastrar e manter regras e condições contábeis |

> Para desfazer o redesign Bootstrap e voltar ao frontend anterior:
> ```bash
> git checkout v1.0-pre-bootstrap -- cmd/frontend/main.go cmd/frontend/templates/
> go build -o frontend.exe ./cmd/frontend
> ```

---

### Pontos de Restore (Git Tags)

| Tag | Descrição |
|-----|-----------|
| `v1.0-pre-bootstrap` | Antes do redesign frontend com Bootstrap |
| `v1.1-pre-conciliacao-ia` | Antes da conciliação inteligente com Gemini |

Para restaurar:
```bash
git checkout <tag>
```
## Telas do Sistema

### Consulta e Cadastro de Regras/Condições

<img width="2557" height="419" alt="image" src="https://github.com/user-attachments/assets/6c25ec2b-a086-4f1c-a987-ca6825ced747" />

<img width="1140" height="228" alt="image" src="https://github.com/user-attachments/assets/7c82b558-3bad-49ac-93ab-39b2c12d867e" />

<img width="501" height="408" alt="image" src="https://github.com/user-attachments/assets/67277ea5-c363-495a-967b-7d9c5c9208ff" />

<img width="805" height="398" alt="image" src="https://github.com/user-attachments/assets/0eb4d187-3a99-408f-aa67-cb396d49eb47" />

### Execução do Movimeto Contábil

<img width="1479" height="476" alt="image" src="https://github.com/user-attachments/assets/2d187d7f-ee5f-457c-9f0b-20bb9630a18d" />

### Consulta do Movimento Contábil

<img width="2559" height="461" alt="image" src="https://github.com/user-attachments/assets/c9cf4935-5eda-48c9-9e8b-c8fba810cfe4" />

### Extrações em .txt e .csv do Movimento Contábil

<img width="590" height="172" alt="image" src="https://github.com/user-attachments/assets/177ace33-4660-434f-9c7c-fc3da78eb151" />

<img width="1035" height="180" alt="image" src="https://github.com/user-attachments/assets/ba4be7e3-1789-4db9-9244-3f53bfa2f955" />

<img width="2129" height="531" alt="image" src="https://github.com/user-attachments/assets/8ff269f8-fd1a-44aa-84da-9728b3d30b4f" />

### Conciliação Movimento x Carteira

<img width="2553" height="687" alt="image" src="https://github.com/user-attachments/assets/323acce0-1c12-4b90-b40f-28819015d329" />

### Consulta e Cadastro de Posição

Essa tela não faz parte da feature de roteirização contábil, porém foi adicinada para facilitar os testes da POC

<img width="2560" height="621" alt="image" src="https://github.com/user-attachments/assets/ba61323c-02b4-4d48-a788-5ea0dd3f3b5b" />

