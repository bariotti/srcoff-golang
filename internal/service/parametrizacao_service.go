package service

import (
	"context"
	"fmt"
	"strings"
)

// Categorias de parametrização suportadas atualmente.
const (
	CategoriaProduto = "produto"
	CategoriaDominio = "dominio"
)

type parametrizacaoRepo interface {
	ListarOpcoes(ctx context.Context, categoria string) ([]string, error)
	AdicionarOpcao(ctx context.Context, categoria, valor string) error
	RemoverOpcao(ctx context.Context, categoria, valor string) error
}

// ParametrizacaoService gerencia as opções parametrizáveis usadas nos combos do sistema.
type ParametrizacaoService struct {
	repo parametrizacaoRepo
}

func NewParametrizacaoService(repo parametrizacaoRepo) *ParametrizacaoService {
	return &ParametrizacaoService{repo: repo}
}

func categoriaValida(categoria string) bool {
	switch categoria {
	case CategoriaProduto, CategoriaDominio:
		return true
	}
	return false
}

func (s *ParametrizacaoService) ListarOpcoes(ctx context.Context, categoria string) ([]string, error) {
	if !categoriaValida(categoria) {
		return nil, fmt.Errorf("categoria inválida: %q", categoria)
	}
	return s.repo.ListarOpcoes(ctx, categoria)
}

func (s *ParametrizacaoService) AdicionarOpcao(ctx context.Context, categoria, valor string) error {
	if !categoriaValida(categoria) {
		return fmt.Errorf("categoria inválida: %q", categoria)
	}
	valor = strings.TrimSpace(valor)
	if valor == "" {
		return fmt.Errorf("valor é obrigatório")
	}
	return s.repo.AdicionarOpcao(ctx, categoria, valor)
}

func (s *ParametrizacaoService) RemoverOpcao(ctx context.Context, categoria, valor string) error {
	if !categoriaValida(categoria) {
		return fmt.Errorf("categoria inválida: %q", categoria)
	}
	if strings.TrimSpace(valor) == "" {
		return fmt.Errorf("valor é obrigatório")
	}
	return s.repo.RemoverOpcao(ctx, categoria, valor)
}
