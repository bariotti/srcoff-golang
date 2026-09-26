-- migrations/003_regra_campos_extras.sql
-- Novos campos na regra contábil: campo_data (coluna de data da posição),
-- pre_condicao (condição combinada com E a cada condição) e natureza (informativo).
-- Idempotente.

IF NOT EXISTS (
    SELECT 1 FROM sys.columns
    WHERE object_id = OBJECT_ID('regra_contabil') AND name = 'campo_data'
)
BEGIN
    ALTER TABLE regra_contabil ADD campo_data VARCHAR(100) NULL;
END;

IF NOT EXISTS (
    SELECT 1 FROM sys.columns
    WHERE object_id = OBJECT_ID('regra_contabil') AND name = 'pre_condicao'
)
BEGIN
    ALTER TABLE regra_contabil ADD pre_condicao VARCHAR(1000) NULL;
END;

IF NOT EXISTS (
    SELECT 1 FROM sys.columns
    WHERE object_id = OBJECT_ID('regra_contabil') AND name = 'natureza'
)
BEGIN
    ALTER TABLE regra_contabil ADD natureza VARCHAR(255) NULL;
END;
