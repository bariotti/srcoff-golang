package model

import "time"

// PosicaoCarteira representa um registro da tabela posicao_carteira.
// Os campos fixos são populados para uso interno do sistema.
// Campos contém TODOS os campos da linha, incluindo os fixos e quaisquer
// colunas adicionais, e é usado diretamente pelo avaliador de expressões.
type PosicaoCarteira struct {
	ID                           int64                  `json:"id"`
	DataPosicaoCarteira          time.Time              `json:"data_posicao_carteira"`
	CodigoVersaoConteudo         int                    `json:"codigo_versao_conteudo"`
	CodigoIdentificadorBoleto    string                 `json:"codigo_identificador_boleto"`
	DescricaoVeiculo             string                 `json:"descricao_veiculo"`
	IndicadorContraparteAfiliada bool                   `json:"indicador_contraparte_afiliada"`
	ValorMTM                     float64                `json:"valor_mtm"`
	PrincipalRemanescente        float64                `json:"principal_remanescente"`
	MoedaPrincipalRemanescente   string                 `json:"moeda_principal_remanescente"`
	Campos                       map[string]interface{} `json:"campos,omitempty"`
}
