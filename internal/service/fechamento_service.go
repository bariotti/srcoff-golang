package service

import (
	"context"
	"fmt"
	"strings"

	"srcoff/internal/model"
)

// controleFechamentoRepo é o repositório do controle de fechamento (RF-132). Implementado no
// SQLite; nos demais backends a feature fica indisponível (service não construído).
type controleFechamentoRepo interface {
	Carregar(ctx context.Context) (model.FechamentoConfig, error)
	DefinirModo(ctx context.Context, modo string) error
	AdicionarEscopo(ctx context.Context, produto, dominio string) error
	RemoverEscopo(ctx context.Context, id int64) error
	Liberar(ctx context.Context, ano, mes int, produto, dominio string) error
	RemoverLiberacao(ctx context.Context, id int64) error
}

// FechamentoService orquestra o controle de fechamento de competência: escolher o modo
// (off/todos/selecionados), gerir os escopos controlados e liberar competências por mês/ano.
// Spec: RF-132.
type FechamentoService struct{ repo controleFechamentoRepo }

func NewFechamentoService(repo controleFechamentoRepo) *FechamentoService {
	return &FechamentoService{repo: repo}
}

// Estado devolve o estado completo do controle para a tela.
func (s *FechamentoService) Estado(ctx context.Context) (model.FechamentoConfig, error) {
	return s.repo.Carregar(ctx)
}

// DefinirModo valida e grava o modo do controle.
func (s *FechamentoService) DefinirModo(ctx context.Context, modo string) error {
	modo = strings.TrimSpace(modo)
	switch modo {
	case model.FechamentoOff, model.FechamentoTodos, model.FechamentoSelecionados:
		return s.repo.DefinirModo(ctx, modo)
	default:
		return fmt.Errorf("modo inválido (use off, todos ou selecionados)")
	}
}

// AdicionarEscopo inclui um produto/domínio na lista controlada (modo selecionados). Pelo menos
// um dos dois deve ser informado — um escopo totalmente vazio equivaleria a "todos".
func (s *FechamentoService) AdicionarEscopo(ctx context.Context, produto, dominio string) error {
	produto, dominio = strings.TrimSpace(produto), strings.TrimSpace(dominio)
	if produto == "" && dominio == "" {
		return fmt.Errorf("informe ao menos o produto ou o domínio (vazio = todos: use o modo \"todos\")")
	}
	return s.repo.AdicionarEscopo(ctx, produto, dominio)
}

func (s *FechamentoService) RemoverEscopo(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id do escopo é obrigatório")
	}
	return s.repo.RemoverEscopo(ctx, id)
}

// Liberar registra o OK de uma competência (mês/ano) para um escopo (vazio = todos).
func (s *FechamentoService) Liberar(ctx context.Context, ano, mes int, produto, dominio string) error {
	if mes < 1 || mes > 12 {
		return fmt.Errorf("mês inválido (1 a 12)")
	}
	if ano < 2000 || ano > 2100 {
		return fmt.Errorf("ano inválido")
	}
	return s.repo.Liberar(ctx, ano, mes, strings.TrimSpace(produto), strings.TrimSpace(dominio))
}

func (s *FechamentoService) RemoverLiberacao(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id da liberação é obrigatório")
	}
	return s.repo.RemoverLiberacao(ctx, id)
}
