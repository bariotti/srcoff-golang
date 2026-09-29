-- migrations/010_padrao_coluna_data.sql
-- Nome da coluna do arquivo que contém a data base da posição, por padrão de arquivo.
-- É a coluna que define a data do lote. Obrigatório no cadastro (validação na aplicação);
-- a coluna é NULL para permitir padrões legados. Idempotente.

IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('padrao_arquivo') AND name = 'coluna_data')
    ALTER TABLE padrao_arquivo ADD coluna_data VARCHAR(100) NULL;
