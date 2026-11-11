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
type FechamentoService struct {
	repo      controleFechamentoRepo
	auditoria *AuditoriaService // opcional: trilha de auditoria (RF-136)
}

func NewFechamentoService(repo controleFechamentoRepo) *FechamentoService {
	return &FechamentoService{repo: repo}
}

// ComAuditoria liga a trilha de auditoria global (RF-136) ao controle de fechamento.
func (s *FechamentoService) ComAuditoria(a *AuditoriaService) *FechamentoService {
	s.auditoria = a
	return s
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
		if err := s.repo.DefinirModo(ctx, modo); err != nil {
			return err
		}
		s.auditoria.Log(ctx, model.AudFechamento, "fechamento.modo", "controle de fechamento", "sistema", "modo definido para "+modo,
			map[string]interface{}{"modo": modo})
		return nil
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
	if err := s.repo.AdicionarEscopo(ctx, produto, dominio); err != nil {
		return err
	}
	s.auditoria.Log(ctx, model.AudFechamento, "fechamento.escopo.adicionado", escopoRotulo(produto, dominio), "sistema", "escopo incluído no controle de fechamento",
		map[string]interface{}{"produto": produto, "dominio": dominio})
	return nil
}

func (s *FechamentoService) RemoverEscopo(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id do escopo é obrigatório")
	}
	if err := s.repo.RemoverEscopo(ctx, id); err != nil {
		return err
	}
	s.auditoria.Log(ctx, model.AudFechamento, "fechamento.escopo.removido", fmt.Sprintf("escopo #%d", id), "sistema", "escopo removido do controle de fechamento",
		map[string]interface{}{"id": id})
	return nil
}

// Liberar registra o OK de uma competência (mês/ano) para um escopo (vazio = todos).
func (s *FechamentoService) Liberar(ctx context.Context, ano, mes int, produto, dominio string) error {
	if mes < 1 || mes > 12 {
		return fmt.Errorf("mês inválido (1 a 12)")
	}
	if ano < 2000 || ano > 2100 {
		return fmt.Errorf("ano inválido")
	}
	produto, dominio = strings.TrimSpace(produto), strings.TrimSpace(dominio)
	if err := s.repo.Liberar(ctx, ano, mes, produto, dominio); err != nil {
		return err
	}
	s.auditoria.Log(ctx, model.AudLiberacao, "competencia.liberada",
		fmt.Sprintf("%04d-%02d · %s", ano, mes, escopoRotulo(produto, dominio)), "sistema",
		fmt.Sprintf("competência %04d-%02d liberada", ano, mes),
		map[string]interface{}{"ano": ano, "mes": mes, "produto": produto, "dominio": dominio})
	return nil
}

func (s *FechamentoService) RemoverLiberacao(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id da liberação é obrigatório")
	}
	if err := s.repo.RemoverLiberacao(ctx, id); err != nil {
		return err
	}
	s.auditoria.Log(ctx, model.AudLiberacao, "competencia.liberacao_removida", fmt.Sprintf("liberação #%d", id), "sistema", "liberação de competência removida",
		map[string]interface{}{"id": id})
	return nil
}

// escopoRotulo monta um rótulo curto para um escopo produto/domínio (vazio = "todos").
func escopoRotulo(produto, dominio string) string {
	p, d := strings.TrimSpace(produto), strings.TrimSpace(dominio)
	if p == "" {
		p = "todos"
	}
	if d == "" {
		d = "todos"
	}
	return p + "/" + d
}
