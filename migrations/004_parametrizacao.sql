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

-- Sem seed de opções padrão (ADR-009): os combos (Produto/Domínio) nascem vazios e são
-- cadastrados pelo usuário via Parametrizações. Mantido em paridade com os backends
-- file e sqlite. (Bases já provisionadas não são afetadas — nada é removido aqui.)
