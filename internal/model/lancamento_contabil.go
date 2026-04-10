package model

import "time"

type LancamentoContabil struct {
	ID                        int64
	DataLoteContabil          time.Time
	CodigoVersaoConteudo      int
	CodigoIdentificadorBoleto string
	ValorLancamentoContabil   float64
	MoedaLancamentoContabil   string
	ContaDebito               string
	ContaCredito              string
	IndicadorReversao         bool
	DescricaoRegraContabil    string
	DescricaoCondicaoContabil string
	IDRegraContabil           int64
}
