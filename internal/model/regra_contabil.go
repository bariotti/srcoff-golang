package model

import "strings"

// Tipos de lançamento de uma regra contábil (combo obrigatório no cadastro):
//   - TipoReverte:            posta e, no dia seguinte, estorna os lançamentos (comportamento clássico);
//   - TipoNaoReverte:         posta e NÃO estorna (lançamentos definitivos);
//   - TipoIncremental:        rótulo "Incremental/Não Reverte". NÃO estorna; o valor lançado em D0 é
//     |valor_D0 - valor_D-N| para a mesma (boleto, regra), onde D-N é a mesma data-base do estorno.
//     Se o boleto some da posição, o saldo permanece (sem baixa).
//   - TipoIncrementalReverte: rótulo "Incremental/Reverte". Igual ao incremental no cálculo diário,
//     porém se o boleto some da posição de D0, gera uma BAIXA que zera o saldo líquido postado
//     do boleto para a regra (RN-121).
const (
	TipoReverte            = "reverte"
	TipoNaoReverte         = "nao_reverte"
	TipoIncremental        = "incremental"
	TipoIncrementalReverte = "incremental_reverte"
)

type RegraContabil struct {
	ID                       int64           `json:"id"`
	Descricao                string          `json:"descricao"`
	CodigoProdutoCorporativo string          `json:"codigo_produto_corporativo"` // separado por vírgula: "NDF,SWAP"
	Dominio                  string          `json:"dominio"`                    // separado por vírgula: "Posição,Liquidação"
	CampoProduto             string          `json:"campo_produto"`              // nome do campo da posição comparado com o código do produto
	PreCondicao              string          `json:"pre_condicao"`               // expressão opcional combinada com E a cada condição da regra
	Ativo                    bool            `json:"ativo"`
	TipoLancamento           string          `json:"tipo_lancamento"` // reverte | nao_reverte | incremental
	Condicoes                []CondicaoRegra `json:"condicoes"`
}

// EhReverte indica se a regra gera estorno de D-1 (comportamento clássico).
// Vazio é tratado como "reverte" por retrocompatibilidade/segurança.
func (r RegraContabil) EhReverte() bool {
	return r.TipoLancamento == TipoReverte || r.TipoLancamento == ""
}

// EhIncremental indica se a regra é do tipo incremental "Não Reverte"
// (cálculo |D0 - D-N|; boleto que some mantém o saldo).
func (r RegraContabil) EhIncremental() bool {
	return r.TipoLancamento == TipoIncremental
}

// EhIncrementalReverte indica se a regra é do tipo incremental "Reverte"
// (cálculo |D0 - D-N|; boleto que some é baixado — RN-121).
func (r RegraContabil) EhIncrementalReverte() bool {
	return r.TipoLancamento == TipoIncrementalReverte
}

// UsaCalculoIncremental indica se a regra usa o cálculo incremental diário
// (|D0 - D-N|), comum aos tipos incremental e incremental_reverte.
func (r RegraContabil) UsaCalculoIncremental() bool {
	return r.EhIncremental() || r.EhIncrementalReverte()
}

// Spec: RN-024 / ADR-001 (docs/especificacao.md §5.3, §12).
// NormalizaTipoLancamento devolve o tipo de lançamento canônico
// (reverte | nao_reverte | incremental), ignorando espaços e maiúsculas/minúsculas.
// Qualquer valor desconhecido (inclusive vazio) é tratado como "reverte".
// É a única fonte da regra de normalização — usada por todos os backends e pela importação.
func NormalizaTipoLancamento(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case TipoNaoReverte:
		return TipoNaoReverte
	case TipoIncremental:
		return TipoIncremental
	case TipoIncrementalReverte:
		return TipoIncrementalReverte
	default:
		return TipoReverte
	}
}

type CondicaoRegra struct {
	ID           int64  `json:"id"`
	IDRegra      int64  `json:"id_regra"`
	Condicao     string `json:"condicao"`
	ContaDebito  string `json:"conta_debito"`
	ContaCredito string `json:"conta_credito"`
	CampoValor   string `json:"campo_valor"`
	CampoMoeda   string `json:"campo_moeda"`
	CampoBoleto  string `json:"campo_boleto"` // nome do campo da posição usado como identificador do boleto no lançamento
	Ativo        bool   `json:"ativo"`
	// Moeda base do veículo (RF-036/RN-182). BaseNaPosicao define a origem dos dois dados:
	//   - true  → valor e moeda base vêm da posição (CampoValorMoedaBase + CampoMoedaBase);
	//             CampoDataConversao é ignorado.
	//   - false → serão CONVERTIDOS na geração usando a data de CampoDataConversao;
	//             CampoValorMoedaBase/CampoMoedaBase são ignorados.
	BaseNaPosicao       bool   `json:"base_na_posicao"`
	CampoValorMoedaBase string `json:"campo_valor_moeda_base"`
	CampoMoedaBase      string `json:"campo_moeda_base"`
	CampoDataConversao  string `json:"campo_data_conversao"`
}
