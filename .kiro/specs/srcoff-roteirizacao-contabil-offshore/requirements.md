# Documento de Requisitos

## Introdução

O Sistema de Roteirização Contábil Offshore (SRCOff) tem como objetivo gerar o movimento contábil diário com base na posição de carteira dos sistemas offshore da Tesouraria. O sistema processa em lote os registros de posição de carteira de uma determinada data, aplica regras contábeis parametrizadas dinamicamente e persiste os lançamentos contábeis resultantes. Além disso, o sistema é responsável por estornar o lote contábil do dia anterior (D-1) quando os valores divergem, e por consolidar o lote contábil final. O sistema expõe uma API REST e um frontend em Golang para operação e consulta dos movimentos.

---

## Glossário

- **SRCOff**: Sistema de Roteirização Contábil Offshore — sistema objeto desta especificação.
- **Posicao_Carteira**: Conjunto de registros diários que representam a posição de carteira offshore da Tesouraria para uma determinada data.
- **Regra_Contabil**: Entidade parametrizável que define um conjunto de condições e os respectivos lançamentos contábeis a serem gerados quando as condições são satisfeitas.
- **Condicao_Regra**: Entidade vinculada a uma Regra_Contabil que define a expressão booleana de filtragem, as contas de débito/crédito, o campo de valor e o campo de moeda do lançamento.
- **Lancamento_Contabil**: Registro gerado pelo SRCOff representando um movimento contábil para um registro de posição que satisfez uma condição de regra.
- **Lote_Contabil**: Conjunto de todos os Lancamentos_Contabeis gerados para uma determinada data.
- **Estorno**: Lançamento contábil gerado para reverter um Lancamento_Contabil de D-1, com as contas de débito e crédito invertidas e o indicador_reversao igual a verdadeiro.
- **D-1**: Data imediatamente anterior à data do Lote_Contabil em processamento.
- **Avaliador_Expressao**: Componente responsável por avaliar dinamicamente expressões booleanas e fórmulas de valor sobre os campos da Posicao_Carteira, sem necessidade de código hardcoded.
- **codigo_versao_conteudo**: Número inteiro que representa a versão do conteúdo de um conjunto de registros para uma mesma data, utilizado tanto na Posicao_Carteira quanto no movimento_contabil.
- **codigo_identificador_boleto**: Identificador único de uma operação dentro da Posicao_Carteira.
- **MTM**: Mark-to-Market — valor de mercado da operação presente na Posicao_Carteira.
- **Principal_Remanescente**: Valor do principal remanescente da operação presente na Posicao_Carteira.
- **Frontend**: Interface web desenvolvida em Golang para operação e consulta do SRCOff.
- **API**: Interface de programação REST desenvolvida em Golang que executa toda a lógica de negócio do SRCOff.
- **Banco_de_Dados**: Instância do Microsoft SQL Server Express acessada via Trusted Connection no servidor DESKTOP-BBARIOTTI.

---

## Requisitos

### Requisito 1: Carregamento da Posição de Carteira

**User Story:** Como operador da Tesouraria, quero que o SRCOff carregue a posição de carteira de uma data específica, para que somente os dados mais atualizados sejam utilizados no processamento contábil.

#### Critérios de Aceitação

1. WHEN o endpoint de geração de movimento contábil é acionado com uma data, THE API SHALL consultar a tabela posicao_carteira filtrando os registros pelo campo data_posicao_carteira igual à data recebida no payload.
2. WHEN existem múltiplas versões de posição para a mesma data, THE API SHALL selecionar exclusivamente os registros com o maior valor de codigo_versao_conteudo disponível para aquela data.
3. IF nenhum registro de posicao_carteira for encontrado para a data informada, THEN THE API SHALL retornar uma resposta indicando ausência de dados e encerrar o processamento sem gerar lançamentos.

---

### Requisito 2: Avaliação Dinâmica de Regras Contábeis

**User Story:** Como analista contábil, quero que as condições e fórmulas das regras contábeis sejam avaliadas dinamicamente, para que novas regras possam ser parametrizadas sem alteração de código.

#### Critérios de Aceitação

1. THE Avaliador_Expressao SHALL avaliar a expressão booleana presente no campo condicao da tabela condicao_regra utilizando os valores dos campos da Posicao_Carteira como variáveis de entrada.
2. THE Avaliador_Expressao SHALL avaliar a expressão de valor presente no campo campo_valor da tabela condicao_regra utilizando os valores dos campos da Posicao_Carteira como variáveis de entrada, retornando um valor numérico decimal.
3. WHEN uma nova Regra_Contabil ou Condicao_Regra é cadastrada no banco de dados, THE API SHALL aplicá-la no próximo processamento sem necessidade de recompilação ou redeploy.
4. IF a expressão presente no campo condicao não puder ser avaliada para um registro de posição, THEN THE Avaliador_Expressao SHALL registrar o erro no log e prosseguir para o próximo registro sem interromper o processamento do lote.
5. IF a expressão presente no campo campo_valor não puder ser avaliada para um registro de posição, THEN THE Avaliador_Expressao SHALL registrar o erro no log e prosseguir para o próximo registro sem interromper o processamento do lote.

---

### Requisito 3: Geração do Movimento Contábil

**User Story:** Como operador da Tesouraria, quero que o SRCOff gere os lançamentos contábeis para cada registro de posição que satisfaça as condições das regras, para que o movimento contábil diário seja produzido corretamente.

#### Critérios de Aceitação

1. WHEN o Avaliador_Expressao retorna verdadeiro para uma condicao_regra aplicada a um registro de posicao_carteira, THE API SHALL gerar um Lancamento_Contabil e persistir na tabela movimento_contabil.
2. THE API SHALL preencher o campo data_lote_contabil do Lancamento_Contabil com a data recebida como parâmetro no payload.
3. THE API SHALL preencher o campo codigo_identificador_boleto do Lancamento_Contabil com o valor do campo codigo_identificador_boleto do registro de posicao_carteira correspondente.
4. THE API SHALL preencher o campo valor_lancamento_contabil do Lancamento_Contabil com o resultado da avaliação da expressão definida no campo campo_valor da condicao_regra sobre os campos do registro de posicao_carteira.
5. THE API SHALL preencher o campo moeda_lancamento_contabil do Lancamento_Contabil com o valor do campo da posicao_carteira referenciado pelo campo campo_moeda da condicao_regra.
6. THE API SHALL preencher o campo conta_debito do Lancamento_Contabil com o valor do campo conta_debito da condicao_regra correspondente.
7. THE API SHALL preencher o campo conta_credito do Lancamento_Contabil com o valor do campo conta_credito da condicao_regra correspondente.
8. THE API SHALL preencher o campo indicador_reversao do Lancamento_Contabil com o valor falso para lançamentos de movimento contábil.
9. THE API SHALL preencher o campo descricao_regra_contabil do Lancamento_Contabil com o valor do campo descricao da tabela regra correspondente.
10. THE API SHALL preencher o campo descricao_condicao_contabil do Lancamento_Contabil com o valor do campo condicao da condicao_regra correspondente.
11. THE API SHALL preencher o campo codigo_versao_conteudo do Lancamento_Contabil com o valor igual ao maior codigo_versao_conteudo existente na tabela movimento_contabil para a mesma data_lote_contabil acrescido de 1, ou 1 caso não exista nenhum registro para aquela data.
12. WHEN o processamento do lote é concluído, THE API SHALL ter persistido todos os Lancamentos_Contabeis gerados para a data informada, compondo o Lote_Contabil daquela data.

---

### Requisito 4: Regras Contábeis Iniciais (NDF - Registro de Notional - Nassau)

**User Story:** Como analista contábil, quero que as regras contábeis iniciais do produto NDF sejam aplicadas corretamente, para que os lançamentos de Notional de Nassau sejam gerados conforme as condições de afiliação e MTM.

#### Critérios de Aceitação

1. WHEN descricao_veiculo é igual a "NASSAU" e indicador_contraparte_afiliada é verdadeiro e valor_mtm é maior que zero, THE API SHALL gerar um Lancamento_Contabil com conta_debito "111111111", conta_credito "222222222", valor igual a principal_remanescente somado a valor_mtm e moeda igual a moeda_principal_remanescente.
2. WHEN descricao_veiculo é igual a "NASSAU" e indicador_contraparte_afiliada é verdadeiro e valor_mtm é menor que zero, THE API SHALL gerar um Lancamento_Contabil com conta_debito "333333333", conta_credito "444444444", valor igual a principal_remanescente e moeda igual a moeda_principal_remanescente.
3. WHEN descricao_veiculo é igual a "NASSAU" e indicador_contraparte_afiliada é falso e valor_mtm é maior que zero, THE API SHALL gerar um Lancamento_Contabil com conta_debito "555555555", conta_credito "666666666", valor igual a principal_remanescente somado a valor_mtm e moeda igual a moeda_principal_remanescente.
4. WHEN descricao_veiculo é igual a "NASSAU" e indicador_contraparte_afiliada é falso e valor_mtm é menor que zero, THE API SHALL gerar um Lancamento_Contabil com conta_debito "777777777", conta_credito "888888888", valor igual a principal_remanescente e moeda igual a moeda_principal_remanescente.
5. WHEN um registro de posicao_carteira não satisfaz nenhuma condicao_regra de nenhuma Regra_Contabil, THE API SHALL ignorar o registro sem gerar Lancamento_Contabil e sem registrar erro.

---

### Requisito 5: Estorno do Lote Contábil de D-1

**User Story:** Como operador da Tesouraria, quero que o SRCOff estorne automaticamente os lançamentos de D-1 que divergem do lote atual, para que o movimento contábil reflita apenas os valores corretos da data em processamento.

#### Critérios de Aceitação

1. WHEN o endpoint de estorno é acionado com uma data D, THE API SHALL recuperar todos os Lancamentos_Contabeis do Lote_Contabil de D-1 (data igual a D menos um dia útil) da tabela movimento_contabil onde indicador_reversao é falso.
2. WHEN o endpoint de estorno é acionado com uma data D, THE API SHALL recuperar todos os Lancamentos_Contabeis do Lote_Contabil de D da tabela movimento_contabil onde indicador_reversao é falso.
3. THE API SHALL comparar os lançamentos de D-1 com os lançamentos de D utilizando como chave de comparação o par (codigo_identificador_boleto, id_regra_contabil).
4. WHEN um Lancamento_Contabil de D-1 possui valor_lancamento_contabil diferente do Lancamento_Contabil correspondente em D para a mesma chave, THE API SHALL gerar um Estorno com a data_lote_contabil igual a D, o valor_lancamento_contabil igual ao valor do lançamento de D-1, conta_debito igual à conta_credito do lançamento de D-1 e conta_credito igual à conta_debito do lançamento de D-1.
5. THE API SHALL preencher o campo indicador_reversao do Estorno com o valor verdadeiro.
6. WHEN um Lancamento_Contabil de D-1 não possui correspondente no Lote_Contabil de D para a mesma chave, THE API SHALL gerar um Estorno para aquele lançamento de D-1.
7. WHEN um Lancamento_Contabil de D-1 possui valor_lancamento_contabil igual ao Lancamento_Contabil correspondente em D para a mesma chave, THE API SHALL não gerar Estorno para aquele lançamento.
8. IF nenhum Lote_Contabil existir para D-1, THEN THE API SHALL retornar uma resposta indicando ausência de lote em D-1 e encerrar o processamento de estorno sem gerar lançamentos.

---

### Requisito 6: Consolidação do Lote Contábil

**User Story:** Como operador da Tesouraria, quero que o lote contábil seja consolidado após a geração do movimento e do estorno, para que o resultado final esteja disponível para consulta e exportação.

#### Critérios de Aceitação

1. WHEN o movimento contábil e o estorno de D-1 são gerados com sucesso para uma data D, THE API SHALL consolidar o Lote_Contabil de D, tornando-o disponível para consulta.
2. THE API SHALL garantir que o Lote_Contabil consolidado contenha todos os Lancamentos_Contabeis com indicador_reversao igual a falso e todos os Estornos com indicador_reversao igual a verdadeiro gerados para a data D.

---

### Requisito 7: Cadastro de Regras e Condições Contábeis

**User Story:** Como analista contábil, quero cadastrar, editar e consultar regras e condições contábeis pelo frontend, para que o sistema possa ser parametrizado sem intervenção técnica.

#### Critérios de Aceitação

1. THE Frontend SHALL permitir ao usuário criar uma nova Regra_Contabil informando os campos descricao e codigo_produto_corporativo.
2. THE Frontend SHALL permitir ao usuário criar uma nova Condicao_Regra vinculada a uma Regra_Contabil existente, informando os campos condicao, conta_debito, conta_credito, campo_valor e campo_moeda.
3. THE Frontend SHALL permitir ao usuário consultar a lista de Regras_Contabeis cadastradas.
4. THE Frontend SHALL permitir ao usuário consultar as Condicoes_Regra vinculadas a uma Regra_Contabil selecionada.
5. THE Frontend SHALL permitir ao usuário editar os campos de uma Regra_Contabil existente.
6. THE Frontend SHALL permitir ao usuário editar os campos de uma Condicao_Regra existente.
7. IF o usuário tentar salvar uma Regra_Contabil sem preencher o campo descricao, THEN THE Frontend SHALL exibir uma mensagem de validação e impedir o envio do formulário.
8. IF o usuário tentar salvar uma Condicao_Regra sem preencher todos os campos obrigatórios (condicao, conta_debito, conta_credito, campo_valor, campo_moeda), THEN THE Frontend SHALL exibir uma mensagem de validação e impedir o envio do formulário.

---

### Requisito 8: Acionamento do Processamento pelo Frontend

**User Story:** Como operador da Tesouraria, quero acionar o processamento do movimento contábil e do estorno diretamente pelo frontend informando uma data, para que eu possa controlar quando cada etapa é executada.

#### Critérios de Aceitação

1. THE Frontend SHALL exibir um botão para iniciar o processamento do movimento contábil, com um campo de entrada para a data do processamento.
2. WHEN o usuário aciona o botão de processamento do movimento contábil, THE Frontend SHALL enviar uma requisição ao endpoint correspondente da API com a data informada.
3. THE Frontend SHALL exibir um botão para iniciar o estorno do movimento contábil, com um campo de entrada para a data do estorno.
4. WHEN o usuário aciona o botão de estorno, THE Frontend SHALL enviar uma requisição ao endpoint correspondente da API com a data informada.
5. WHEN a API retorna uma resposta de sucesso para o processamento ou estorno, THE Frontend SHALL exibir uma mensagem de confirmação ao usuário.
6. IF a API retornar uma resposta de erro para o processamento ou estorno, THEN THE Frontend SHALL exibir a mensagem de erro retornada pela API ao usuário.

---

### Requisito 9: Consulta do Movimento Contábil

**User Story:** Como operador da Tesouraria, quero consultar o movimento contábil de um determinado dia com paginação, para que eu possa visualizar e auditar os lançamentos gerados.

#### Critérios de Aceitação

1. THE Frontend SHALL permitir ao usuário informar uma data e consultar todos os Lancamentos_Contabeis do Lote_Contabil correspondente.
2. THE API SHALL retornar os Lancamentos_Contabeis de uma data em páginas, com o número de registros por página e o número da página definidos como parâmetros da requisição.
3. THE API SHALL retornar o total de registros disponíveis para a data consultada junto com cada página de resultados.
4. THE Frontend SHALL exibir os controles de navegação entre páginas com base no total de registros e no tamanho da página retornados pela API.

---

### Requisito 10: Desempenho do Processamento em Lote

**User Story:** Como operador da Tesouraria, quero que o processamento do lote contábil seja concluído em tempo hábil, para que o movimento contábil esteja disponível dentro da janela operacional diária.

#### Critérios de Aceitação

1. WHEN o endpoint de geração de movimento contábil é acionado com uma posição contendo até 50.000 registros, THE API SHALL concluir todo o processamento e persistência dos lançamentos em até 10 minutos.
2. THE API SHALL processar os registros da posicao_carteira em lote, sem processamento registro a registro com chamadas individuais ao banco de dados para cada lançamento.

---

### Requisito 11: Conectividade com o Banco de Dados

**User Story:** Como administrador do sistema, quero que o SRCOff se conecte ao banco de dados utilizando autenticação integrada, para que não seja necessário gerenciar credenciais de banco de dados na aplicação.

#### Critérios de Aceitação

1. THE API SHALL conectar-se ao Microsoft SQL Server Express no servidor DESKTOP-BBARIOTTI utilizando Trusted Connection (autenticação integrada do Windows).
2. IF a conexão com o Banco_de_Dados não puder ser estabelecida na inicialização da API, THEN THE API SHALL registrar o erro no log e encerrar a inicialização com código de saída diferente de zero.

---

### Requisito 12: Consulta de Movimento Contábil por Período e Boleto

**User Story:** Como operador da Tesouraria, quero consultar o movimento contábil filtrando por período de datas e/ou número do boleto, para que eu possa localizar lançamentos específicos com mais flexibilidade.

#### Critérios de Aceitação

1. THE Frontend SHALL permitir ao usuário informar uma data de início, uma data de fim e/ou um número de boleto (parcial ou completo) para filtrar os lançamentos.
2. THE API SHALL aceitar os parâmetros `data_inicio`, `data_fim` e `boleto` no endpoint de consulta de movimento contábil.
3. WHEN apenas `boleto` é informado sem datas, THE API SHALL retornar todos os lançamentos que contenham o valor informado no campo `codigo_identificador_boleto`.
4. THE API SHALL suportar busca parcial por boleto utilizando correspondência por substring.

---

### Requisito 13: Filtro de Versão na Consulta de Movimento Contábil

**User Story:** Como operador da Tesouraria, quero filtrar os lançamentos por versão do lote contábil, para que eu possa auditar versões específicas ou visualizar apenas a versão vigente.

#### Critérios de Aceitação

1. THE Frontend SHALL exibir um seletor de versão com as opções: Vigente (maior versão por data), Todas as versões, e Versão específica.
2. WHEN o usuário seleciona "Vigente", THE API SHALL retornar apenas os lançamentos cuja `codigo_versao_conteudo` é igual ao maior valor disponível para cada data no período consultado.
3. WHEN o usuário seleciona "Todas", THE API SHALL retornar lançamentos de todas as versões sem filtro adicional.
4. WHEN o usuário seleciona "Específica", THE Frontend SHALL exibir um campo numérico para o usuário informar o número da versão desejada.
5. THE Frontend SHALL exibir a coluna `codigo_versao_conteudo` no grid de resultados.
6. O filtro padrão SHALL ser "Vigente".

---

### Requisito 14: Configuração do Servidor de Banco de Dados via Variável de Ambiente

**User Story:** Como administrador do sistema, quero configurar o servidor de banco de dados via variável de ambiente, para que a aplicação possa ser executada em diferentes ambientes sem recompilação.

#### Critérios de Aceitação

1. THE API SHALL ler o servidor de banco de dados a partir da variável de ambiente `DB_SERVER`.
2. IF `DB_SERVER` não estiver definida, THE API SHALL utilizar o valor padrão `DESKTOP-B1QQIIN\SQLEXPRESS`.
3. THE API SHALL ler o nome do banco de dados a partir da variável de ambiente `DB_NAME` com padrão `srcoff`.

---

### Requisito 15: Conciliação entre Posição de Carteira e Movimento Contábil

**User Story:** Como operador da Tesouraria, quero conciliar a posição de carteira com o movimento contábil de uma data específica, para que eu possa identificar inconsistências antes do fechamento contábil.

#### Critérios de Aceitação

1. THE Frontend SHALL disponibilizar uma página de conciliação onde o usuário informa uma data e aciona a verificação.
2. WHEN acionada, THE API SHALL comparar os registros da posicao_carteira (versão máxima) com os lançamentos do movimento_contabil (versão vigente) para a data informada.
3. IF um registro de posicao_carteira não possuir nenhum lançamento correspondente no movimento_contabil para o mesmo `codigo_identificador_boleto` e data, THEN THE API SHALL reportar uma inconsistência do tipo `POSICAO_SEM_MOVIMENTO`.
4. IF existir mais de um lançamento no movimento_contabil para o mesmo `codigo_identificador_boleto`, `descricao_regra_contabil` e `indicador_reversao` na mesma data, THEN THE API SHALL reportar uma inconsistência do tipo `LANCAMENTO_DUPLICADO`.
5. THE API SHALL retornar o resultado da conciliação sem persistir as inconsistências no banco de dados.
6. THE Frontend SHALL exibir as inconsistências em um grid com tipo, boleto, regra, indicador de reversão e detalhe.
7. WHEN nenhuma inconsistência for encontrada, THE Frontend SHALL exibir uma mensagem de confirmação de conciliação bem-sucedida.

---

### Requisito 16: Backend de Persistência Configurável

**User Story:** Como administrador do sistema, quero poder escolher entre persistência em banco de dados SQL Server ou em arquivos JSON, para que o sistema possa ser executado em ambientes sem SQL Server disponível.

#### Critérios de Aceitação

1. THE API SHALL suportar dois backends de persistência: `sqlserver` e `file`.
2. THE API SHALL ler o backend ativo a partir da variável de ambiente `STORAGE_BACKEND`. Se não definida, o padrão SHALL ser `sqlserver`.
3. WHEN `STORAGE_BACKEND=file`, THE API SHALL persistir todos os dados em arquivos JSON no diretório configurado pela variável de ambiente `FILE_STORAGE_DIR` (padrão: `./data`).
4. WHEN `STORAGE_BACKEND=sqlserver`, THE API SHALL utilizar o Microsoft SQL Server conforme configuração existente.
5. A troca de backend SHALL ser feita exclusivamente via variável de ambiente, sem necessidade de recompilação.
6. Ambos os backends SHALL implementar as mesmas interfaces de repositório, garantindo comportamento equivalente para todas as operações.
7. WHEN `STORAGE_BACKEND=file`, os dados SHALL ser armazenados em três arquivos: `posicao_carteira.json`, `regras.json` e `movimento_contabil.json`.

---

### Requisito 17: Carregamento Dinâmico de Campos da Posição de Carteira

**User Story:** Como analista contábil, quero que novas colunas adicionadas à tabela posicao_carteira sejam automaticamente disponibilizadas nas expressões das regras contábeis, sem necessidade de alteração de código.

#### Critérios de Aceitação

1. THE API SHALL carregar todos os campos da tabela posicao_carteira usando `SELECT *`, sem listar colunas explicitamente.
2. THE API SHALL construir o ambiente de avaliação de expressões dinamicamente a partir dos tipos de coluna retornados pelo banco de dados, usando `ColumnTypes()`.
3. WHEN uma coluna da tabela posicao_carteira possui valor NULL, THE Avaliador_Expressao SHALL substituir o valor por zero-value do tipo correspondente (0 para numéricos, "" para strings, false para booleanos) antes de avaliar a expressão.
4. THE Avaliador_Expressao SHALL compilar expressões sem type-checking estático, permitindo que qualquer campo presente no ambiente seja referenciado nas expressões sem recompilação.
5. WHEN uma nova coluna é adicionada à tabela posicao_carteira, THE API SHALL disponibilizá-la automaticamente nas expressões das regras contábeis sem necessidade de alteração de código ou redeploy.

---

### Requisito 18: Manutenção da Posição de Carteira pelo Frontend

**User Story:** Como operador da Tesouraria, quero inserir e excluir registros da posição de carteira diretamente pelo frontend, para que eu possa gerenciar os dados de posição sem acesso direto ao banco de dados.

#### Critérios de Aceitação

1. THE Frontend SHALL disponibilizar uma página de manutenção de posição onde o usuário pode consultar registros por data.
2. THE Frontend SHALL permitir ao usuário inserir um novo registro de posição informando os campos: data, versão, boleto, veículo, indicador de contraparte afiliada, valor MTM, principal remanescente e moeda.
3. THE Frontend SHALL permitir ao usuário excluir um registro de posição pelo seu ID.
4. THE API SHALL expor os endpoints `GET /api/v1/posicao`, `POST /api/v1/posicao` e `DELETE /api/v1/posicao?id={id}`.
5. WHEN um registro é inserido sem `codigo_versao_conteudo`, THE API SHALL assumir versão 1 como padrão.
6. IF `codigo_identificador_boleto` ou `data_posicao_carteira` não forem informados, THEN THE API SHALL retornar erro de validação.

---

### Requisito 19: Exportação do Movimento Contábil em CSV

**User Story:** Como operador da Tesouraria, quero exportar os lançamentos do movimento contábil em formato CSV, para que eu possa importar os dados no Excel e realizar análises offline.

#### Critérios de Aceitação

1. THE API SHALL expor o endpoint `GET /api/v1/movimento-contabil/export` que retorna um arquivo CSV com os lançamentos filtrados.
2. THE API SHALL aceitar os mesmos parâmetros de filtro do endpoint de consulta: `data_inicio`, `data_fim`, `boleto`, `versao_modo` e `versao`.
3. THE API SHALL retornar o arquivo CSV com BOM UTF-8 (`EF BB BF`) para garantir a correta exibição de caracteres acentuados no Microsoft Excel.
4. THE API SHALL utilizar ponto-e-vírgula (`;`) como separador de campos no CSV, conforme padrão do Excel em localidades pt-BR.
5. THE CSV SHALL conter as colunas: Data Lote, Versão, Boleto, Conta Débito, Conta Crédito, Valor, Moeda, Reversão, Regra, Condição.
6. THE API SHALL nomear o arquivo retornado como `movimento_contabil_{data_inicio}_{data_fim}.csv`.
7. IF `data_inicio` não for informado, THE API SHALL assumir `2000-01-01` como padrão. IF `data_fim` não for informado, THE API SHALL assumir `2999-12-31` como padrão.

---

### Requisito 16: Backend de Persistência Configurável

**User Story:** Como administrador do sistema, quero poder escolher entre SQL Server e arquivos JSON como backend de persistência, para que o sistema possa ser executado sem dependência de banco de dados.

#### Critérios de Aceitação

1. THE API SHALL ler a variável de ambiente `STORAGE_BACKEND` para determinar o backend (`sqlserver` ou `file`).
2. WHEN `STORAGE_BACKEND=sqlserver` (padrão), THE API SHALL usar os repositórios SQL Server existentes.
3. WHEN `STORAGE_BACKEND=file`, THE API SHALL usar repositórios baseados em arquivos JSON no diretório `FILE_STORAGE_DIR` (padrão: `./data`).
4. Os arquivos JSON gerados são: `posicao_carteira.json`, `regras.json` e `movimento_contabil.json`.
5. Todas as funcionalidades do sistema devem operar identicamente em ambos os backends.
6. As interfaces de repositório devem ser definidas em `internal/repository/interfaces.go`.
7. O backend de arquivo deve implementar todas as operações de filtragem em memória.

---

### Requisito 17: Carregamento Dinâmico de Campos da Posição de Carteira

**User Story:** Como analista contábil, quero que novas colunas adicionadas à tabela `posicao_carteira` sejam automaticamente disponibilizadas nas expressões das regras contábeis, sem necessidade de alteração de código.

#### Critérios de Aceitação

1. THE API SHALL usar `SELECT *` ao consultar a tabela `posicao_carteira`, sem listar colunas explicitamente.
2. THE API SHALL usar `rows.ColumnTypes()` para determinar o tipo de cada coluna e alocar o tipo Go correto antes do scan.
3. THE API SHALL construir o mapa de variáveis do avaliador de expressões diretamente a partir das colunas retornadas pelo banco.
4. WHEN uma coluna numérica retorna `NULL`, THE API SHALL substituir por `0.0` para evitar erros de tipo nas expressões.
5. WHEN uma nova coluna é adicionada à tabela `posicao_carteira`, THE API SHALL disponibilizá-la automaticamente nas expressões das regras sem recompilação.

---

### Requisito 18: Manutenção da Posição de Carteira

**User Story:** Como operador da Tesouraria, quero inserir e excluir registros da posição de carteira pelo frontend, para que eu possa gerenciar os dados de posição sem acesso direto ao banco de dados.

#### Critérios de Aceitação

1. THE Frontend SHALL disponibilizar uma página `/posicao` com formulário de inserção e grid de consulta por data.
2. THE Frontend SHALL permitir consultar todos os registros de posição de uma data específica.
3. THE Frontend SHALL exibir todas as colunas da posição dinamicamente no grid, incluindo colunas adicionais.
4. THE API SHALL expor `POST /api/v1/posicao` para inserção de novos registros.
5. THE API SHALL expor `DELETE /api/v1/posicao?id={id}` para exclusão de registros por ID.
6. THE API SHALL expor `GET /api/v1/posicao?data={data}` para listagem de registros por data.
7. THE Frontend SHALL exibir link para a página de posição na navegação de todas as páginas.

---

### Requisito 19: Exportação CSV do Movimento Contábil

**User Story:** Como operador da Tesouraria, quero exportar o resultado da consulta de movimento contábil para um arquivo CSV compatível com Excel, para que eu possa analisar os dados em planilha.

#### Critérios de Aceitação

1. THE Frontend SHALL exibir um botão "⬇ Excel" ao lado do botão "Consultar" na página de consulta.
2. WHEN o usuário clica no botão, THE Frontend SHALL gerar um download com os mesmos filtros aplicados na consulta.
3. THE API SHALL expor `GET /api/v1/movimento-contabil/export` aceitando os mesmos parâmetros da consulta.
4. O arquivo gerado SHALL ter extensão `.csv`, separador `;` e BOM UTF-8 para compatibilidade com Excel.
5. O arquivo SHALL conter cabeçalho com colunas: Data Lote, Versão, Boleto, Conta Débito, Conta Crédito, Valor, Moeda, Reversão, Regra, Condição.
6. O arquivo SHALL aplicar o mesmo filtro de eliminação de lançamentos com saldo zero da consulta do frontend.
7. O nome do arquivo SHALL seguir o padrão `movimento_contabil_{data_inicio}_{data_fim}.csv`.

---

### Requisito 20: Exportação TXT do Movimento Contábil

**User Story:** Como operador da Tesouraria, quero exportar o movimento contábil de um dia específico em formato TXT estruturado, para integração com sistemas legados.

#### Critérios de Aceitação

1. THE Frontend SHALL disponibilizar uma seção de exportação TXT na página de consulta com campo de data única.
2. THE API SHALL expor `GET /api/v1/movimento-contabil/export-txt?data={data}`.
3. A primeira linha do arquivo SHALL ser o cabeçalho no formato `C;AAAAMMDD`.
4. A última linha SHALL ser o totalizador no formato `T;{soma_total}`.
5. Cada lançamento SHALL gerar duas linhas de detalhe: uma para conta débito e uma para conta crédito.
6. Cada linha de detalhe SHALL seguir o formato: `D;{conta};{D/C};{moeda};{regra};{boleto};{S/N};{valor}`.
7. IF não houver lançamentos para a data, THE API SHALL retornar mensagem indicando ausência de dados.
8. THE Frontend SHALL exibir mensagem de aviso quando não há dados para a data informada.

---

### Requisito 21: Exclusão de Movimento Contábil

**User Story:** Como operador da Tesouraria, quero excluir lançamentos contábeis de uma data e/ou versão específica, para corrigir processamentos incorretos.

#### Critérios de Aceitação

1. THE Frontend SHALL disponibilizar uma seção de exclusão na página de consulta com campos de data e versão opcional.
2. WHEN versão não é informada, THE API SHALL excluir todos os lançamentos da data.
3. WHEN versão é informada, THE API SHALL excluir apenas os lançamentos da data e versão especificadas.
4. THE API SHALL expor `DELETE /api/v1/movimento-contabil?data={data}&versao={versao}`.

---

### Requisito 22: Filtro de Lançamentos com Saldo Zero na Consulta

**User Story:** Como operador da Tesouraria, quero que a consulta de movimento contábil exiba apenas lançamentos com saldo líquido diferente de zero, para que lançamentos já estornados não apareçam na visualização.

#### Critérios de Aceitação

1. WHEN consultando lançamentos pelo frontend, THE API SHALL eliminar grupos de lançamentos cujo saldo líquido (soma de normais menos soma de reversões) é zero para o mesmo boleto, valor e regra.
2. Este filtro SHALL ser aplicado apenas na consulta do frontend e na exportação CSV/TXT.
3. Funcionalidades internas como estorno e conciliação SHALL usar a consulta sem este filtro.
