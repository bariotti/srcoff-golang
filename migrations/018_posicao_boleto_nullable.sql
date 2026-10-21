-- migrations/018_posicao_boleto_nullable.sql
-- A posição de carteira é 100% dinâmica (coluna `campos` NVARCHAR(MAX), migration 002).
-- O código insere apenas (data_posicao_carteira, codigo_versao_conteudo, campos). As colunas
-- de negócio ANTIGAS deixaram de ser usadas; porém `codigo_identificador_boleto` permaneceu
-- VARCHAR(50) NOT NULL (migration 001) e bloqueia o INSERT com o erro
-- "Cannot insert the value NULL into column 'codigo_identificador_boleto'".
-- Esta migration torna a coluna anulável. Idempotente (só altera se ainda for NOT NULL).
-- Observação: as colunas antigas (descricao_veiculo, valor_mtm, etc.) já são anuláveis e
-- podem ser removidas manualmente depois, já que os dados vivem em `campos`.

IF EXISTS (
    SELECT 1 FROM sys.columns
    WHERE object_id = OBJECT_ID('posicao_carteira')
      AND name = 'codigo_identificador_boleto'
      AND is_nullable = 0
)
BEGIN
    ALTER TABLE posicao_carteira ALTER COLUMN codigo_identificador_boleto VARCHAR(50) NULL;
END;
