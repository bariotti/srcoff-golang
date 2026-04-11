package model

type PaginaLancamentos struct {
	Total       int                  `json:"total"`
	Pagina      int                  `json:"pagina"`
	Tamanho     int                  `json:"tamanho"`
	Lancamentos []LancamentoContabil `json:"lancamentos"`
}
