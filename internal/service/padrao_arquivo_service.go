package service

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"srcoff/internal/model"
)

type padraoArquivoRepo interface {
	Listar(ctx context.Context) ([]model.PadraoArquivo, error)
	Criar(ctx context.Context, p model.PadraoArquivo) (int64, error)
	Excluir(ctx context.Context, id int64) error
}

// PadraoArquivoService gerencia os padrões de nome de arquivo → produto e resolve
// o(s) produto(s) de um arquivo pelo seu nome.
type PadraoArquivoService struct {
	repo padraoArquivoRepo
}

func NewPadraoArquivoService(repo padraoArquivoRepo) *PadraoArquivoService {
	return &PadraoArquivoService{repo: repo}
}

func (s *PadraoArquivoService) Listar(ctx context.Context) ([]model.PadraoArquivo, error) {
	return s.repo.Listar(ctx)
}

func (s *PadraoArquivoService) Criar(ctx context.Context, p model.PadraoArquivo) (int64, error) {
	p.Padrao = strings.TrimSpace(p.Padrao)
	p.Produto = strings.TrimSpace(p.Produto)
	p.Dominio = strings.TrimSpace(p.Dominio)
	p.FormatoData = strings.TrimSpace(p.FormatoData)
	if p.Padrao == "" {
		return 0, fmt.Errorf("informe o padrão do arquivo (ex: posicao_ndf*.csv)")
	}
	if p.Produto == "" {
		return 0, fmt.Errorf("informe o produto do padrão")
	}
	if p.Dominio == "" {
		return 0, fmt.Errorf("informe o domínio do padrão")
	}
	if p.FormatoData == "" {
		return 0, fmt.Errorf("informe o formato de data do arquivo")
	}
	return s.repo.Criar(ctx, p)
}

// ProdutoDominio identifica o par produto+domínio resolvido de um arquivo.
type ProdutoDominio = model.ProdutoDominio

func (s *PadraoArquivoService) Excluir(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id inválido")
	}
	return s.repo.Excluir(ctx, id)
}

// ResolverPadroes retorna os padrões cujo nome casa com o arquivo, na ordem de
// cadastro. Um mesmo arquivo pode casar com mais de um padrão. Cada padrão carrega
// o produto, o domínio e os parâmetros de parsing (delimitador/decimal/milhar).
func (s *PadraoArquivoService) ResolverPadroes(ctx context.Context, nomeArquivo string) ([]model.PadraoArquivo, error) {
	padroes, err := s.repo.Listar(ctx)
	if err != nil {
		return nil, err
	}
	base := strings.ToLower(filepath.Base(nomeArquivo))
	var casados []model.PadraoArquivo
	for _, p := range padroes {
		ok, err := path.Match(strings.ToLower(p.Padrao), base)
		if err != nil || !ok {
			continue
		}
		casados = append(casados, p)
	}
	return casados, nil
}
