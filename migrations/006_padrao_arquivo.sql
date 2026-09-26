-- migrations/006_padrao_arquivo.sql
-- Padrões de nome de arquivo → produto (para importação em lote e monitoramento de
-- pasta) e configurações chave→valor (ex: pastas monitorada/processados). Idempotente.

IF OBJECT_ID('padrao_arquivo', 'U') IS NULL
BEGIN
    CREATE TABLE padrao_arquivo (
        id      BIGINT IDENTITY PRIMARY KEY,
        padrao  VARCHAR(255) NOT NULL,
        produto VARCHAR(50)  NOT NULL
    );
END;

IF OBJECT_ID('configuracao', 'U') IS NULL
BEGIN
    CREATE TABLE configuracao (
        chave VARCHAR(100) NOT NULL PRIMARY KEY,
        valor VARCHAR(1000) NULL
    );
END;
