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
	if p.Padrao == "" {
		return 0, fmt.Errorf("informe o padrão do arquivo (ex: posicao_ndf*.csv)")
	}
	if p.Produto == "" {
		return 0, fmt.Errorf("informe o produto do padrão")
	}
	return s.repo.Criar(ctx, p)
}

func (s *PadraoArquivoService) Excluir(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id inválido")
	}
	return s.repo.Excluir(ctx, id)
}

// ResolverProdutos retorna os produtos cujos padrões casam com o nome do arquivo.
// Um mesmo arquivo pode casar com mais de um padrão; nesse caso todos os produtos
// (distintos) são retornados, preservando a ordem de cadastro.
func (s *PadraoArquivoService) ResolverProdutos(ctx context.Context, nomeArquivo string) ([]string, error) {
	padroes, err := s.repo.Listar(ctx)
	if err != nil {
		return nil, err
	}
	base := strings.ToLower(filepath.Base(nomeArquivo))
	visto := map[string]bool{}
	var produtos []string
	for _, p := range padroes {
		ok, err := path.Match(strings.ToLower(p.Padrao), base)
		if err != nil || !ok {
			continue
		}
		if !visto[p.Produto] {
			visto[p.Produto] = true
			produtos = append(produtos, p.Produto)
		}
	}
	return produtos, nil
}
