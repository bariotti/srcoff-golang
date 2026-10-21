-- migrations/016_saldo_conta.sql
-- Projeção materializada de saldo por chave (produto, domínio, boleto, id_regra, contas).
-- Cache derivado de movimento_contabil (RN-170 / ADR-010). Reconstruível a partir do movimento.
-- Idempotente.

IF OBJECT_ID('saldo_conta', 'U') IS NULL
BEGIN
    -- Tamanhos das colunas da chave mantidos pequenos de propósito: a PK composta
    -- precisa caber no limite de 900 bytes do índice clusterizado do SQL Server.
    CREATE TABLE saldo_conta (
        produto                     VARCHAR(100) NOT NULL,
        dominio                     VARCHAR(100) NOT NULL,
        codigo_identificador_boleto VARCHAR(100) NOT NULL,
        id_regra_contabil           BIGINT       NOT NULL,
        conta_debito                VARCHAR(50)  NOT NULL,
        conta_credito               VARCHAR(50)  NOT NULL,
        saldo                        FLOAT        NOT NULL DEFAULT 0,
        moeda_lancamento            VARCHAR(50)  NULL,
        descricao_regra             VARCHAR(255) NULL,
        descricao_condicao          VARCHAR(500) NULL,
        CONSTRAINT PK_saldo_conta PRIMARY KEY (produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito)
    );
    CREATE INDEX IX_saldo_combo ON saldo_conta (produto, dominio);
END;
