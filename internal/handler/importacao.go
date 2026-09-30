package handler

import (
	"context"
	"fmt"
	"io"

	"srcoff/internal/model"
	"srcoff/internal/service"
)

// padraoArquivoResolver resolve os padrões (produto, domínio, config de parsing) de
// um arquivo pelo seu nome.
type padraoArquivoResolver interface {
	ResolverPadroes(ctx context.Context, nomeArquivo string) ([]model.PadraoArquivo, error)
}

// layoutsFormatoData mapeia o rótulo escolhido no padrão de arquivo para o layout
// de referência do Go (Mon Jan 2 2006). Aplica-se a todas as colunas de data do arquivo.
var layoutsFormatoData = map[string]string{
	"AAAA/MM/DD": "2006/01/02",
	"AAAA-MM-DD": "2006-01-02",
	"DD/MM/AAAA": "02/01/2006",
	"DD-MM-AAAA": "02-01-2006",
	"AAAA/DD/MM": "2006/02/01",
	"AAAA-DD-MM": "2006-02-01",
	"MM/DD/AAAA": "01/02/2006",
	"MM-DD-AAAA": "01-02-2006",
}

// configDoPadrao converte os separadores textuais e o formato de data do padrão em parseConfig.
func configDoPadrao(p model.PadraoArquivo) parseConfig {
	cfg := parseConfig{
		sepDecimal:  p.SeparadorDecimal,
		sepMilhar:   p.SeparadorMilhar,
		formatoData: layoutsFormatoData[p.FormatoData], // rótulo desconhecido/vazio → "" (automático)
	}
	switch p.Delimitador {
	case ";":
		cfg.delimitador = ';'
	case ",":
		cfg.delimitador = ','
	}
	return cfg
}

// importarConteudoParaProduto resolve a coluna de data (via campo_data da regra do
// produto ou detecção), preenche data, produto e domínio em cada registro e importa
// o lote. Cada combinação recebe uma cópia dos registros para não compartilhar mutações.
func importarConteudoParaProduto(ctx context.Context, svc posicaoCarteiraSvc, registros []map[string]interface{}, colunas []string, produto, dominio, colunaDataPadrao string) ([]service.LoteImportado, error) {
	// Prioridade: coluna de data informada no padrão de arquivo; senão (legado),
	// resolve pela regra do produto ou tenta detectar automaticamente.
	colunaData := ""
	if colunaDataPadrao != "" {
		colunaData = normalizarNome(colunaDataPadrao)
	}
	if colunaData == "" && produto != "" {
		if c, _ := svc.ResolverCampoData(ctx, produto); c != "" {
			colunaData = normalizarNome(c)
		}
	}
	if colunaData == "" {
		colunaData = detectarColunaData(colunas)
	}
	if colunaData == "" {
		return nil, fmt.Errorf("não foi possível determinar a coluna de data — informe a coluna de data base no padrão de arquivo do produto %q", produto)
	}
	if !colunaExiste(colunas, colunaData) {
		return nil, fmt.Errorf("coluna de data base %q (do padrão de arquivo) não existe no arquivo", colunaData)
	}

	clones := make([]map[string]interface{}, len(registros))
	for i, reg := range registros {
		m := make(map[string]interface{}, len(reg)+3)
		for k, v := range reg {
			m[k] = v
		}
		data, err := parseDataCampo(m[colunaData])
		if err != nil {
			return nil, fmt.Errorf("linha %d: %v", i+1, err)
		}
		if !service.EhDiaUtil(data) {
			return nil, fmt.Errorf("linha %d: data base %s não é dia útil (%s); a posição só pode ser importada em dias úteis", i+1, data.Format("2006-01-02"), service.DescricaoDiasNaoUteis)
		}
		m["data_posicao_carteira"] = data.Format("2006-01-02")
		m["produto"] = produto
		m["dominio"] = dominio
		clones[i] = m
	}
	return svc.ImportarArquivo(ctx, clones)
}

// ResultadoImportacaoProduto descreve o resultado da importação de um arquivo para uma combinação produto+domínio.
type ResultadoImportacaoProduto struct {
	Produto string                  `json:"produto"`
	Dominio string                  `json:"dominio"`
	Lotes   []service.LoteImportado `json:"lotes,omitempty"`
	Erro    string                  `json:"erro,omitempty"`
}

// ResultadoImportacaoArquivo agrupa os resultados de um arquivo (um por combinação casada).
type ResultadoImportacaoArquivo struct {
	Arquivo   string                       `json:"arquivo"`
	Produtos  []ResultadoImportacaoProduto `json:"produtos"`
	SemPadrao bool                         `json:"sem_padrao,omitempty"`
}

// importarArquivoPorPadrao faz o parse do arquivo, resolve o(s) par(es) produto+domínio
// pelo nome (via padrões) e importa para cada um. Um arquivo pode casar com vários
// padrões: nesse caso a posição é inserida para cada combinação distinta.
func importarArquivoPorPadrao(ctx context.Context, svc posicaoCarteiraSvc, padraoSvc padraoArquivoResolver, reader io.Reader, filename string) ResultadoImportacaoArquivo {
	res := ResultadoImportacaoArquivo{Arquivo: filename}

	padroes, err := padraoSvc.ResolverPadroes(ctx, filename)
	if err != nil {
		res.Produtos = append(res.Produtos, ResultadoImportacaoProduto{Erro: "erro ao resolver padrões: " + err.Error()})
		return res
	}
	if len(padroes) == 0 {
		res.SemPadrao = true
		return res
	}

	// A configuração de parsing vem do primeiro padrão casado (formato do arquivo é único).
	cfg := configDoPadrao(padroes[0])
	registros, colunas, err := parsePosicaoArquivoCfg(reader, filename, cfg)
	if err != nil {
		for _, pd := range padroes {
			res.Produtos = append(res.Produtos, ResultadoImportacaoProduto{Produto: pd.Produto, Dominio: pd.Dominio, Erro: err.Error()})
		}
		return res
	}

	// Importa para cada combinação (produto, domínio) distinta dos padrões casados.
	visto := map[string]bool{}
	for _, pd := range padroes {
		chave := pd.Produto + "\x00" + pd.Dominio
		if visto[chave] {
			continue
		}
		visto[chave] = true
		lotes, err := importarConteudoParaProduto(ctx, svc, registros, colunas, pd.Produto, pd.Dominio, pd.ColunaData)
		if err != nil {
			res.Produtos = append(res.Produtos, ResultadoImportacaoProduto{Produto: pd.Produto, Dominio: pd.Dominio, Erro: err.Error()})
			continue
		}
		res.Produtos = append(res.Produtos, ResultadoImportacaoProduto{Produto: pd.Produto, Dominio: pd.Dominio, Lotes: lotes})
	}
	return res
}

// combosImportados retorna as combinações (produto, domínio) importadas com sucesso.
func (r ResultadoImportacaoArquivo) combosImportados() []service.ProdutoDominio {
	var combos []service.ProdutoDominio
	for _, p := range r.Produtos {
		if p.Erro == "" {
			combos = append(combos, service.ProdutoDominio{Produto: p.Produto, Dominio: p.Dominio})
		}
	}
	return combos
}

// sucesso indica se o arquivo foi importado com sucesso para todas as combinações casadas.
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
