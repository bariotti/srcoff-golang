package service

import (
	"context"
	"time"

	"srcoff/internal/model"
)

type inconsistenciaRepoReader interface {
	ListarPorData(ctx context.Context, data time.Time) ([]model.InconsistenciaProcessamento, error)
}

// InconsistenciaService expõe a consulta das inconsistências persistidas.
type InconsistenciaService struct {
	repo inconsistenciaRepoReader
}

func NewInconsistenciaService(repo inconsistenciaRepoReader) *InconsistenciaService {
	return &InconsistenciaService{repo: repo}
}

func (s *InconsistenciaService) ListarPorData(ctx context.Context, data time.Time) ([]model.InconsistenciaProcessamento, error) {
	return s.repo.ListarPorData(ctx, data)
}
