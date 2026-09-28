-- migrations/004_parametrizacao.sql
-- Opções parametrizáveis usadas nos combos do sistema (menu Parametrizações).
-- Ex.: valores possíveis para Produto e Domínio da regra contábil. Idempotente.

IF OBJECT_ID('parametrizacao', 'U') IS NULL
BEGIN
    CREATE TABLE parametrizacao (
        id        BIGINT IDENTITY PRIMARY KEY,
        categoria VARCHAR(50)  NOT NULL,
        valor     VARCHAR(255) NOT NULL
    );
    CREATE UNIQUE INDEX UX_parametrizacao_cat_valor ON parametrizacao (categoria, valor);
END;

-- Seed de opções padrão (só insere as que ainda não existem).
INSERT INTO parametrizacao (categoria, valor)
SELECT c, v FROM (VALUES
    ('produto', 'NDF'),
    ('produto', 'SWAP'),
    ('produto', 'FXO'),
    ('dominio', 'Posição'),
    ('dominio', 'Liquidação')
) AS seed(c, v)
WHERE NOT EXISTS (
    SELECT 1 FROM parametrizacao p WHERE p.categoria = seed.c AND p.valor = seed.v
);
