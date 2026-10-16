# Análise de Performance — Backend SQLite

> Diagnóstico de por que o SQLite parece mais lento que o backend de arquivo e **recomendações** (índices, PRAGMAs, ajustes). **Nada aplicado** — material de decisão.

| | |
|---|---|
| **Data** | 2026-10-13 |
| **Driver** | `modernc.org/sqlite` (pure-Go, sem cgo) |
| **Escopo** | `internal/repository/sqlite/*` + `Open` |

---

## 1. Por que "parece mais lento" que o arquivo

O backend **file** mantém cada coleção num JSON pequeno e filtra **em memória, em Go** — sem round-trip, sem serialização SQL, sem fsync por operação. Para o **volume atual (dezenas de linhas)** isso é imbatível em latência.

O backend **sqlite** paga custos fixos que, nesse volume pequeno, dominam o tempo:

1. **Conexão única** — `Open` fixa `db.SetMaxOpenConns(1)` ([db.go:29](../internal/repository/sqlite/db.go)). **Todas** as queries (inclusive leituras) são **serializadas** numa só conexão. Com o frontend disparando várias chamadas à API, elas enfileiram. É o maior fator de latência percebida na UI.
2. **Escrita com fsync a cada transação** — o `journal_mode` default é `DELETE` (rollback journal) e `synchronous=FULL`. Cada importação/`BulkInsert` sincroniza no disco. Importar vários arquivos sente isso.
3. **Driver pure-Go** — `modernc.org/sqlite` é ~2–3× mais lento que o SQLite C (sem cgo). Custo fixo, não contornável sem trocar o driver.
4. **Overhead HTTP + (de)serialização** por chamada, que no file é quase nada.

> **Importante:** no volume atual, o gargalo **não** é falta de índice (tabelas minúsculas varrem instantaneamente). Os índices abaixo são **preventivos** — passam a importar quando `movimento_contabil`/`posicao_carteira` crescerem para milhares de linhas. A partir daí, o file degrada **mais rápido** (recarrega e reparseia o JSON inteiro a cada consulta, sem índice), e o SQLite com os índices certos escala melhor.

---

## 2. Índices atuais

| Tabela | Índice existente |
|--------|------------------|
| `posicao_carteira` | `IX_posicao_data_versao (data_posicao_carteira, codigo_versao_conteudo)` |
| `movimento_contabil` | `IX_movimento_data_versao (data_lote_contabil, codigo_versao_conteudo)` · `IX_movimento_data_reversao (data_lote_contabil, indicador_reversao)` |
| `parametrizacao` | `UX_param_categoria_valor (categoria, valor)` único |
| demais | só a PK |

---

## 3. Índices recomendados (DDL pronto)

Mapeados às consultas reais. Ordem = maior impacto previsto quando o volume crescer.

```sql
-- (1) Consulta de movimento "vigente" e "sem cancelados": a subquery correlacionada
--     MAX(codigo_versao_conteudo) agrupa por (data, produto, dominio) e roda por linha.
--     Hoje não há índice cobrindo essa chave → escala mal.
--     Serve: movimento.go BuscarPorDataEIndicador (L77), ConsultarPaginadoFiltrado (L147/160).
CREATE INDEX IF NOT EXISTS IX_mov_data_prod_dom_ver
  ON movimento_contabil (data_lote_contabil, produto, dominio, codigo_versao_conteudo);

-- (2) Filtro/junção por boleto (consulta por boleto e a BAIXA do incremental_reverte,
--     que lê o histórico vigente e agrega por boleto). Serve: ConsultarPaginadoFiltrado
--     com boleto; bloco 4b de geração.
CREATE INDEX IF NOT EXISTS IX_mov_data_boleto
  ON movimento_contabil (data_lote_contabil, codigo_identificador_boleto);

-- (3) Resolução de datas na GERAÇÃO (hot): obrigatoriedade D-1, data de estorno e D-N
--     do incremental. Serve: execucao.go DatasExecutadas/DatasComMovimento (L61/65) e o
--     DELETE por (data_lote, produto, dominio) em RegistrarExecucao.
CREATE INDEX IF NOT EXISTS IX_exec_prod_dom_data
  ON movimento_execucao (produto, dominio, data_lote);

-- (4) Status por data. Serve: execucao.go ListarPorData (L39).
CREATE INDEX IF NOT EXISTS IX_exec_data
  ON movimento_execucao (data_lote);

-- (5) Inconsistências por data (listagem e SubstituirPorEscopo). Serve: inconsistencia.go L25/47.
CREATE INDEX IF NOT EXISTS IX_incons_data
  ON inconsistencia_processamento (data_lote_contabil);

-- (6) Condições por regra — elimina a varredura no N+1 do carregamento de regras
--     (ListarRegrasAtivas chama ListarCondicoes por regra a cada geração). Serve: regra.go L76.
CREATE INDEX IF NOT EXISTS IX_cond_regra
  ON condicao_regra (id_regra, ativo);
```

**Após criar (e depois de cargas grandes):**
```sql
ANALYZE;   -- gera estatísticas (sqlite_stat1) p/ o planner escolher os índices. Persistente.
```

**Trade-off:** cada índice novo adiciona custo marginal de escrita (manutenção do índice no insert). Os 6 acima são sobre colunas de leitura quente; o impacto na importação é pequeno e compensa na consulta/geração.

---

## 4. PRAGMAs (maior ganho de latência real)

| PRAGMA | Efeito | Persistente? | Como aplicar sem mexer no código |
|--------|--------|--------------|----------------------------------|
| `journal_mode=WAL` | Escrita mais rápida; leitor não bloqueia escritor | **Sim** (fica no arquivo .db) | rodar `PRAGMA journal_mode=WAL;` **uma vez** |
| `synchronous=NORMAL` | Menos fsync (seguro com WAL) | Não (por conexão) | precisa setar no `Open` (código) |
| `temp_store=MEMORY`, `cache_size=-16000`, `mmap_size` | Menos I/O temporário | Não (por conexão) | código |

> `PRAGMA journal_mode=WAL;` é o ajuste de **maior relação custo/benefício** e **não exige alteração de código** (persiste no arquivo). `synchronous=NORMAL` complementa, mas é por conexão.

---

## 5. Ajustes de código (futuros — não agora)

Estes são os que mais reduziriam a latência de UI, mas exigem mudança em `internal/repository/sqlite`:

1. **Pool de conexões com WAL** — hoje `SetMaxOpenConns(1)` serializa tudo ([db.go:29](../internal/repository/sqlite/db.go)). Com `journal_mode=WAL` dá para permitir **vários leitores + 1 escritor** (ex.: `SetMaxOpenConns(N)` e um mutex/serialização só nas escritas), eliminando o enfileiramento de leituras.
2. **Eliminar o N+1 de condições** — `ListarRegrasAtivas` faz uma query de condições **por regra** ([regra.go:40](../internal/repository/sqlite/regra.go)). Uma única query (`WHERE id_regra IN (...)` ou `LEFT JOIN`) carrega tudo de uma vez. Chamado no início de **toda** geração.
3. **`synchronous=NORMAL` + cache** no `Open` (ver §4).

---

## 6. Como medir (sem alterar nada)

- Plano de execução de uma consulta: `EXPLAIN QUERY PLAN <sua SELECT>;` — mostra se está usando índice (`SEARCH ... USING INDEX`) ou varrendo (`SCAN`).
- Tamanho atual: `SELECT COUNT(*) FROM movimento_contabil;` — confirma se o volume já justifica índices.

---

## 7. Recomendação

1. **Imediato, sem código:** `PRAGMA journal_mode=WAL;` (uma vez) + criar os **índices §3** + `ANALYZE;`. Ganho em escrita e proteção para quando o volume crescer.
2. **Quando puder tocar no código:** `synchronous=NORMAL`, pool de conexões com WAL e remover o N+1 (§5) — é o que derruba a latência de UI percebida.
3. **Expectativa honesta:** no volume atual, mesmo com tudo isso, o file tende a parecer igual ou um tiquinho mais rápido (é in-memory). O SQLite compensa na **escala** (milhares de lançamentos), na **concorrência** e na **consulta filtrada** — que é onde o file passa a recarregar/parsear tudo.
