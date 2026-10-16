-- migrations/016_saldo_conta.sql
-- Projeção materializada de saldo por chave (produto, domínio, boleto, id_regra, contas).
-- Cache derivado de movimento_contabil (RN-170 / ADR-010). Reconstruível a partir do movimento.
-- Idempotente.

IF OBJECT_ID('saldo_conta', 'U') IS NULL
BEGIN
    CREATE TABLE saldo_conta (
        produto                     VARCHAR(255) NOT NULL,
        dominio                     VARCHAR(255) NOT NULL,
        codigo_identificador_boleto VARCHAR(255) NOT NULL,
        id_regra_contabil           BIGINT       NOT NULL,
        conta_debito                VARCHAR(255) NOT NULL,
        conta_credito               VARCHAR(255) NOT NULL,
        saldo                        FLOAT        NOT NULL DEFAULT 0,
        moeda_lancamento            VARCHAR(50)  NULL,
        descricao_regra             VARCHAR(255) NULL,
        descricao_condicao          VARCHAR(500) NULL,
        CONSTRAINT PK_saldo_conta PRIMARY KEY (produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito)
    );
    CREATE INDEX IX_saldo_combo ON saldo_conta (produto, dominio);
END;
