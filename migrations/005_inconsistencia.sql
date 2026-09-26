-- migrations/005_inconsistencia.sql
-- Inconsistências detectadas na geração do movimento contábil (ex: expressão de
-- regra referencia coluna ausente na posição). Persistidas por data do lote para
-- consulta futura e exportação. Idempotente.

IF OBJECT_ID('inconsistencia_processamento', 'U') IS NULL
BEGIN
    CREATE TABLE inconsistencia_processamento (
        id                          BIGINT IDENTITY PRIMARY KEY,
        data_lote_contabil          DATE NOT NULL,
        codigo_identificador_boleto VARCHAR(50),
        produto                     VARCHAR(50),
        id_regra_contabil           BIGINT,
        descricao_regra_contabil    VARCHAR(255),
        tipo                        VARCHAR(30) NOT NULL,
        expressao                   VARCHAR(1000),
        campos_faltantes            VARCHAR(500),
        detalhe                     VARCHAR(1000),
        criado_em                   DATETIME2 NOT NULL DEFAULT SYSDATETIME()
    );
    CREATE INDEX IX_inconsistencia_data ON inconsistencia_processamento (data_lote_contabil);
END;
