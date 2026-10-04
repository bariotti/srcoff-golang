package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"srcoff/internal/model"
)

type posicaoCarteiraRepoFull interface {
	posicaoCarteiraRepo
	ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error)
	ListarPorPeriodo(ctx context.Context, dataInicio, dataFim time.Time) ([]model.PosicaoCarteira, error)
	ImportarLote(ctx context.Context, data time.Time, versao int, registros []map[string]interface{}) error
}

type PosicaoCarteiraService struct {
	repo posicaoCarteiraRepoFull
}

func NewPosicaoCarteiraService(repo posicaoCarteiraRepoFull) *PosicaoCarteiraService {
	return &PosicaoCarteiraService{repo: repo}
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
		// Um upload corresponde a um produto+domínio; a versão é por (data, produto,
		// domínio) para que combinações distintas da mesma data coexistam no vigente.
		produto, _ := grupos[d][0]["produto"].(string)
		dominio, _ := grupos[d][0]["dominio"].(string)
		versao, err := s.proximaVersao(ctx, data, produto, dominio)
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
func (s *PosicaoCarteiraService) proximaVersao(ctx context.Context, data time.Time, produto, dominio string) (int, error) {
	existentes, err := s.repo.ListarPorData(ctx, data)
	if err != nil {
		return 0, err
	}
	max := 0
	for _, p := range existentes {
		if produtoDaPosicao(p) != produto || dominioDaPosicao(p) != dominio {
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
	return campoStrModel(p, "produto")
}

// dominioDaPosicao lê o campo `dominio` de uma posição.
func dominioDaPosicao(p model.PosicaoCarteira) string {
	return campoStrModel(p, "dominio")
}

func campoStrModel(p model.PosicaoCarteira, campo string) string {
	if v, ok := p.Campos[campo]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
