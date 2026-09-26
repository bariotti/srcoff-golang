-- migrations/002_posicao_dinamica.sql
-- Torna a posição de carteira totalmente dinâmica e parametriza boleto/produto nas regras.
-- Idempotente.

-- ============================================================
-- posicao_carteira: coluna `campos` (JSON) com todos os campos dinâmicos.
-- O sistema passa a ler/gravar a posição exclusivamente por esta coluna.
-- As colunas de negócio antigas (descricao_veiculo, valor_mtm, etc.) deixam de
-- ser usadas pelo código; podem ser removidas manualmente após migrar os dados.
-- ============================================================
IF NOT EXISTS (
    SELECT 1 FROM sys.columns
    WHERE object_id = OBJECT_ID('posicao_carteira') AND name = 'campos'
)
BEGIN
    ALTER TABLE posicao_carteira ADD campos NVARCHAR(MAX) NULL;
END;

-- Backfill opcional: consolida as colunas fixas existentes em JSON na coluna `campos`
-- para as linhas ainda não migradas (só roda se as colunas antigas existirem).
IF EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('posicao_carteira') AND name = 'codigo_identificador_boleto')
BEGIN
    UPDATE posicao_carteira
    SET campos = (
        SELECT
            codigo_identificador_boleto     AS codigo_identificador_boleto,
            descricao_veiculo               AS descricao_veiculo,
            indicador_contraparte_afiliada  AS indicador_contraparte_afiliada,
            valor_mtm                       AS valor_mtm,
            principal_remanescente          AS principal_remanescente,
            moeda_principal_remanescente    AS moeda_principal_remanescente
        FROM posicao_carteira p2
        WHERE p2.id = posicao_carteira.id
        FOR JSON PATH, WITHOUT_ARRAY_WRAPPER
    )
    WHERE campos IS NULL;
END;

-- ============================================================
-- regra_contabil: campo_produto (nome do campo da posição comparado ao produto)
-- ============================================================
IF NOT EXISTS (
    SELECT 1 FROM sys.columns
    WHERE object_id = OBJECT_ID('regra_contabil') AND name = 'campo_produto'
)
BEGIN
    ALTER TABLE regra_contabil ADD campo_produto VARCHAR(100) NULL;
END;

-- ============================================================
-- condicao_regra: campo_boleto (nome do campo da posição usado como boleto)
-- ============================================================
IF NOT EXISTS (
    SELECT 1 FROM sys.columns
    WHERE object_id = OBJECT_ID('condicao_regra') AND name = 'campo_boleto'
)
BEGIN
    ALTER TABLE condicao_regra ADD campo_boleto VARCHAR(100) NULL;
END;

-- Preenche campo_boleto padrão nas condições existentes que não o definiram.
UPDATE condicao_regra
SET campo_boleto = 'codigo_identificador_boleto'
WHERE campo_boleto IS NULL OR campo_boleto = '';
