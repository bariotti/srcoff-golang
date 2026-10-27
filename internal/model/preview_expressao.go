package model

// PreviewExpressao é o resultado de avaliar uma expressão (condição ou valor) contra uma
// linha de posição REAL de uma data — para o usuário testar antes de ativar a regra.
// Spec: RF-072.
type PreviewExpressao struct {
	Encontrada      bool        `json:"encontrada"` // achou uma posição que casa com o escopo
	Boleto          string      `json:"boleto"`
	CamposRef       []string    `json:"campos_referenciados"`
	CamposFaltantes []string    `json:"campos_faltantes"`
	Resultado       interface{} `json:"resultado"` // bool (condição) ou número (valor)
	Erro            string      `json:"erro,omitempty"`
}
