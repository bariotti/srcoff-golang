package service

import (
	"context"

	"srcoff/internal/model"
)

type notificacaoRepo interface {
	Criar(ctx context.Context, n model.Notificacao) (int64, error)
	Listar(ctx context.Context, limite int) ([]model.Notificacao, error)
	ContarNaoLidas(ctx context.Context) (int, error)
	MarcarTodasLidas(ctx context.Context) error
}

// NotificacaoService gerencia as notificações de eventos automáticos.
type NotificacaoService struct {
	repo notificacaoRepo
}

func NewNotificacaoService(repo notificacaoRepo) *NotificacaoService {
	return &NotificacaoService{repo: repo}
}

func (s *NotificacaoService) Criar(ctx context.Context, n model.Notificacao) (int64, error) {
	return s.repo.Criar(ctx, n)
}

func (s *NotificacaoService) Listar(ctx context.Context, limite int) ([]model.Notificacao, error) {
	return s.repo.Listar(ctx, limite)
}

func (s *NotificacaoService) ContarNaoLidas(ctx context.Context) (int, error) {
	return s.repo.ContarNaoLidas(ctx)
}

func (s *NotificacaoService) MarcarTodasLidas(ctx context.Context) error {
	return s.repo.MarcarTodasLidas(ctx)
}
