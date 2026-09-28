package model

import "time"

// Tipos de inconsistência detectados durante a geração do movimento contábil.
const (
	InconsistenciaPreCondicao = "PRE_CONDICAO"
	InconsistenciaCondicao    = "CONDICAO"
	InconsistenciaCampoValor  = "CAMPO_VALOR"
)

// InconsistenciaProcessamento registra uma falha ocorrida ao processar uma posição
// contra uma regra/condição — por exemplo, quando a expressão referencia um campo
// que não existe na posição. Nesses casos o lançamento não é gerado e a
// inconsistência é persistida para consulta futura por data do movimento.
type InconsistenciaProcessamento struct {
	ID                        int64     `json:"id"`
	DataLoteContabil          time.Time `json:"data_lote_contabil"`
	CodigoIdentificadorBoleto string    `json:"codigo_identificador_boleto"`
	Produto                   string    `json:"produto"`
	Dominio                   string    `json:"dominio"`
	IDRegraContabil           int64     `json:"id_regra_contabil"`
	DescricaoRegraContabil    string    `json:"descricao_regra_contabil"`
	Tipo                      string    `json:"tipo"`      // PRE_CONDICAO | CONDICAO | CAMPO_VALOR
	Expressao                 string    `json:"expressao"` // expressão que falhou
	CamposFaltantes           string    `json:"campos_faltantes"`
	Detalhe                   string    `json:"detalhe"`
	CriadoEm                  time.Time `json:"criado_em"`
}
