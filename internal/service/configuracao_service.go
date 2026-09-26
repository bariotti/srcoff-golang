package service

import (
	"context"
	"strings"
)

// Chaves de configuração conhecidas.
const (
	ConfigPastaMonitorada     = "pasta_monitorada"
	ConfigPastaProcessados    = "pasta_processados"
	ConfigIntervaloScanMinutos = "intervalo_scan_minutos"
)

type configuracaoRepo interface {
	Obter(ctx context.Context, chave string) (string, error)
	Definir(ctx context.Context, chave, valor string) error
	ListarTodas(ctx context.Context) (map[string]string, error)
}

// ConfiguracaoService gerencia configurações chave→valor do sistema.
type ConfiguracaoService struct {
	repo configuracaoRepo
}

func NewConfiguracaoService(repo configuracaoRepo) *ConfiguracaoService {
	return &ConfiguracaoService{repo: repo}
}

func (s *ConfiguracaoService) Obter(ctx context.Context, chave string) (string, error) {
	return s.repo.Obter(ctx, chave)
}

func (s *ConfiguracaoService) Definir(ctx context.Context, chave, valor string) error {
	return s.repo.Definir(ctx, chave, strings.TrimSpace(valor))
}

func (s *ConfiguracaoService) ListarTodas(ctx context.Context) (map[string]string, error) {
	m, err := s.repo.ListarTodas(ctx)
	if err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]string{}
	}
	return m, nil
}
