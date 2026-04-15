-- migrations/001_create_tables.sql
-- DDL idempotente para criação das tabelas do SRCOff
-- Respeita dependências de chave estrangeira: regra_contabil → condicao_regra → movimento_contabil

-- ============================================================
-- Tabela: regra_contabil
-- ============================================================
IF OBJECT_ID('regra_contabil', 'U') IS NULL
BEGIN
    CREATE TABLE regra_contabil (
        id                          BIGINT IDENTITY PRIMARY KEY,
        descricao                   VARCHAR(255) NOT NULL,
        codigo_produto_corporativo  VARCHAR(50) NOT NULL,
        ativo                       BIT NOT NULL DEFAULT 1
    );
END;

-- ============================================================
-- Tabela: condicao_regra
-- ============================================================
IF OBJECT_ID('condicao_regra', 'U') IS NULL
BEGIN
    CREATE TABLE condicao_regra (
        id              BIGINT IDENTITY PRIMARY KEY,
        id_regra        BIGINT NOT NULL REFERENCES regra_contabil(id),
        condicao        VARCHAR(1000) NOT NULL,
        conta_debito    VARCHAR(20) NOT NULL,
        conta_credito   VARCHAR(20) NOT NULL,
        campo_valor     VARCHAR(500) NOT NULL,
        campo_moeda     VARCHAR(100) NOT NULL,
        ativo           BIT NOT NULL DEFAULT 1
    );
END;

-- ============================================================
-- Tabela: posicao_carteira
-- ============================================================
IF OBJECT_ID('posicao_carteira', 'U') IS NULL
BEGIN
    CREATE TABLE posicao_carteira (
        id                              BIGINT IDENTITY PRIMARY KEY,
        data_posicao_carteira           DATE NOT NULL,
        codigo_versao_conteudo          INT NOT NULL,
        codigo_identificador_boleto     VARCHAR(50) NOT NULL,
        descricao_veiculo               VARCHAR(100),
        indicador_contraparte_afiliada  BIT,
        valor_mtm                       DECIMAL(18,6),
        principal_remanescente          DECIMAL(18,6),
        moeda_principal_remanescente    VARCHAR(10)
    );
END;

IF NOT EXISTS (
    SELECT 1 FROM sys.indexes
    WHERE name = 'IX_posicao_data_versao'
      AND object_id = OBJECT_ID('posicao_carteira')
)
BEGIN
    CREATE INDEX IX_posicao_data_versao ON posicao_carteira (data_posicao_carteira, codigo_versao_conteudo);
END;

-- ============================================================
-- Tabela: movimento_contabil
-- ============================================================
IF OBJECT_ID('movimento_contabil', 'U') IS NULL
BEGIN
    CREATE TABLE movimento_contabil (
        id                          BIGINT IDENTITY PRIMARY KEY,
        data_lote_contabil          DATE NOT NULL,
        codigo_versao_conteudo      INT NOT NULL,
        codigo_identificador_boleto VARCHAR(50) NOT NULL,
        valor_lancamento_contabil   DECIMAL(18,6) NOT NULL,
        moeda_lancamento_contabil   VARCHAR(10) NOT NULL,
        conta_debito                VARCHAR(20) NOT NULL,
        conta_credito               VARCHAR(20) NOT NULL,
        indicador_reversao          BIT NOT NULL DEFAULT 0,
        descricao_regra_contabil    VARCHAR(255),
        descricao_condicao_contabil VARCHAR(1000),
        id_regra_contabil           BIGINT REFERENCES regra_contabil(id)
    );
END;

IF NOT EXISTS (
    SELECT 1 FROM sys.indexes
    WHERE name = 'IX_movimento_data_versao'
      AND object_id = OBJECT_ID('movimento_contabil')
)
BEGIN
    CREATE INDEX IX_movimento_data_versao ON movimento_contabil (data_lote_contabil, codigo_versao_conteudo);
END;

IF NOT EXISTS (
    SELECT 1 FROM sys.indexes
    WHERE name = 'IX_movimento_data_reversao'
      AND object_id = OBJECT_ID('movimento_contabil')
)
BEGIN
    CREATE INDEX IX_movimento_data_reversao ON movimento_contabil (data_lote_contabil, indicador_reversao);
END;

-- Adicionar coluna produto na posicao_carteira (idempotente)
IF NOT EXISTS (
    SELECT 1 FROM sys.columns
    WHERE object_id = OBJECT_ID('posicao_carteira') AND name = 'produto'
)
BEGIN
    ALTER TABLE posicao_carteira ADD produto VARCHAR(50) NULL;
END;
