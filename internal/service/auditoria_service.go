package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"srcoff/internal/model"
)

type auditoriaRepo interface {
	Inserir(ctx context.Context, ev model.EventoAuditoria) error
	Listar(ctx context.Context, f model.FiltroAuditoria) (*model.PaginaAuditoria, error)
}

// AuditoriaService centraliza a trilha de auditoria global (RF-136). Emite SEMPRE uma linha de
// log estruturada em JSON e, quando há backend de persistência (sqlite-first), grava o evento
// para consulta/exportação. Os métodos são nil-safe e best-effort: auditar nunca falha a ação.
type AuditoriaService struct {
	repo auditoriaRepo // nil em file/sqlserver → só a linha de log JSON, sem consulta
}

func NewAuditoriaService(repo auditoriaRepo) *AuditoriaService {
	return &AuditoriaService{repo: repo}
}

// Disponivel informa se a consulta/exportação da trilha está ligada neste backend.
func (s *AuditoriaService) Disponivel() bool { return s != nil && s.repo != nil }

// Registrar emite a linha de log JSON e persiste o evento quando há backend. Nil-safe.
func (s *AuditoriaService) Registrar(ctx context.Context, ev model.EventoAuditoria) {
	if s == nil {
		return
	}
	if ev.CriadoEm == "" {
		ev.CriadoEm = time.Now().Format("2006-01-02 15:04:05")
	}
	if ev.Origem == "" {
		ev.Origem = "sistema"
	}
	// Linha de log ESTRUTURADA em JSON — sempre, mesmo sem backend de consulta.
	if b, err := json.Marshal(ev); err == nil {
		log.Printf("[auditoria] %s", string(b))
	}
	if s.repo != nil {
		if err := s.repo.Inserir(ctx, ev); err != nil {
			log.Printf("[auditoria] falha ao persistir evento (%s/%s): %v", ev.Categoria, ev.Acao, err)
		}
	}
}

// Log monta e registra um evento (atalho).
func (s *AuditoriaService) Log(ctx context.Context, categoria, acao, entidade, origem, detalhe string, dados map[string]interface{}) {
	s.Registrar(ctx, model.EventoAuditoria{
		Categoria: categoria, Acao: acao, Entidade: entidade, Origem: origem, Detalhe: detalhe, Dados: dados,
	})
}

// Listar consulta a trilha (paginada, filtrável). Erro quando a feature está off.
func (s *AuditoriaService) Listar(ctx context.Context, f model.FiltroAuditoria) (*model.PaginaAuditoria, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("auditoria indisponível neste backend (disponível apenas no SQLite)")
	}
	return s.repo.Listar(ctx, f)
}
