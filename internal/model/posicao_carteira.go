package model

import "time"

type PosicaoCarteira struct {
	ID                           int64
	DataPosicaoCarteira          time.Time
	CodigoVersaoConteudo         int
	CodigoIdentificadorBoleto    string
	DescricaoVeiculo             string
	IndicadorContraparteAfiliada bool
	ValorMTM                     float64
	PrincipalRemanescente        float64
	MoedaPrincipalRemanescente   string
}
