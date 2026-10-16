# Massa de Testes — Mapa de Boletos para Validação

> Massa determinística gerada e **executada** pelo motor real (`cmd/massa_testes`). Cada boleto isola um cenário de [`cenarios-teste.md`](cenarios-teste.md). Resultados abaixo foram **conferidos na execução**.

| | |
|---|---|
| **Gerado por** | `go run ./cmd/massa_testes` (backend file) |
| **Diretório** | `./data-massa` (regenerável; não versionar) |
| **Produto** | `NDF` (um domínio por tipo, para isolar cada boleto) |
| **Datas** | D1=2026-10-05 · D2=2026-10-06 · D3=2026-10-07 · D4=2026-10-08 · D5=2026-10-09 (seg–sex) |

## Como validar na aplicação

1. Gerar/atualizar a massa:
   ```bash
   go run ./cmd/massa_testes
   ```
2. Subir a aplicação apontando para a massa (dois processos):
   ```bash
   STORAGE_BACKEND=file FILE_STORAGE_DIR=./data-massa ./api.exe
   STORAGE_BACKEND=file FILE_STORAGE_DIR=./data-massa ./frontend.exe
   ```
3. No frontend: **Regras** (vê os tipos), **Consulta de Movimento** (filtra por boleto/data) e **Status/Calendário**.

> **Atenção à consulta padrão:** a tela de Consulta **oculta pares lançamento+estorno de saldo zero** (RN-050.1). Para auditar as linhas cruas (incl. estornos que se cancelam), use a exportação/consulta "com cancelados".

---

## Boletos × cenários × resultado esperado (conferido)

Contas no formato `débito/crédito`. "ESTORNO" = `indicador_reversao = true` (contas invertidas).

### Tipo Reverte — domínio `Reverte`
| Boleto | Cenários | Resultado (vigente) |
|--------|----------|---------------------|
| **70001** | CT-H01, CT-J01, CT-J02 | D1: LANC 1000 (1001/2001) · D2: ESTORNO 1000 (2001/1001) + LANC 1500 (1001/2001) |
| **70002** | CT-L07 (par oculto) | D1: LANC 800 · D2: ESTORNO 800 + LANC 800 → **na consulta padrão o par se cancela e some**; visível só com "cancelados" |

### Tipo Não Reverte — domínio `NaoReverte`
| Boleto | Cenários | Resultado |
|--------|----------|-----------|
| **70101** | CT-H02 | D1: LANC 1000 (1003/2003) · D2: LANC 1200 (1003/2003) · **nenhum estorno** |

### Tipo Incremental (Não Reverte) — domínio `Incremental`
| Boleto | Cenários | Resultado |
|--------|----------|-----------|
| **70201** | CT-H03/04/05/08, CT-K01/K02 | D1: LANC **1000** (cheio) · D2: LANC **300** `|1300-1000|` (**v2** após reprocesso) · D3: LANC **400** `|900-1300|` absoluto |
| **70202** | CT-H09 | D1: LANC 1000 · some em D2 → **saldo permanece, sem baixa** (nenhuma linha nova) |

### Tipo Incremental/Reverte — domínio `IncReverte`
| Boleto | Cenários | Resultado |
|--------|----------|-----------|
| **70301** | CT-H10/11/12/13/16, ADR-007 | D1: LANC 1000 · D2: LANC 300 · D3: LANC 400 · **D4: ESTORNO 1700 (baixa = saldo líquido, não 900 da posição)** · D5: LANC 500 (reaparece = série nova) |
| **70302** | CT-H10 (série limpa) | D1 500 · D2 100 · D3 100 · D4 100 · D5 100 — nunca some, sem baixa |
| **70303** | CT-H14 | D1: LANC 1000 · D2–D5 presente na posição com `valor_mtm<0` (condição falsa) → **nenhum lançamento e NENHUMA baixa** |

### Tipo Incremental/Reverte — domínio `IncRevSome` (combo some inteiro)
| Boleto | Cenários | Resultado |
|--------|----------|-----------|
| **70401** | CT-H15, CT-O04 | D1: LANC 1000 · D2: LANC 300 · **D3: ESTORNO 1300 (baixa)** — combo sem posição em D3, mas baixa versionada (v1) e **execução registrada** (lanc=0/estornos=1) |

### Tipo Incremental/Reverte multi-conta — domínio `MultiConta`
| Boleto | Cenários | Resultado |
|--------|----------|-----------|
| **70501** | CT-H18 | D1: LANC 1000 (1009/2009) + LANC 500 (3009/4009) · D2: LANC 200 + 200 · **D3: ESTORNO 1200 (2009/1009) + ESTORNO 700 (4009/3009)** — baixa dos **dois** pares |

### Tipo Reverte com obrigatoriedade de D-1 — domínios `D1Ok` / `D1Block`
| Boleto | Cenários | Resultado |
|--------|----------|-----------|
| **70601** | CT-I02/I03, CT-J02 | Presente D1/D2/D3 → processa sempre; D2: ESTORNO 1000 + LANC 1200; D3: ESTORNO 1200 + LANC 1000 (estorna o D-1 útil) |
| **70611** | CT-I01 | D1: LANC 1000 · ausente D2 · **D3: BLOQUEADO** (há contábil anterior mas falta o D-1 útil) → nada gerado em D3 |

---

## Execuções relevantes (Status / RN-160)
- **D3 `IncRevSome`**: `lanc=0 estornos=1 bruto=0` → confirma CT-O04 (combo só com baixa é registrado).
- **D3 `MultiConta`**: `lanc=0 estornos=2 bruto=0`.
- **D2 `Reverte`**: `lanc=1 estornos=1 bruto=2` → a consulta mostra 1 (par de 70002 cancela), mas o bruto é 2.

---

## Cenários que NÃO envolvem boleto/lançamento (validar por outros meios)

Estes não têm boleto associado — valide por API/tela ou testes unitários:

- **A** (dias úteis/calendário), **B** (upload/preview/lote/watcher), **C** (padrões), **D** (parametrizações/configurações), **E** (CRUD de regra/condição, validação de expressão), **F** (import/export CSV de regras e normalização de tipo), **M** (exportações CSV/TXT — usam os mesmos boletos acima), **N** (exclusão por data/versão), **P** (inconsistências), **Q** (conciliação), **R** (IA/NL Query), **S** (notificações), **T** (equivalência de backends).

> Observação: CT-M (exportações) pode ser validado exportando CSV/TXT das datas D1–D5 e conferindo os boletos acima.
