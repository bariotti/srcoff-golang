package model

import "time"

type LancamentoContabil struct {
	ID                        int64     `json:"id"`
	DataLoteContabil          time.Time `json:"data_lote_contabil"`
	CodigoVersaoConteudo      int       `json:"codigo_versao_conteudo"`
	CodigoIdentificadorBoleto string    `json:"codigo_identificador_boleto"`
	ValorLancamentoContabil   float64   `json:"valor_lancamento_contabil"`
	MoedaLancamentoContabil   string    `json:"moeda_lancamento_contabil"`
	ContaDebito               string    `json:"conta_debito"`
	ContaCredito              string    `json:"conta_credito"`
	IndicadorReversao         bool      `json:"indicador_reversao"`
	DescricaoRegraContabil    string    `json:"descricao_regra_contabil"`
	DescricaoCondicaoContabil string    `json:"descricao_condicao_contabil"`
	IDRegraContabil           int64     `json:"id_regra_contabil"`
}
