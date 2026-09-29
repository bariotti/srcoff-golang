package model

// PadraoArquivo mapeia um padrão de nome de arquivo (glob, ex: "posicao_ndf*.csv")
// ao produto correspondente. Usado na importação em lote e no monitoramento de
// pasta para identificar o produto da posição pelo nome do arquivo.
type PadraoArquivo struct {
	ID      int64  `json:"id"`
	Padrao  string `json:"padrao"`
	Produto string `json:"produto"`
	Dominio string `json:"dominio"`
	// Parâmetros opcionais de parsing do CSV. Vazios = comportamento automático.
	Delimitador      string `json:"delimitador"`       // ";" ou "," (vazio = auto-detecta)
	SeparadorDecimal string `json:"separador_decimal"` // "," ou "." (vazio = auto)
	SeparadorMilhar  string `json:"separador_milhar"`  // "." , "," ou "" (nenhum)
	// Formato de data de TODAS as colunas de data do arquivo (obrigatório no cadastro).
	// Guarda o rótulo escolhido (ex.: "DD/MM/AAAA", "MM/DD/AAAA"). Vazio = automático
	// (mantido apenas para padrões legados cadastrados antes deste campo).
	FormatoData string `json:"formato_data"`
	// Nome da coluna do arquivo que contém a data base da posição — é ela que define a
	// data do lote (obrigatório no cadastro). Comparada após normalização (snake_case).
	// Vazio = comportamento legado (resolve pela regra/auto-detecção).
	ColunaData string `json:"coluna_data"`
}
