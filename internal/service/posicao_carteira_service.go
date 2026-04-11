package service

import (
	"context"
	"fmt"
	"time"

	"srcoff/internal/model"
)

type posicaoCarteiraRepoFull interface {
	posicaoCarteiraRepo
	ListarPorData(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error)
	Inserir(ctx context.Context, p model.PosicaoCarteira) (int64, error)
	Deletar(ctx context.Context, id int64) error
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

func (s *PosicaoCarteiraService) Inserir(ctx context.Context, p model.PosicaoCarteira) (int64, error) {
	if p.CodigoIdentificadorBoleto == "" {
		return 0, fmt.Errorf("campo obrigatório ausente: codigo_identificador_boleto")
	}
	if p.DataPosicaoCarteira.IsZero() {
		return 0, fmt.Errorf("campo obrigatório ausente: data_posicao_carteira")
	}
	if p.CodigoVersaoConteudo == 0 {
		p.CodigoVersaoConteudo = 1
	}
	return s.repo.Inserir(ctx, p)
}

func (s *PosicaoCarteiraService) Deletar(ctx context.Context, id int64) error {
	if id == 0 {
		return fmt.Errorf("id inválido")
	}
	return s.repo.Deletar(ctx, id)
}
