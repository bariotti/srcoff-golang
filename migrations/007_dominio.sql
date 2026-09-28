-- migrations/007_dominio.sql
-- Adiciona a dimensão Domínio (paralela ao Produto) à regra contábil, ao padrão de
-- arquivo e ao movimento contábil. Cria o log de execução e as notificações. Idempotente.

-- Domínio na regra contábil (lista separada por vírgula, como o produto).
IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('regra_contabil') AND name = 'dominio')
    ALTER TABLE regra_contabil ADD dominio VARCHAR(255) NULL;

-- Domínio no padrão de arquivo.
IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('padrao_arquivo') AND name = 'dominio')
    ALTER TABLE padrao_arquivo ADD dominio VARCHAR(50) NULL;

-- Domínio na inconsistência de processamento.
IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('inconsistencia_processamento') AND name = 'dominio')
    ALTER TABLE inconsistencia_processamento ADD dominio VARCHAR(50) NULL;

-- Produto e Domínio no movimento contábil (gravados em cada lançamento).
IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('movimento_contabil') AND name = 'produto')
    ALTER TABLE movimento_contabil ADD produto VARCHAR(50) NULL;
IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('movimento_contabil') AND name = 'dominio')
    ALTER TABLE movimento_contabil ADD dominio VARCHAR(50) NULL;

-- Log de execução do contábil por (data, produto, domínio) — base de processados/pendentes.
IF OBJECT_ID('movimento_execucao', 'U') IS NULL
BEGIN
    CREATE TABLE movimento_execucao (
        id               BIGINT IDENTITY PRIMARY KEY,
        data_lote        DATE NOT NULL,
        produto          VARCHAR(50) NOT NULL,
        dominio          VARCHAR(50) NOT NULL,
        qtd_lancamentos  INT NOT NULL DEFAULT 0,
        qtd_estornos     INT NOT NULL DEFAULT 0,
        criado_em        DATETIME2 NOT NULL DEFAULT SYSDATETIME()
    );
    CREATE INDEX IX_execucao_data ON movimento_execucao (data_lote);
END;

-- Notificações de eventos automáticos (importação/execução).
IF OBJECT_ID('notificacao', 'U') IS NULL
BEGIN
    CREATE TABLE notificacao (
        id         BIGINT IDENTITY PRIMARY KEY,
        tipo       VARCHAR(30) NOT NULL,
        data_lote  DATE NULL,
        produto    VARCHAR(50) NULL,
        dominio    VARCHAR(50) NULL,
        mensagem   VARCHAR(500) NOT NULL,
        lida       BIT NOT NULL DEFAULT 0,
        criado_em  DATETIME2 NOT NULL DEFAULT SYSDATETIME()
    );
END;
