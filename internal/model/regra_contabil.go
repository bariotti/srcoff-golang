package model

import "strings"

// Tipos de lançamento de uma regra contábil (combo obrigatório no cadastro):
//   - TipoReverte:     posta e, no dia seguinte, estorna os lançamentos (comportamento clássico);
//   - TipoNaoReverte:  posta e NÃO estorna (lançamentos definitivos);
//   - TipoIncremental: NÃO estorna; o valor lançado em D0 é |valor_D0 - valor_D-N| para a
//     mesma (boleto, regra), onde D-N é a mesma data-base usada no estorno.
const (
	TipoReverte     = "reverte"
	TipoNaoReverte  = "nao_reverte"
	TipoIncremental = "incremental"
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

// EhIncremental indica se a regra usa o cálculo incremental (|D0 - D-N|, sem estorno).
func (r RegraContabil) EhIncremental() bool {
	return r.TipoLancamento == TipoIncremental
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
}
