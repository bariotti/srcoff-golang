package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ddlGarantido são statements T-SQL idempotentes que garantem, em bancos SQL Server já
// existentes, as colunas/tabelas que o código atual usa e que foram adicionadas ao longo
// da evolução do schema (equivalente às migrations). Cada um só altera quando necessário.
// Isso evita erros como "Invalid column name 'origem'" quando uma migration não foi rodada
// manualmente — o SQL Server passa a se auto-corrigir no startup, como o backend SQLite.
var ddlGarantido = []string{
	// posicao_carteira: modelo dinâmico (coluna campos) e boleto anulável.
	`IF COL_LENGTH('posicao_carteira','campos') IS NULL BEGIN ALTER TABLE posicao_carteira ADD campos NVARCHAR(MAX) NULL END`,
	`IF COL_LENGTH('posicao_carteira','produto') IS NULL BEGIN ALTER TABLE posicao_carteira ADD produto VARCHAR(50) NULL END`,
	`IF EXISTS (SELECT 1 FROM sys.columns WHERE object_id=OBJECT_ID('posicao_carteira') AND name='codigo_identificador_boleto' AND is_nullable=0) BEGIN ALTER TABLE posicao_carteira ALTER COLUMN codigo_identificador_boleto VARCHAR(50) NULL END`,
	// regra_contabil
	`IF COL_LENGTH('regra_contabil','dominio') IS NULL BEGIN ALTER TABLE regra_contabil ADD dominio VARCHAR(100) NULL END`,
	`IF COL_LENGTH('regra_contabil','campo_produto') IS NULL BEGIN ALTER TABLE regra_contabil ADD campo_produto VARCHAR(100) NULL END`,
	`IF COL_LENGTH('regra_contabil','pre_condicao') IS NULL BEGIN ALTER TABLE regra_contabil ADD pre_condicao VARCHAR(500) NULL END`,
	`IF COL_LENGTH('regra_contabil','tipo_lancamento') IS NULL BEGIN ALTER TABLE regra_contabil ADD tipo_lancamento VARCHAR(20) NULL END`,
	// condicao_regra
	`IF COL_LENGTH('condicao_regra','campo_boleto') IS NULL BEGIN ALTER TABLE condicao_regra ADD campo_boleto VARCHAR(100) NULL END`,
	`IF COL_LENGTH('condicao_regra','base_na_posicao') IS NULL BEGIN ALTER TABLE condicao_regra ADD base_na_posicao BIT NULL END`,
	`IF COL_LENGTH('condicao_regra','campo_valor_moeda_base') IS NULL BEGIN ALTER TABLE condicao_regra ADD campo_valor_moeda_base VARCHAR(100) NULL END`,
	`IF COL_LENGTH('condicao_regra','campo_moeda_base') IS NULL BEGIN ALTER TABLE condicao_regra ADD campo_moeda_base VARCHAR(100) NULL END`,
	`IF COL_LENGTH('condicao_regra','campo_data_conversao') IS NULL BEGIN ALTER TABLE condicao_regra ADD campo_data_conversao VARCHAR(100) NULL END`,
	// movimento_contabil
	`IF COL_LENGTH('movimento_contabil','produto') IS NULL BEGIN ALTER TABLE movimento_contabil ADD produto VARCHAR(50) NULL END`,
	`IF COL_LENGTH('movimento_contabil','dominio') IS NULL BEGIN ALTER TABLE movimento_contabil ADD dominio VARCHAR(100) NULL END`,
	`IF COL_LENGTH('movimento_contabil','origem') IS NULL BEGIN ALTER TABLE movimento_contabil ADD origem VARCHAR(20) NULL END`,
	`IF COL_LENGTH('movimento_contabil','veiculo') IS NULL BEGIN ALTER TABLE movimento_contabil ADD veiculo VARCHAR(100) NULL END`,
	`IF COL_LENGTH('movimento_contabil','valor_moeda_base_veiculo') IS NULL BEGIN ALTER TABLE movimento_contabil ADD valor_moeda_base_veiculo FLOAT NULL END`,
	`IF COL_LENGTH('movimento_contabil','moeda_base_veiculo') IS NULL BEGIN ALTER TABLE movimento_contabil ADD moeda_base_veiculo VARCHAR(50) NULL END`,
	// movimento_execucao
	`IF COL_LENGTH('movimento_execucao','qtd_movimento') IS NULL BEGIN ALTER TABLE movimento_execucao ADD qtd_movimento INT NULL END`,
	// inconsistencia_processamento
	`IF COL_LENGTH('inconsistencia_processamento','dominio') IS NULL BEGIN ALTER TABLE inconsistencia_processamento ADD dominio VARCHAR(100) NULL END`,
	// padrao_arquivo (parâmetros de parsing/import)
	`IF COL_LENGTH('padrao_arquivo','dominio') IS NULL BEGIN ALTER TABLE padrao_arquivo ADD dominio VARCHAR(100) NULL END`,
	`IF COL_LENGTH('padrao_arquivo','delimitador') IS NULL BEGIN ALTER TABLE padrao_arquivo ADD delimitador VARCHAR(5) NULL END`,
	`IF COL_LENGTH('padrao_arquivo','separador_decimal') IS NULL BEGIN ALTER TABLE padrao_arquivo ADD separador_decimal VARCHAR(5) NULL END`,
	`IF COL_LENGTH('padrao_arquivo','separador_milhar') IS NULL BEGIN ALTER TABLE padrao_arquivo ADD separador_milhar VARCHAR(5) NULL END`,
	`IF COL_LENGTH('padrao_arquivo','formato_data') IS NULL BEGIN ALTER TABLE padrao_arquivo ADD formato_data VARCHAR(20) NULL END`,
	`IF COL_LENGTH('padrao_arquivo','coluna_data') IS NULL BEGIN ALTER TABLE padrao_arquivo ADD coluna_data VARCHAR(100) NULL END`,
	`IF COL_LENGTH('padrao_arquivo','coluna_boleto') IS NULL BEGIN ALTER TABLE padrao_arquivo ADD coluna_boleto VARCHAR(100) NULL END`,
	`IF COL_LENGTH('padrao_arquivo','coluna_veiculo') IS NULL BEGIN ALTER TABLE padrao_arquivo ADD coluna_veiculo VARCHAR(100) NULL END`,
	// veiculo_de_para (De/Para nome do veículo → código; usado só na extração CSV — RF-035)
	`IF OBJECT_ID('veiculo_de_para','U') IS NULL BEGIN
	   CREATE TABLE veiculo_de_para (
	     id BIGINT IDENTITY(1,1) PRIMARY KEY,
	     veiculo VARCHAR(255) NOT NULL, codigo VARCHAR(100) NOT NULL);
	 END`,
	`IF COL_LENGTH('veiculo_de_para','moeda_base') IS NULL BEGIN ALTER TABLE veiculo_de_para ADD moeda_base VARCHAR(50) NULL END`,
	// cotacao (taxa de conversão entre moedas por data — RF-037)
	`IF OBJECT_ID('cotacao','U') IS NULL BEGIN
	   CREATE TABLE cotacao (
	     id BIGINT IDENTITY(1,1) PRIMARY KEY,
	     data DATE NOT NULL, moeda1 VARCHAR(50) NOT NULL, moeda2 VARCHAR(50) NOT NULL,
	     direcao VARCHAR(20) NOT NULL, taxa FLOAT NOT NULL);
	   CREATE INDEX IX_cotacao_lookup ON cotacao (data, moeda1, moeda2);
	 END`,
	`IF COL_LENGTH('padrao_arquivo','obrigatorio_mov_d1') IS NULL BEGIN ALTER TABLE padrao_arquivo ADD obrigatorio_mov_d1 BIT NULL END`,
	// saldo_conta (projeção materializada — RN-170). Tamanhos pequenos p/ a PK caber em 900 bytes.
	`IF OBJECT_ID('saldo_conta','U') IS NULL BEGIN
	   CREATE TABLE saldo_conta (
	     produto VARCHAR(100) NOT NULL, dominio VARCHAR(100) NOT NULL, codigo_identificador_boleto VARCHAR(100) NOT NULL,
	     id_regra_contabil BIGINT NOT NULL, conta_debito VARCHAR(50) NOT NULL, conta_credito VARCHAR(50) NOT NULL,
	     saldo FLOAT NOT NULL DEFAULT 0, moeda_lancamento VARCHAR(50) NULL, descricao_regra VARCHAR(255) NULL, descricao_condicao VARCHAR(500) NULL,
	     CONSTRAINT PK_saldo_conta PRIMARY KEY (produto, dominio, codigo_identificador_boleto, id_regra_contabil, conta_debito, conta_credito));
	   CREATE INDEX IX_saldo_combo ON saldo_conta (produto, dominio);
	 END`,
	// Índices que tornam as subqueries correlacionadas (vigente/saldo-zero) e a geração
	// rápidas em volume — seeks no lugar de scans. Idempotentes.
	`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name='IX_mov_data_prod_dom_ver' AND object_id=OBJECT_ID('movimento_contabil')) BEGIN CREATE INDEX IX_mov_data_prod_dom_ver ON movimento_contabil (data_lote_contabil, produto, dominio, codigo_versao_conteudo) END`,
	`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name='IX_mov_data_boleto_regra' AND object_id=OBJECT_ID('movimento_contabil')) BEGIN CREATE INDEX IX_mov_data_boleto_regra ON movimento_contabil (data_lote_contabil, codigo_identificador_boleto, id_regra_contabil) END`,
	`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name='IX_mov_data_reversao' AND object_id=OBJECT_ID('movimento_contabil')) BEGIN CREATE INDEX IX_mov_data_reversao ON movimento_contabil (data_lote_contabil, indicador_reversao) END`,
	`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name='IX_exec_prod_dom_data' AND object_id=OBJECT_ID('movimento_execucao')) BEGIN CREATE INDEX IX_exec_prod_dom_data ON movimento_execucao (produto, dominio, data_lote) END`,
	`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name='IX_exec_data' AND object_id=OBJECT_ID('movimento_execucao')) BEGIN CREATE INDEX IX_exec_data ON movimento_execucao (data_lote) END`,
	`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name='IX_incons_data' AND object_id=OBJECT_ID('inconsistencia_processamento')) BEGIN CREATE INDEX IX_incons_data ON inconsistencia_processamento (data_lote_contabil) END`,
	`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name='IX_posicao_data_versao' AND object_id=OBJECT_ID('posicao_carteira')) BEGIN CREATE INDEX IX_posicao_data_versao ON posicao_carteira (data_posicao_carteira, codigo_versao_conteudo) END`,
	`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name='IX_cond_regra' AND object_id=OBJECT_ID('condicao_regra')) BEGIN CREATE INDEX IX_cond_regra ON condicao_regra (id_regra, ativo) END`,
}

// GarantirSchema executa, de forma idempotente, os ajustes de schema que o código atual
// exige no SQL Server. Não falha a inicialização por um statement isolado: registra os
// erros e segue (retorna um erro agregado apenas para log). As tabelas-base continuam sendo
// criadas pelas migrations (001+); aqui garantimos as colunas/tabelas adicionadas depois.
func GarantirSchema(ctx context.Context, db *sql.DB) error {
	var erros []string
	for _, stmt := range ddlGarantido {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			erros = append(erros, err.Error())
		}
	}
	if len(erros) > 0 {
		return fmt.Errorf("garantir schema SQL Server: %s", strings.Join(erros, "; "))
	}
	return nil
}
