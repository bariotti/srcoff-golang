package model

// TipoInconsistencia descreve o tipo de problema encontrado na conciliação.
type TipoInconsistencia string

const (
	InconsistenciaSemMovimento TipoInconsistencia = "POSICAO_SEM_MOVIMENTO"
	InconsistenciaDuplicidade  TipoInconsistencia = "LANCAMENTO_DUPLICADO"
)

// Inconsistencia representa um problema encontrado na conciliação.
type Inconsistencia struct {
	Tipo                      TipoInconsistencia
	CodigoIdentificadorBoleto string
	DescricaoRegra            string
	IndicadorReversao         bool
	Detalhe                   string
}

// ResultadoConciliacao agrupa o resultado da conciliação de uma data.
type ResultadoConciliacao struct {
	Data             string
	TotalPosicoes    int
	TotalMovimentos  int
	Inconsistencias  []Inconsistencia
}
