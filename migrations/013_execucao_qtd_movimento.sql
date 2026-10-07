-- migrations/013_execucao_qtd_movimento.sql
-- Contagem BRUTA de lançamentos de movimento (não-estorno) gerados por execução.
-- Diferente de qtd_lancamentos (que é a contagem VISÍVEL, sem cancelados), qtd_movimento
-- indica se a data realmente gerou contábil — base da obrigatoriedade de D-1 e da escolha
-- da data de estorno. Idempotente.

IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('movimento_execucao') AND name = 'qtd_movimento')
    ALTER TABLE movimento_execucao ADD qtd_movimento INT NULL;
