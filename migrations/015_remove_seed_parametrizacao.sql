-- migrations/015_remove_seed_parametrizacao.sql
-- ADR-009: remove as opções de combos que eram semeadas por padrão, em bases SQL Server
-- JÁ provisionadas (a migration 004 deixou de semeá-las em bases novas).
-- Remove SOMENTE os 5 pares padrão; quaisquer opções cadastradas pelo usuário permanecem.
-- Idempotente (rodar de novo não faz nada se já não existirem).
--
-- Atenção: isto remove 'NDF'/'SWAP'/'FXO' (produto) e 'Posição'/'Liquidação' (domínio)
-- apenas da LISTA de opções dos combos (tabela parametrizacao). Não afeta regras,
-- posições ou movimento que referenciem esses valores.

-- Padrões de 'dominio' usam LIKE sem acentos ('Posi%o', 'Liquida%o') para serem
-- robustos a diferenças de code page (VARCHAR) entre o seed via sqlcmd e os INSERTs
-- feitos pela aplicação (go-mssqldb).
DELETE FROM parametrizacao
WHERE (categoria = 'produto' AND valor IN ('NDF', 'SWAP', 'FXO'))
   OR (categoria = 'dominio' AND (valor LIKE 'Posi%o' OR valor LIKE 'Liquida%o'));
