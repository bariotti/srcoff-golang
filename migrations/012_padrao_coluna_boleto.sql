-- migrations/012_padrao_coluna_boleto.sql
-- Nome da coluna do arquivo que contém o número do boleto, por padrão de arquivo.
-- Essa coluna é persistida SEMPRE como texto na importação, preservando zeros à
-- esquerda e a precisão de identificadores longos. Obrigatório no cadastro
-- (validação na aplicação); a coluna é NULL para permitir padrões legados. Idempotente.

IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('padrao_arquivo') AND name = 'coluna_boleto')
    ALTER TABLE padrao_arquivo ADD coluna_boleto VARCHAR(100) NULL;
