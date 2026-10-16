-- migrations/017_movimento_origem.sql
-- Marca a origem de cada lançamento contábil: 'automatico' (gerado pelo motor) ou 'manual'
-- (inserido pelo usuário). Idempotente. Bases existentes: linhas atuais ficam como 'automatico'.

IF COL_LENGTH('movimento_contabil', 'origem') IS NULL
BEGIN
    ALTER TABLE movimento_contabil ADD origem VARCHAR(20) NULL;
END;
GO

UPDATE movimento_contabil SET origem = 'automatico' WHERE origem IS NULL;
GO
