package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"srcoff/internal/model"
)

type posicaoCarteiraRepoFull interface {
	posicaoCarteiraRepo
	ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error)
	ListarPorPeriodo(ctx context.Context, dataInicio, dataFim time.Time) ([]model.PosicaoCarteira, error)
	ImportarLote(ctx context.Context, data time.Time, versao int, registros []map[string]interface{}) error
}

// regraLookup permite ao serviço de posição descobrir configurações das regras
// (ex: qual campo da posição contém a data) a partir do produto informado no upload.
type regraLookup interface {
	ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error)
}

type PosicaoCarteiraService struct {
	repo   posicaoCarteiraRepoFull
	regras regraLookup
}

func NewPosicaoCarteiraService(repo posicaoCarteiraRepoFull, regras regraLookup) *PosicaoCarteiraService {
	return &PosicaoCarteiraService{repo: repo, regras: regras}
}

// ResolverCampoData retorna o nome do campo da posição que contém a data, conforme
// configurado (campo_data) na regra do produto informado. Retorna "" se não houver
// regra/produto correspondente com campo_data definido — nesse caso o chamador deve
// recorrer à detecção automática.
func (s *PosicaoCarteiraService) ResolverCampoData(ctx context.Context, produto string) (string, error) {
	if s.regras == nil {
		return "", nil
	}
	regras, err := s.regras.ListarRegrasAtivas(ctx)
	if err != nil {
		return "", err
	}
	produto = strings.TrimSpace(produto)
	// 1ª preferência: regra cujo produto casa e que define campo_data.
	for _, r := range regras {
		if strings.TrimSpace(r.CampoData) == "" {
			continue
		}
		if produtoCasaComRegra(r.CodigoProdutoCorporativo, produto) {
			return strings.TrimSpace(r.CampoData), nil
		}
	}
	return "", nil
}

// produtoCasaComRegra verifica se o produto informado está na lista de produtos da
// regra (separada por vírgula). Regra sem produto casa com qualquer um.
func produtoCasaComRegra(produtosRegra, produto string) bool {
	if strings.TrimSpace(produtosRegra) == "" {
		return true
	}
	for _, p := range strings.Split(produtosRegra, ",") {
		if strings.TrimSpace(p) == produto {
			return true
		}
	}
	return false
}

func (s *PosicaoCarteiraService) ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	return s.repo.ListarPorData(ctx, data)
}

func (s *PosicaoCarteiraService) ListarPorPeriodo(ctx context.Context, dataInicio, dataFim time.Time) ([]model.PosicaoCarteira, error) {
	return s.repo.ListarPorPeriodo(ctx, dataInicio, dataFim)
}

// LoteImportado descreve um lote de posição persistido durante a importação.
type LoteImportado struct {
	Data   string `json:"data"`
	Versao int    `json:"versao"`
	Total  int    `json:"total"`
}

// ImportarArquivo persiste a posição de carteira importada de um arquivo (CSV/XLSX).
// A data de cada registro vem do próprio arquivo (campo data_posicao_carteira, já
// resolvido pelo handler), então os registros são agrupados por data e cada grupo
// recebe automaticamente a próxima versão disponível para sua data.
func (s *PosicaoCarteiraService) ImportarArquivo(ctx context.Context, registros []map[string]interface{}) ([]LoteImportado, error) {
	if len(registros) == 0 {
		return nil, fmt.Errorf("nenhum registro encontrado no arquivo")
	}

	grupos := map[string][]map[string]interface{}{}
	var ordem []string
	for _, r := range registros {
		d, _ := r["data_posicao_carteira"].(string)
		if d == "" {
			return nil, fmt.Errorf("registro sem data de posição")
		}
		if _, ok := grupos[d]; !ok {
			ordem = append(ordem, d)
		}
		grupos[d] = append(grupos[d], r)
	}

	var lotes []LoteImportado
	for _, d := range ordem {
		data, err := time.Parse("2006-01-02", d)
		if err != nil {
			return nil, fmt.Errorf("data inválida no arquivo: %q", d)
		}
		// Um upload corresponde a um produto; a versão é por (data, produto) para
		// que produtos distintos da mesma data coexistam no snapshot vigente.
		produto, _ := grupos[d][0]["produto"].(string)
		versao, err := s.proximaVersao(ctx, data, produto)
		if err != nil {
			return nil, err
		}
		if err := s.repo.ImportarLote(ctx, data, versao, grupos[d]); err != nil {
			return nil, err
		}
		lotes = append(lotes, LoteImportado{Data: d, Versao: versao, Total: len(grupos[d])})
	}
	return lotes, nil
}

// CamposDisponiveis retorna os nomes de campos presentes no último lote (versão
// vigente) de posição para a data informada — usado para sugerir os campos no
// cadastro de regras/condições. Se não houver data ou registros, retorna vazio.
func (s *PosicaoCarteiraService) CamposDisponiveis(ctx context.Context, data time.Time) ([]string, error) {
	posicoes, err := s.repo.BuscarPorDataEVersaoMaxima(ctx, data)
	if err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	for _, p := range posicoes {
		for k := range p.Campos {
			set[k] = struct{}{}
		}
	}
	campos := make([]string, 0, len(set))
	for k := range set {
		campos = append(campos, k)
	}
	sort.Strings(campos)
	return campos, nil
}

// proximaVersao calcula a próxima versão disponível para a combinação (data, produto),
// preservando as versões anteriores desse mesmo produto. Produtos distintos versionam
// de forma independente, permitindo que coexistam no snapshot vigente da data.
func (s *PosicaoCarteiraService) proximaVersao(ctx context.Context, data time.Time, produto string) (int, error) {
	existentes, err := s.repo.ListarPorData(ctx, data)
	if err != nil {
		return 0, err
	}
	max := 0
	for _, p := range existentes {
		if produtoDaPosicao(p) != produto {
			continue
		}
		if p.CodigoVersaoConteudo > max {
			max = p.CodigoVersaoConteudo
		}
	}
	return max + 1, nil
}

// produtoDaPosicao lê o campo `produto` de uma posição.
func produtoDaPosicao(p model.PosicaoCarteira) string {
	if v, ok := p.Campos["produto"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
