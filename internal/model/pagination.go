package model

type PaginaLancamentos struct {
	Total       int                  `json:"total"`
	Pagina      int                  `json:"pagina"`
	Tamanho     int                  `json:"tamanho"`
	Lancamentos []LancamentoContabil `json:"lancamentos"`
}

type PaginaPosicoes struct {
	Total     int               `json:"total"`
	Pagina    int               `json:"pagina"`
	Tamanho   int               `json:"tamanho"`
	Registros []PosicaoCarteira `json:"registros"`
}
