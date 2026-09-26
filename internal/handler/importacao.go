package handler

import (
	"context"
	"fmt"
	"io"

	"srcoff/internal/service"
)

// padraoArquivoResolver resolve os produtos de um arquivo pelo seu nome.
type padraoArquivoResolver interface {
	ResolverProdutos(ctx context.Context, nomeArquivo string) ([]string, error)
}

// importarConteudoParaProduto resolve a coluna de data (via campo_data da regra do
// produto ou detecção), preenche data e produto em cada registro e importa o lote.
// Cada produto recebe uma cópia dos registros para não compartilhar mutações.
func importarConteudoParaProduto(ctx context.Context, svc posicaoCarteiraSvc, registros []map[string]interface{}, colunas []string, produto string) ([]service.LoteImportado, error) {
	colunaData := ""
	if produto != "" {
		if c, _ := svc.ResolverCampoData(ctx, produto); c != "" {
			colunaData = normalizarNome(c)
		}
	}
	if colunaData == "" {
		colunaData = detectarColunaData(colunas)
	}
	if colunaData == "" {
		return nil, fmt.Errorf("não foi possível determinar a coluna de data — configure o Campo Data na regra do produto %q ou inclua uma coluna de data no arquivo", produto)
	}
	if !colunaExiste(colunas, colunaData) {
		return nil, fmt.Errorf("coluna de data %q (configurada na regra) não existe no arquivo", colunaData)
	}

	clones := make([]map[string]interface{}, len(registros))
	for i, reg := range registros {
		m := make(map[string]interface{}, len(reg)+2)
		for k, v := range reg {
			m[k] = v
		}
		data, err := parseDataCampo(m[colunaData])
		if err != nil {
			return nil, fmt.Errorf("linha %d: %v", i+1, err)
		}
		m["data_posicao_carteira"] = data.Format("2006-01-02")
		m["produto"] = produto
		clones[i] = m
	}
	return svc.ImportarArquivo(ctx, clones)
}

// ResultadoImportacaoProduto descreve o resultado da importação de um arquivo para um produto.
type ResultadoImportacaoProduto struct {
	Produto string                   `json:"produto"`
	Lotes   []service.LoteImportado  `json:"lotes,omitempty"`
	Erro    string                   `json:"erro,omitempty"`
}

// ResultadoImportacaoArquivo agrupa os resultados de um arquivo (um por produto casado).
type ResultadoImportacaoArquivo struct {
	Arquivo   string                       `json:"arquivo"`
	Produtos  []ResultadoImportacaoProduto `json:"produtos"`
	SemPadrao bool                         `json:"sem_padrao,omitempty"`
}

// importarArquivoPorPadrao faz o parse do arquivo, resolve o(s) produto(s) pelo nome
// (via padrões) e importa para cada produto casado. Um arquivo pode casar com vários
// padrões: nesse caso a posição é inserida para cada produto distinto.
func importarArquivoPorPadrao(ctx context.Context, svc posicaoCarteiraSvc, padraoSvc padraoArquivoResolver, reader io.Reader, filename string) ResultadoImportacaoArquivo {
	res := ResultadoImportacaoArquivo{Arquivo: filename}

	produtos, err := padraoSvc.ResolverProdutos(ctx, filename)
	if err != nil {
		res.Produtos = append(res.Produtos, ResultadoImportacaoProduto{Erro: "erro ao resolver padrões: " + err.Error()})
		return res
	}
	if len(produtos) == 0 {
		res.SemPadrao = true
		return res
	}

	registros, colunas, err := parsePosicaoArquivo(reader, filename)
	if err != nil {
		for _, p := range produtos {
			res.Produtos = append(res.Produtos, ResultadoImportacaoProduto{Produto: p, Erro: err.Error()})
		}
		return res
	}

	for _, produto := range produtos {
		lotes, err := importarConteudoParaProduto(ctx, svc, registros, colunas, produto)
		if err != nil {
			res.Produtos = append(res.Produtos, ResultadoImportacaoProduto{Produto: produto, Erro: err.Error()})
			continue
		}
		res.Produtos = append(res.Produtos, ResultadoImportacaoProduto{Produto: produto, Lotes: lotes})
	}
	return res
}

// sucesso indica se o arquivo foi importado com sucesso para todos os produtos casados.
func (r ResultadoImportacaoArquivo) sucesso() bool {
	if r.SemPadrao || len(r.Produtos) == 0 {
		return false
	}
	for _, p := range r.Produtos {
		if p.Erro != "" {
			return false
		}
	}
	return true
}
