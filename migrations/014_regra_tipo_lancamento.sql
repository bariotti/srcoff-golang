-- migrations/014_regra_tipo_lancamento.sql
-- Tipo de lançamento da regra contábil (substitui a flag posta_reverte por um enum):
--   'reverte'      -> posta e estorna (comportamento clássico; antigo posta_reverte = 1)
--   'nao_reverte'  -> posta e não estorna (antigo posta_reverte = 0)
--   'incremental'  -> não estorna; valor = | |valor_D0| - |valor_D-N| | por (boleto, regra)
-- Idempotente. A coluna antiga posta_reverte é mantida (não usada pela aplicação).

IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('regra_contabil') AND name = 'tipo_lancamento')
    ALTER TABLE regra_contabil ADD tipo_lancamento VARCHAR(20) NULL;
GO

-- Backfill a partir de posta_reverte (quando a coluna antiga existir).
IF EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('regra_contabil') AND name = 'posta_reverte')
    UPDATE regra_contabil
       SET tipo_lancamento = CASE WHEN ISNULL(posta_reverte, 1) = 1 THEN 'reverte' ELSE 'nao_reverte' END
     WHERE tipo_lancamento IS NULL;
GO

-- Garante valor para quaisquer linhas remanescentes.
UPDATE regra_contabil SET tipo_lancamento = 'reverte' WHERE tipo_lancamento IS NULL;
