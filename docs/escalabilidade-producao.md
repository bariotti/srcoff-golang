# Análise de Escalabilidade — Produção

> Avaliação honesta da arquitetura atual frente a um volume produtivo e **recomendações** priorizadas. **Nada implementado** — material de decisão.

| | |
|---|---|
| **Data** | 2026-10-16 |
| **Volume alvo** | ~300 mil posições/dia (vários produtos/domínios) · ~1 milhão de lançamentos contábeis/dia |
| **Projeção anual** | ~75 M posições/ano · ~250 M lançamentos/ano (252 dias úteis) — **antes** de reprocessamentos, que multiplicam linhas |

---

## Veredito

**Como está, não é escalável para esse volume.** A arquitetura é ótima para **corretude e POC multi-backend**, mas o caminho de geração ("carrega o dia inteiro em memória → avalia em Go single-thread → uma transação gigante") e os backends `file`/`sqlite` são bloqueadores. **Com SQL Server (ou PostgreSQL) + as mudanças abaixo, 1 M/dia é viável**; sem elas, não.

- **`file`**: carrega o JSON inteiro na memória a cada operação → inviável já na casa de dezenas de milhares. **Só dev.**
- **`sqlite` (modernc, pure-Go)**: `SetMaxOpenConns(1)` serializa tudo + driver sem cgo (2–3× mais lento). **Só dev/single-user.**
- **`sqlserver`**: único candidato a produção — mas precisa de particionamento, modelo de vigência, índices e carga em lote.

---

## Gargalos por camada (observados no código)

### 1. Avaliação de expressões — o mais crítico em CPU
- `evaluator.go`: **`expr.Compile` é chamado a CADA `EvaluateCondition`/`EvaluateValue`** (sem cache). A compilação depende só do **texto** da expressão, não do `env` — logo é 100% cacheável.
- `sanitizeEnv` **copia o mapa de campos inteiro** a cada chamada (e são 2+ chamadas por condição).
- Custo: `~300k posições × Nregras × Mcondições` avaliações/dia = **dezenas de milhões de compilações + cópias de mapa**, single-thread. Domina o tempo de geração.

### 2. Geração carrega tudo em memória
- `GerarMovimentoEscopo`: `BuscarPorDataEVersaoMaxima` traz **todas** as posições do dia para um slice; os lançamentos (até ~1 M) são acumulados em outro slice; `BulkInsert` único.
- Pico de memória proporcional ao dia inteiro; pressão de GC; uma transação muito longa.

### 3. Consulta/vigência com subquery correlacionada
- "vigente" e "sem cancelados" usam `codigo_versao_conteudo = (SELECT MAX(...) WHERE data/produto/dominio)` **por linha**, e o filtro de saldo-zero tem outra subquery correlacionada.
- Em 250 M linhas, isso degrada rápido mesmo com índice — é o padrão que pior escala.

### 4. Versionamento acumulativo sem retenção
- Cada reprocesso **mantém** as versões antigas (vigente = MAX). A tabela cresce **sem limite** e as queries de vigência varrem cada vez mais.
- Não há particionamento, arquivamento nem purga.

### 5. Posição como blob JSON
- `posicao_carteira.campos` é **TEXT JSON**; `produto`/`dominio`/`boleto` ficam dentro do JSON. Filtrar por eles exige parse/scan (hoje feito em Go após carregar por data).
- Sem índice nos campos dinâmicos — não escala para consultas que não sejam "por data".

### 6. Carga/escrita linha a linha
- SQL Server: `INSERT ... VALUES` montado por **concatenação de string** (lotes de `maxLinhasInsert`) — funcional, mas longe de um **bulk load** (BULK INSERT / `bcp` / TVP).
- SQLite: `INSERT` preparado linha a linha, conexão única.
- Importação de 300k linhas: parse em Go + uma transação — sem `COPY`/bulk.

### 7. Índices e estatísticas
- Faltam índices compostos para os acessos quentes (ver [performance-sqlite.md](performance-sqlite.md); valem também para SQL Server): `(data, produto, dominio, versao)`, `(data, boleto)`, `execucao(produto,dominio,data)`, etc. Sem `ANALYZE`/estatísticas atualizadas, o planner erra.

### 8. Processo único, sem paralelismo/fila
- Um processo Go, geração sequencial, sem workers por combo, sem fila, sem escala horizontal. Reprocessos concorrentes não têm controle de concorrência por `(data, combo)`.

### 9. Pontos que JÁ ajudam (manter)
- **Saldo materializado** (RN-170) com manutenção incremental delta — evita varrer histórico na baixa do `incremental_reverte`. ✔
- Versionamento por combo e "sem cancelados" bem definidos (corretude). ✔
- Camadas limpas (repos intercambiáveis) — facilitam trocar backend e otimizar isoladamente. ✔

---

## Recomendações (priorizadas; sem implementar agora)

### P0 — Pré-requisitos de produção
1. **Backend único = SQL Server ou PostgreSQL.** `file`/`sqlite` só dev. (PostgreSQL tende a escalar melhor para esse volume e tem particionamento declarativo e `COPY`.)
2. **Particionamento por data** em `posicao_carteira` e `movimento_contabil` (partição diária/mensal) → consultas por data e **purga por retenção** baratas; manutenção de índice por partição.
3. **Modelo de vigência explícito** no lugar da subquery `MAX` correlacionada: uma tabela `versao_vigente(data,produto,dominio)->versao` (ou flag `vigente` mantida na geração). Transforma O(n²) em O(n) com índice e simplifica "sem cancelados".

### P1 — Throughput da geração
4. **Cache de expressões compiladas** no `evaluator` (map `expressão→program`, compilar uma vez). Ganho grande e isolado.
5. **Evitar cópia de `env` por avaliação** (reutilizar/poolar o mapa, ou avaliar condição e valor sobre o mesmo env já sanitizado).
6. **Geração em streaming + paralelismo por combo**: cursor/stream das posições, workers por `(produto,dominio)`, **commits em chunks** — em vez de dia inteiro em memória + transação única.
7. **Bulk load**: importação e persistência via `BULK INSERT`/`bcp`/**TVP** (SQL Server) ou `COPY` (Postgres), não `INSERT` concatenado linha a linha.

### P2 — Modelo de dados e consulta
8. **Promover campos quentes** (`produto`, `dominio`, `boleto`) a colunas reais e indexadas na posição (manter o JSON para o resto).
9. **Índices compostos + estatísticas** (lista em [performance-sqlite.md](performance-sqlite.md)); `FILLFACTOR`/`ANALYZE`/manutenção.
10. **Arquivamento/retenção**: mover versões superseded e dados frios para tabelas/partições de histórico; purga agendada.

### P3 — Operação e escala
11. **Fila/assíncrono**: disparar geração por fila (um job por combo/data) → retry, paralelismo e **escala horizontal**.
12. **Controle de concorrência** por `(data, combo)` (lock/advisory) para reprocessos simultâneos.
13. **Pool de conexões** adequado; abandonar `MaxOpenConns(1)`/pure-Go em produção.
14. **Observabilidade**: métricas de tempo/volume de geração, locks, planos de execução (`EXPLAIN`).

---

## Esboço de arquitetura-alvo (conceitual)

```
Importação (bulk/COPY) ──▶ posicao_carteira (particionada por data, campos quentes indexados)
                                   │
            fila de jobs por (data, produto, domínio)
                                   ▼
   Geração paralela (workers) ──▶ movimento_contabil (particionada por data)
     - expressões compiladas em cache          + versao_vigente (sem subquery MAX)
     - stream de posições, commits em chunks    + saldo_conta (projeção, já existe)
                                   │
                        Retenção/arquivamento (purga de versões superseded)
```

## Resumo executivo

- **Corretude**: sólida. **Escala atual**: não, no volume citado.
- **Maiores alavancas**: (1) sair de file/sqlite para SQL Server/Postgres; (2) particionar + modelo de vigência explícito; (3) cache de expressões + geração paralela em streaming; (4) bulk load.
- Com esse pacote, 1 M lançamentos/dia é factível num RDBMS bem dimensionado. A estrutura em camadas do projeto facilita evoluir cada peça sem reescrever o domínio.

---

# Anexo — Detalhamento de projeto (desenho, não implementado)

Duas alavancas de maior relação custo/benefício, detalhadas para decisão. Ambas **preservam o resultado** (são otimizações de como o mesmo valor é computado).

## A. Cache de expressões compiladas (+ env sanitizado por posição)

### Problema (confirmado em `internal/evaluator/evaluator.go`)
- `EvaluateCondition`/`EvaluateValue` chamam **`expr.Compile` a cada avaliação**. A compilação depende **apenas do texto** da expressão (não do `env`), então recompilar por posição é desperdício puro.
- `sanitizeEnv` **copia o mapa inteiro de campos** a cada chamada; há 1 sanitização por condição e outra por `campo_valor`.
- Em `300k posições × Nregras × Mcondições`, isso são **dezenas de milhões de compilações + cópias de mapa/dia**, single-thread. A compilação costuma custar **muito mais** que a execução (`expr.Run`).

### Desenho
1. **Cache de programas** no `ExprEvaluator`:
   - Estrutura: `map[chave]*vm.Program` protegido por `sync.RWMutex` (ou `sync.Map`). Chave = `texto + "|" + modo` (modo = `bool` para condição, `valor` para aritmética), porque `expr.Compile(expr, expr.AsBool())` e `expr.Compile(expr)` geram programas diferentes.
   - Fluxo: ao avaliar, procura no cache; se ausente, compila uma vez e guarda; depois sempre `expr.Run(program, env)`.
   - **Thread-safe** (necessário para a geração paralela por combo — item 6 do corpo).
   - Opcional: `singleflight` para não compilar a mesma expressão N vezes no "cold start" sob concorrência.
2. **Env sanitizado uma vez por posição** (não por avaliação):
   - Hoje cada posição avalia pré-condição + condições + `campo_valor` — todas sobre o **mesmo** `env`. Sanitizar **uma vez por posição** e reutilizar o `safeEnv` em todas as expressões daquela posição elimina a maior parte das cópias.
   - Forma possível: um método interno `avaliarComEnv(program, safeEnv)` e sanitização no laço de geração, por posição.

### Propriedades
- **Correção idêntica**: compilação continua sem `expr.Env` (type-check estático), preservando campos dinâmicos; só deixa de recompilar. Resultado byte-a-byte igual.
- **Tamanho do cache**: limitado pelo nº de expressões **distintas** (pré-condições + condições + `campo_valor` de todas as regras) — dezenas/centenas. Sem necessidade de evicção; se um dia as expressões virarem arbitrárias/ilimitadas, um LRU resolve.
- **Invalidção**: expressões mudam só quando a **regra** muda (raro). Como a chave é o texto, uma regra editada vira uma chave nova; entradas órfãs são inofensivas (ou um `limpar()` no recarregamento de regras).
- **Interface estável**: `Evaluator` não muda; o cache é interno. (Opcional: um `PrewarmRegras(regras)` para compilar tudo no início da geração.)

### Impacto e risco
- **Impacto**: remove o custo dominante (compilação) do caminho quente; menos alocação (env por posição). Em geração CPU-bound, ganho tipicamente de **vários×** no tempo de avaliação.
- **Risco**: baixo e isolado no `evaluator`. Validar com teste de equivalência (cache × sem cache) e um benchmark (`go test -bench`).

### Esboço (ilustrativo — não aplicado)
```go
type ExprEvaluator struct {
    mu    sync.RWMutex
    cache map[string]*vm.Program
}
func (e *ExprEvaluator) programa(exprStr string, asBool bool) (*vm.Program, error) {
    k := exprStr; if asBool { k = "b|" + exprStr } else { k = "v|" + exprStr }
    e.mu.RLock(); p := e.cache[k]; e.mu.RUnlock()
    if p != nil { return p, nil }
    var opts []expr.Option; if asBool { opts = append(opts, expr.AsBool()) }
    p, err := expr.Compile(exprStr, opts...); if err != nil { return nil, err }
    e.mu.Lock(); e.cache[k] = p; e.mu.Unlock()
    return p, nil
}
// EvaluateCondition: programa(expr,true) → expr.Run(p, safeEnv)  (safeEnv já sanitizado 1×/posição)
```

---

## B. Modelo de vigência explícito (fim da subquery `MAX` correlacionada)

### Problema (confirmado em `movimento_contabil_repo.go` / `sqlite/movimento.go`)
- "vigente" e "sem cancelados" filtram `codigo_versao_conteudo = (SELECT MAX(...) WHERE mesma data/produto/domínio)` **por linha**; o filtro de saldo-zero aninha outra subquery correlacionada. É o padrão que pior escala em tabelas grandes.

### Desenho recomendado: **tabela-ponteiro de vigência**
```sql
versao_vigente (
  data_lote_contabil,  produto,  dominio,  versao,  atualizado_em,
  PRIMARY KEY (data_lote_contabil, produto, dominio)
)
```
- **Manutenção** (nos mesmos pontos que já calculam a versão):
  - **Geração** (passo 5/6): após definir `versaoPorCombo[combo]=V`, faz `UPSERT versao_vigente(data,combo)=V`.
  - **Lançamento manual**: anexa à vigente → **não muda** o ponteiro.
  - **Estorno avulso**: ao criar nova versão, atualiza o ponteiro do(s) combo(s).
  - **Exclusão**: ao remover a versão vigente, recomputa o ponteiro para o novo MAX (ou remove a linha se não sobrar versão) — ponto sensível, igual ao do saldo.
- **Consulta** (reescrita): troca a subquery por um **join**:
  ```sql
  FROM movimento_contabil m
  JOIN versao_vigente vv
    ON vv.data_lote_contabil = m.data_lote_contabil
   AND vv.produto = m.produto AND vv.dominio = m.dominio
   AND m.codigo_versao_conteudo = vv.versao
  ```
  → **index seek O(n)** no lugar do O(n²) correlacionado. O filtro de saldo-zero passa a somar só sobre as linhas já restritas à vigente.
- **Backfill/reconstrução**: `INSERT ... SELECT data,produto,dominio,MAX(versao) ... GROUP BY data,produto,dominio` (uma vez), e uma rotina `ReconstruirVigencia` reconstruível a partir do movimento — igual à filosofia do `saldo_conta`.

### Alternativa: **flag `vigente` por linha**
- Coluna `vigente BIT` em `movimento_contabil`; consulta vira `WHERE vigente = 1` (índice) — **leitura trivial**.
- Custo: a cada nova versão, **UPDATE** zera o `vigente` das linhas da versão anterior do combo/data → **amplificação de escrita** (pode ser um update grande por reprocesso).
- **Trade-off**: ponteiro = escrita barata (1 upsert/combo) + join na leitura; flag = leitura trivial + escrita cara. Em 1 M linhas/dia com reprocesso, **recomendo o ponteiro** (evita updates massivos); a flag só se a leitura for o gargalo absoluto e reprocesso for raro.

### Propriedades
- **Correção idêntica**: a definição de vigente não muda (MAX por data/produto/domínio); só a forma de computá-la. Resultado igual.
- **Backends**: é uma otimização de SQL (SQL Server/Postgres/SQLite). O backend `file` já resolve vigência em memória; mantém o caminho atual. A manutenção do ponteiro fica no repositório (ex.: método `DefinirVersaoVigente(data,combo,versao)`), chamado pela geração — sem acoplar ao `BulkInsert`.
- **Sinergia**: `versao_vigente` e `saldo_conta` são ambas **projeções derivadas e reconstruíveis**; combinam com **particionamento por data** (join e purga por partição).

### Impacto e risco
- **Impacto**: consultas de movimento (tela, export, status, baixa) e os passos de vigência da geração deixam de depender da subquery correlacionada → ganho grande e crescente com o volume.
- **Risco**: concentra-se na **manutenção em reprocesso/exclusão** (mesma classe de cuidado do saldo). Mitigação: rotina de reconstrução + testes de equivalência (join × subquery) e de consistência após reprocesso/exclusão.

### Ordem sugerida de adoção
1. **A (cache de expressões)** primeiro — isolado, baixo risco, ganho imediato de CPU na geração.
2. **B (vigência explícita)** junto do **particionamento por data** — ataca o maior gargalo de leitura/escala e se beneficia das partições.
