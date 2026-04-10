# Tarefas de Implementação — SRCOff: Roteirização Contábil Offshore

## Fase 1: Estrutura do Projeto e Infraestrutura

- [x] 1.1 Inicializar módulo Go e estrutura de diretórios
  - Criar `go.mod` com módulo `srcoff`
  - Criar estrutura de pastas: `cmd/api`, `cmd/frontend`, `internal/handler`, `internal/service`, `internal/repository`, `internal/evaluator`, `internal/model`, `internal/db`
  - Adicionar dependências: `github.com/denisenkom/go-mssqldb`, `github.com/expr-lang/expr`, `github.com/leanovate/gopter`
  - **Requisito:** 11.1

- [x] 1.2 Implementar conexão com o banco de dados (Trusted Connection)
  - Criar `internal/db/db.go` com função `Connect()` que retorna `*sql.DB` usando Trusted Connection para DESKTOP-BBARIOTTI
  - Registrar erro no log e encerrar com `os.Exit(1)` se a conexão falhar na inicialização
  - **Requisito:** 11.1, 11.2

- [x] 1.3 Criar scripts DDL das tabelas
  - Criar `migrations/001_create_tables.sql` com DDL de `posicao_carteira`, `regra_contabil`, `condicao_regra`, `movimento_contabil` conforme modelos de dados do design
  - Incluir índices definidos no design
  - **Requisito:** 3.1, 5.1

## Fase 2: Modelos de Domínio

- [x] 2.1 Implementar structs de domínio
  - Criar `internal/model/posicao_carteira.go` com struct `PosicaoCarteira`
  - Criar `internal/model/regra_contabil.go` com structs `RegraContabil` e `CondicaoRegra`
  - Criar `internal/model/lancamento_contabil.go` com struct `LancamentoContabil`
  - Criar `internal/model/pagination.go` com struct `PaginaLancamentos`
  - **Requisito:** 3.1

## Fase 3: Avaliador de Expressões Dinâmicas

- [x] 3.1 Implementar o Avaliador_Expressao
  - Criar `internal/evaluator/evaluator.go` com interface `Evaluator` e implementação usando `github.com/expr-lang/expr`
  - Implementar `EvaluateCondition(expr string, env map[string]interface{}) (bool, error)`
  - Implementar `EvaluateValue(expr string, env map[string]interface{}) (float64, error)`
  - Implementar conversão de `PosicaoCarteira` para `map[string]interface{}`
  - **Requisito:** 2.1, 2.2

- [x] 3.2 Implementar tratamento de erros do avaliador
  - Garantir que erros de avaliação sejam registrados no log com contexto (data, boleto, expressão, erro)
  - Garantir que erros não interrompam o processamento do lote
  - **Requisito:** 2.4, 2.5

- [x] 3.3 Escrever testes de propriedade para o avaliador (P2)
  - Usar `gopter` com `MinSuccessfulTests: 100`
  - Tag: `// Feature: srcoff-roteirizacao-contabil-offshore, Property 2: Avaliador de expressão é determinístico`
  - Gerar expressões e envs aleatórios; verificar que duas chamadas com os mesmos argumentos retornam o mesmo resultado
  - **Requisito:** 2.1, 2.2

## Fase 4: Repositórios

- [x] 4.1 Implementar repositório de posicao_carteira
  - Criar `internal/repository/posicao_carteira_repo.go`
  - Implementar `BuscarPorDataEVersaoMaxima(ctx, data) ([]PosicaoCarteira, error)` — filtra por data e seleciona apenas registros com o maior `codigo_versao_conteudo`
  - **Requisito:** 1.1, 1.2

- [x] 4.2 Implementar repositório de regras contábeis
  - Criar `internal/repository/regra_contabil_repo.go`
  - Implementar `ListarRegrasAtivas(ctx) ([]RegraContabil, error)` — carrega regras e condições ativas
  - Implementar `CriarRegra`, `EditarRegra`, `ListarCondicoes`, `CriarCondicao`, `EditarCondicao`
  - **Requisito:** 2.3, 7.1–7.6

- [x] 4.3 Implementar repositório de movimento_contabil
  - Criar `internal/repository/movimento_contabil_repo.go`
  - Implementar `BulkInsert(ctx, lancamentos []LancamentoContabil) error` — inserção em lote com múltiplos VALUES
  - Implementar `BuscarPorDataEIndicador(ctx, data, indicadorReversao) ([]LancamentoContabil, error)`
  - Implementar `ObterProximaVersao(ctx, data) (int, error)` — retorna MAX(versao)+1 ou 1
  - Implementar `ConsultarPaginado(ctx, data, pagina, tamanho) (*PaginaLancamentos, error)`
  - **Requisito:** 3.11, 3.12, 5.1, 5.2, 9.2, 9.3

## Fase 5: Serviço de Movimento Contábil

- [x] 5.1 Implementar geração do movimento contábil
  - Criar `internal/service/movimento_contabil_service.go`
  - Implementar `GerarMovimento(ctx, data)`:
    1. Buscar posição com versão máxima para a data
    2. Retornar erro de ausência de dados se posição vazia
    3. Carregar todas as regras e condições ativas
    4. Para cada registro × condição: avaliar expressão booleana; se verdadeira, avaliar expressão de valor e montar `LancamentoContabil`
    5. Calcular `codigo_versao_conteudo` como próxima versão
    6. Bulk insert de todos os lançamentos
  - **Requisito:** 1.1, 1.2, 1.3, 2.1, 2.2, 3.1–3.12

- [x] 5.2 Implementar geração do estorno
  - Implementar `GerarEstorno(ctx, data)`:
    1. Buscar lançamentos de D-1 (indicador_reversao=false)
    2. Retornar erro de ausência se D-1 não existir
    3. Buscar lançamentos de D (indicador_reversao=false)
    4. Comparar por chave (codigo_identificador_boleto, id_regra_contabil)
    5. Gerar estorno para lançamentos com valor divergente ou sem correspondente em D
    6. Preencher indicador_reversao=true, inverter contas
    7. Bulk insert dos estornos
  - **Requisito:** 5.1–5.8

- [x] 5.3 Implementar consulta paginada de lançamentos
  - Implementar `ConsultarLancamentos(ctx, data, pagina, tamanho) (*PaginaLancamentos, error)`
  - **Requisito:** 9.1–9.4

- [x] 5.4 Escrever testes de propriedade para geração de movimento (P1, P3, P4, P5)
  - P1: Tag `// Feature: srcoff-roteirizacao-contabil-offshore, Property 1: Seleção da versão máxima da posição de carteira`
    - Gerar registros com múltiplas versões e datas; verificar que apenas data correta e versão máxima são selecionadas
  - P3: Tag `// Feature: srcoff-roteirizacao-contabil-offshore, Property 3: Lançamentos gerados correspondem às condições satisfeitas`
    - Gerar posições e condições aleatórias; contar pares satisfeitos; comparar com lançamentos gerados
  - P4: Tag `// Feature: srcoff-roteirizacao-contabil-offshore, Property 4: Campos do lançamento contábil são preenchidos corretamente`
    - Gerar posições e condições; verificar todos os campos de cada lançamento contra origem
  - P5: Tag `// Feature: srcoff-roteirizacao-contabil-offshore, Property 5: Versão do lote é sempre incrementada monotonicamente`
    - Gerar lotes sequenciais para a mesma data; verificar incremento estrito
  - **Requisito:** 1.2, 3.1–3.11

- [x] 5.5 Escrever testes de propriedade para estorno (P6, P7)
  - P6: Tag `// Feature: srcoff-roteirizacao-contabil-offshore, Property 6: Invariantes do estorno — inversão de contas e indicador de reversão`
    - Gerar lançamentos de D-1 com divergência; verificar inversão de contas e indicador_reversao=true
  - P7: Tag `// Feature: srcoff-roteirizacao-contabil-offshore, Property 7: Estorno é gerado se e somente se há divergência ou ausência de correspondente`
    - Gerar pares de lotes com combinações de igualdade/divergência/ausência; verificar condição de geração
  - **Requisito:** 5.4–5.7

- [x] 5.6 Escrever testes de propriedade para consulta paginada (P8, P9)
  - P8: Tag `// Feature: srcoff-roteirizacao-contabil-offshore, Property 8: Lote consolidado contém exatamente todos os lançamentos e estornos da data`
    - Gerar lotes completos; verificar que consulta retorna exatamente lançamentos + estornos sem omissões ou duplicatas
  - P9: Tag `// Feature: srcoff-roteirizacao-contabil-offshore, Property 9: Paginação retorna subconjunto correto e total consistente`
    - Gerar N lançamentos; iterar todas as páginas; verificar união igual ao total sem duplicatas
  - **Requisito:** 6.1, 6.2, 9.2, 9.3

## Fase 6: Serviço de Regras Contábeis

- [x] 6.1 Implementar serviço de regras contábeis
  - Criar `internal/service/regra_contabil_service.go`
  - Implementar `ListarRegras`, `CriarRegra`, `EditarRegra`, `ListarCondicoes`, `CriarCondicao`, `EditarCondicao`
  - Validar campos obrigatórios antes de persistir (retornar erro de validação)
  - **Requisito:** 7.1–7.8

## Fase 7: Handlers HTTP (API REST)

- [x] 7.1 Implementar handler de movimento contábil
  - Criar `internal/handler/movimento_contabil_handler.go`
  - `POST /api/v1/movimento-contabil` — decodifica payload `{data}`, chama serviço, retorna 200 ou erro
  - `POST /api/v1/estorno` — decodifica payload `{data}`, chama serviço, retorna 200 ou erro
  - `GET /api/v1/movimento-contabil` — lê query params `data`, `pagina`, `tamanho`, retorna JSON paginado
  - **Requisito:** 8.2, 8.4, 9.2, 9.3

- [x] 7.2 Implementar handler de regras contábeis
  - Criar `internal/handler/regra_contabil_handler.go`
  - Implementar endpoints: `GET /api/v1/regras`, `POST /api/v1/regras`, `PUT /api/v1/regras/{id}`, `GET /api/v1/regras/{id}/condicoes`, `POST /api/v1/regras/{id}/condicoes`, `PUT /api/v1/condicoes/{id}`
  - **Requisito:** 7.1–7.6

- [x] 7.3 Implementar roteador e ponto de entrada da API
  - Criar `cmd/api/main.go` com inicialização do banco, injeção de dependências e registro de rotas
  - **Requisito:** 11.1, 11.2

## Fase 8: Frontend

- [x] 8.1 Implementar templates HTML para operação (processamento e estorno)
  - Criar `cmd/frontend/templates/operacao.html` com formulário de data + botão de processamento e formulário de data + botão de estorno
  - Exibir mensagem de confirmação ou erro retornado pela API
  - **Requisito:** 8.1–8.6

- [x] 8.2 Implementar templates HTML para consulta de movimento contábil
  - Criar `cmd/frontend/templates/consulta.html` com campo de data, tabela de lançamentos e controles de paginação
  - **Requisito:** 9.1, 9.4

- [~] 8.3 Implementar templates HTML para cadastro de regras e condições
  - Criar `cmd/frontend/templates/regras.html` com listagem, formulário de criação/edição de regras e condições
  - Implementar validação de campos obrigatórios no frontend (HTML5 `required` + mensagem de erro)
  - **Requisito:** 7.1–7.8

- [~] 8.4 Implementar handlers e ponto de entrada do frontend
  - Criar `cmd/frontend/main.go` com servidor HTTP que serve os templates e faz proxy das chamadas à API
  - **Requisito:** 8.1–8.6

## Fase 9: Testes Unitários e de Integração

- [~] 9.1 Escrever testes unitários para as regras NDF (Requisito 4)
  - Testar cada uma das 4 combinações (Nassau + afiliada/não-afiliada + MTM positivo/negativo) com valores concretos
  - Verificar contas, valores e moedas esperados para cada caso
  - **Requisito:** 4.1–4.4

- [~] 9.2 Escrever testes unitários para casos de borda
  - Posição vazia para a data informada → sem lançamentos, resposta de ausência
  - Lote D-1 inexistente → sem estornos, resposta de ausência
  - Expressão booleana inválida → log de erro, demais registros processados
  - Expressão de valor inválida → log de erro, demais registros processados
  - **Requisito:** 1.3, 2.4, 2.5, 5.8

- [~] 9.3 Escrever testes de integração para o fluxo completo
  - Testar fluxo: inserir posição → gerar movimento → gerar estorno → consultar lote consolidado
  - Verificar que nova regra cadastrada é aplicada no próximo processamento sem redeploy
  - **Requisito:** 2.3, 6.1, 6.2
