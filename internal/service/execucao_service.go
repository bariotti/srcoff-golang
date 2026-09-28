package service

import (
	"context"
	"strings"
	"time"

	"srcoff/internal/model"
)

type execucaoRepoReader interface {
	ListarPorData(ctx context.Context, data time.Time) ([]model.MovimentoExecucao, error)
	RegistrarExecucao(ctx context.Context, e model.MovimentoExecucao) error
}

type padroesParaEscopo interface {
	Listar(ctx context.Context) ([]model.PadraoArquivo, error)
}

// StatusCombinacao descreve o status de processamento de uma combinação para uma data.
type StatusCombinacao struct {
	Produto        string `json:"produto"`
	Dominio        string `json:"dominio"`
	Processado     bool   `json:"processado"`
	QtdLancamentos int    `json:"qtd_lancamentos"`
	QtdEstornos    int    `json:"qtd_estornos"`
}

// ExecucaoService monta a visão de processados/pendentes por data.
type ExecucaoService struct {
	execRepo   execucaoRepoReader
	padraoRepo padroesParaEscopo
}

func NewExecucaoService(execRepo execucaoRepoReader, padraoRepo padroesParaEscopo) *ExecucaoService {
	return &ExecucaoService{execRepo: execRepo, padraoRepo: padraoRepo}
}

// StatusPorData retorna, para a data, as combinações (produto, domínio) definidas
// nos padrões de arquivo cadastrados em Parametrizações e o status de processamento
// de cada uma (processado/pendente).
func (s *ExecucaoService) StatusPorData(ctx context.Context, data time.Time) ([]StatusCombinacao, error) {
	padroes, err := s.padraoRepo.Listar(ctx)
	if err != nil {
		return nil, err
	}
	// Combinações esperadas = pares (produto, domínio) distintos dos padrões de arquivo.
	esperadas := []model.ProdutoDominio{}
	visto := map[string]bool{}
	for _, p := range padroes {
		produto := strings.TrimSpace(p.Produto)
		dominio := strings.TrimSpace(p.Dominio)
		k := produto + "\x00" + dominio
		if !visto[k] {
			visto[k] = true
			esperadas = append(esperadas, model.ProdutoDominio{Produto: produto, Dominio: dominio})
		}
	}

	execs, err := s.execRepo.ListarPorData(ctx, data)
	if err != nil {
		return nil, err
	}
	porCombo := map[string]model.MovimentoExecucao{}
	for _, e := range execs {
		porCombo[e.Produto+"\x00"+e.Dominio] = e
	}

	var status []StatusCombinacao
	for _, c := range esperadas {
		e, ok := porCombo[c.Produto+"\x00"+c.Dominio]
		status = append(status, StatusCombinacao{
			Produto:        c.Produto,
			Dominio:        c.Dominio,
			Processado:     ok,
			QtdLancamentos: e.QtdLancamentos,
			QtdEstornos:    e.QtdEstornos,
		})
	}
	return status, nil
}
