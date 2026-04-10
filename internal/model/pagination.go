package model

type PaginaLancamentos struct {
	Total       int
	Pagina      int
	Tamanho     int
	Lancamentos []LancamentoContabil
}
