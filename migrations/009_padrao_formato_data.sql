-- migrations/009_padrao_formato_data.sql
-- Formato de data das colunas de data do arquivo, por padrão de arquivo.
-- Ex.: "DD/MM/AAAA", "MM/DD/AAAA". Obrigatório no cadastro (a validação é na aplicação);
-- a coluna é NULL para permitir padrões legados. Idempotente.

IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('padrao_arquivo') AND name = 'formato_data')
    ALTER TABLE padrao_arquivo ADD formato_data VARCHAR(20) NULL;
