package service

import (
	"context"
	"fmt"
	"strings"

	"srcoff/internal/model"
	"srcoff/internal/repository"
)

// CotacaoService gerencia o cadastro de cotações (taxa de conversão entre moedas por data).
// Todos os campos são obrigatórios. Spec: RF-037/RN-183.
type CotacaoService struct {
	repo repository.CotacaoRepository
}

func NewCotacaoService(repo repository.CotacaoRepository) *CotacaoService {
	return &CotacaoService{repo: repo}
}

func (s *CotacaoService) Listar(ctx context.Context) ([]model.Cotacao, error) { return s.repo.Listar(ctx) }

// Criar valida e insere uma cotação. Data, Moeda1, Moeda2, Direção e Taxa são obrigatórios.
func (s *CotacaoService) Criar(ctx context.Context, c model.Cotacao) (int64, error) {
	c.Moeda1 = strings.TrimSpace(c.Moeda1)
	c.Moeda2 = strings.TrimSpace(c.Moeda2)
	c.Direcao = strings.ToLower(strings.TrimSpace(c.Direcao))
	if c.Data.IsZero() {
		return 0, fmt.Errorf("informe a data da cotação")
	}
	if c.Moeda1 == "" || c.Moeda2 == "" {
		return 0, fmt.Errorf("informe as duas moedas da cotação")
	}
	if c.Direcao != model.DirecaoMultiplicar && c.Direcao != model.DirecaoDividir {
		return 0, fmt.Errorf("direção inválida: use %q ou %q", model.DirecaoMultiplicar, model.DirecaoDividir)
	}
	if c.Taxa <= 0 {
		return 0, fmt.Errorf("informe uma taxa de conversão maior que zero")
	}
	return s.repo.Criar(ctx, c)
}

func (s *CotacaoService) Excluir(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id inválido")
	}
	return s.repo.Excluir(ctx, id)
}

// chaveCotacao normaliza (data ISO, moeda1, moeda2) para lookup case-insensitive.
func chaveCotacao(dataISO, moeda1, moeda2 string) string {
	return dataISO + "\x00" + strings.ToUpper(strings.TrimSpace(moeda1)) + "\x00" + strings.ToUpper(strings.TrimSpace(moeda2))
}

// MapaCotacoes devolve as cotações indexadas por (data ISO, moeda1, moeda2), para a geração.
func (s *CotacaoService) MapaCotacoes(ctx context.Context) (map[string]model.Cotacao, error) {
	itens, err := s.repo.Listar(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]model.Cotacao, len(itens))
	for _, c := range itens {
		m[chaveCotacao(c.Data.Format("2006-01-02"), c.Moeda1, c.Moeda2)] = c
	}
	return m, nil
}

// aplicarCotacao converte o valor conforme a direção da cotação.
func aplicarCotacao(valor float64, c model.Cotacao) float64 {
	if c.Direcao == model.DirecaoDividir {
		if c.Taxa == 0 {
			return 0
		}
		return valor / c.Taxa
	}
	return valor * c.Taxa
}
