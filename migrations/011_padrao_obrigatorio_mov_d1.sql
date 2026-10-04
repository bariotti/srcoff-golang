-- migrations/011_padrao_obrigatorio_mov_d1.sql
-- Flag "Obrigatoriedade Movimento Contábil D-1" por padrão de arquivo.
-- NULL = padrão (true). Quando false, o contábil não valida o movimento de D-1 útil e
-- o estorno usa o movimento da maior data anterior disponível. Idempotente.

IF NOT EXISTS (SELECT 1 FROM sys.columns WHERE object_id = OBJECT_ID('padrao_arquivo') AND name = 'obrigatorio_mov_d1')
    ALTER TABLE padrao_arquivo ADD obrigatorio_mov_d1 BIT NULL;
