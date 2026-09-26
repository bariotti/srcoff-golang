package model

// PadraoArquivo mapeia um padrão de nome de arquivo (glob, ex: "posicao_ndf*.csv")
// ao produto correspondente. Usado na importação em lote e no monitoramento de
// pasta para identificar o produto da posição pelo nome do arquivo.
type PadraoArquivo struct {
	ID      int64  `json:"id"`
	Padrao  string `json:"padrao"`
	Produto string `json:"produto"`
}
