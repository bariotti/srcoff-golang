-- migrations/008_padrao_csv_config.sql
-- Parâmetros opcionais de parsing do CSV por padrão de arquivo (delimitador,
-- separador decimal e separador de milhar). Vazios = comportamento automático.
-- Idempotente.

IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('padrao_arquivo') AND name = 'delimitador')
    ALTER TABLE padrao_arquivo ADD delimitador VARCHAR(2) NULL;

IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('padrao_arquivo') AND name = 'separador_decimal')
    ALTER TABLE padrao_arquivo ADD separador_decimal VARCHAR(2) NULL;

IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('padrao_arquivo') AND name = 'separador_milhar')
    ALTER TABLE padrao_arquivo ADD separador_milhar VARCHAR(2) NULL;
