package model

import "time"

// Origem de um lançamento contábil (diferencia gerado pelo motor x inserido manualmente).
const (
	OrigemAutomatico = "automatico"
	OrigemManual     = "manual"
)

type LancamentoContabil struct {
	ID                        int64     `json:"id"`
	DataLoteContabil          time.Time `json:"data_lote_contabil"`
	CodigoVersaoConteudo      int       `json:"codigo_versao_conteudo"`
	CodigoIdentificadorBoleto string    `json:"codigo_identificador_boleto"`
	ValorLancamentoContabil   float64   `json:"valor_lancamento_contabil"`
	MoedaLancamentoContabil   string    `json:"moeda_lancamento_contabil"`
	ContaDebito               string    `json:"conta_debito"`
	ContaCredito              string    `json:"conta_credito"`
	Produto                   string    `json:"produto"`
	Dominio                   string    `json:"dominio"`
	IndicadorReversao         bool      `json:"indicador_reversao"`
	DescricaoRegraContabil    string    `json:"descricao_regra_contabil"`
	DescricaoCondicaoContabil string    `json:"descricao_condicao_contabil"`
	IDRegraContabil           int64     `json:"id_regra_contabil"`
	Origem                    string    `json:"origem"` // automatico | manual (vazio = automatico)
}

// OrigemOuPadrao devolve a origem normalizada (vazio = automatico), para exibição/filtro.
func (l LancamentoContabil) OrigemOuPadrao() string {
	if l.Origem == OrigemManual {
		return OrigemManual
	}
	return OrigemAutomatico
}

// EhManual indica se o lançamento foi inserido manualmente.
func (l LancamentoContabil) EhManual() bool { return l.Origem == OrigemManual }

// LancamentoManualInput é a entrada de um lançamento inserido manualmente (tela ou CSV).
// As contas (débito/crédito) e as descrições vêm da regra/condição referenciadas pelos ids.
type LancamentoManualInput struct {
	Data              string  `json:"data"` // YYYY-MM-DD
	Produto           string  `json:"produto"`
	Dominio           string  `json:"dominio"`
	Boleto            string  `json:"boleto"`
	Moeda             string  `json:"moeda"`
	IDRegra           int64   `json:"id_regra"`
	IDCondicao        int64   `json:"id_condicao"`
	Valor             float64 `json:"valor"`
	IndicadorReversao bool    `json:"indicador_reversao"`
}
