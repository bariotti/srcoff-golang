-- Seed: Regras contábeis iniciais NDF - Registro de Notional - Nassau
-- Conforme Requisito 4 da spec SRCOff

-- ============================================================
-- Regra 1: NDF Nassau - Afiliada - MTM Positivo
-- ============================================================
INSERT INTO regra_contabil (descricao, codigo_produto_corporativo, ativo)
VALUES ('NDF Nassau - Contraparte Afiliada - MTM Positivo', 'NDF', 1);

-- ============================================================
-- Regra 2: NDF Nassau - Afiliada - MTM Negativo
-- ============================================================
INSERT INTO regra_contabil (descricao, codigo_produto_corporativo, ativo)
VALUES ('NDF Nassau - Contraparte Afiliada - MTM Negativo', 'NDF', 1);

-- ============================================================
-- Regra 3: NDF Nassau - Não Afiliada - MTM Positivo
-- ============================================================
INSERT INTO regra_contabil (descricao, codigo_produto_corporativo, ativo)
VALUES ('NDF Nassau - Contraparte Nao Afiliada - MTM Positivo', 'NDF', 1);

-- ============================================================
-- Regra 4: NDF Nassau - Não Afiliada - MTM Negativo
-- ============================================================
INSERT INTO regra_contabil (descricao, codigo_produto_corporativo, ativo)
VALUES ('NDF Nassau - Contraparte Nao Afiliada - MTM Negativo', 'NDF', 1);

-- ============================================================
-- Condições vinculadas às regras
-- Usa SCOPE_IDENTITY() para capturar os IDs gerados
-- ============================================================

-- Condição da Regra 1: Nassau + Afiliada + MTM > 0
-- conta_debito=111111111, conta_credito=222222222
-- valor = principal_remanescente + valor_mtm, moeda = moeda_principal_remanescente
INSERT INTO condicao_regra (id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, ativo)
SELECT id, 
       'descricao_veiculo == "NASSAU" && indicador_contraparte_afiliada == true && valor_mtm > 0',
       '111111111',
       '222222222',
       'principal_remanescente + valor_mtm',
       'moeda_principal_remanescente',
       1
FROM regra_contabil
WHERE descricao = 'NDF Nassau - Contraparte Afiliada - MTM Positivo';

-- Condição da Regra 2: Nassau + Afiliada + MTM < 0
-- conta_debito=333333333, conta_credito=444444444
-- valor = principal_remanescente, moeda = moeda_principal_remanescente
INSERT INTO condicao_regra (id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, ativo)
SELECT id,
       'descricao_veiculo == "NASSAU" && indicador_contraparte_afiliada == true && valor_mtm < 0',
       '333333333',
       '444444444',
       'principal_remanescente',
       'moeda_principal_remanescente',
       1
FROM regra_contabil
WHERE descricao = 'NDF Nassau - Contraparte Afiliada - MTM Negativo';

-- Condição da Regra 3: Nassau + Não Afiliada + MTM > 0
-- conta_debito=555555555, conta_credito=666666666
-- valor = principal_remanescente + valor_mtm, moeda = moeda_principal_remanescente
INSERT INTO condicao_regra (id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, ativo)
SELECT id,
       'descricao_veiculo == "NASSAU" && indicador_contraparte_afiliada == false && valor_mtm > 0',
       '555555555',
       '666666666',
       'principal_remanescente + valor_mtm',
       'moeda_principal_remanescente',
       1
FROM regra_contabil
WHERE descricao = 'NDF Nassau - Contraparte Nao Afiliada - MTM Positivo';

-- Condição da Regra 4: Nassau + Não Afiliada + MTM < 0
-- conta_debito=777777777, conta_credito=888888888
-- valor = principal_remanescente, moeda = moeda_principal_remanescente
INSERT INTO condicao_regra (id_regra, condicao, conta_debito, conta_credito, campo_valor, campo_moeda, ativo)
SELECT id,
       'descricao_veiculo == "NASSAU" && indicador_contraparte_afiliada == false && valor_mtm < 0',
       '777777777',
       '888888888',
       'principal_remanescente',
       'moeda_principal_remanescente',
       1
FROM regra_contabil
WHERE descricao = 'NDF Nassau - Contraparte Nao Afiliada - MTM Negativo';
