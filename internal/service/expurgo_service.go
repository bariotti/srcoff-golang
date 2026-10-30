package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Expurgo/retenção de movimentos (RF-123). Remove os movimentos contábeis — e as
// inconsistências/execuções das mesmas datas — cuja data de lote seja <= hoje − N dias,
// onde N é o parâmetro de retenção. É uma operação DESTRUTIVA e IRREVERSÍVEL, executada
// por um job recorrente (ver cmd/api); N<=0 desativa (nada é apagado).

// Chaves de configuração do expurgo.
const (
	// ConfigRetencaoDiasMovimento: dias de movimento a RETER. 0/vazio = desativado.
	ConfigRetencaoDiasMovimento = "retencao_dias_movimento"
	// ConfigRetencaoIntervaloHoras: periodicidade do job de expurgo (default 24h).
	ConfigRetencaoIntervaloHoras = "retencao_intervalo_horas"
)

// ExpurgavelPorData é um repositório que sabe apagar seus registros até uma data de corte
// (inclusive). Implementado pelos repositórios SQLite de movimento, inconsistência e execução.
type ExpurgavelPorData interface {
	ExpurgarAteData(ctx context.Context, corte time.Time) (int64, error)
}

type expurgoConfig interface {
	Obter(ctx context.Context, chave string) (string, error)
}

// ExpurgoService aplica a política de retenção lendo o parâmetro configurado.
type ExpurgoService struct {
	cfg        expurgoConfig
	posicoes   ExpurgavelPorData
	movimentos ExpurgavelPorData
	inconsist  ExpurgavelPorData
	execucoes  ExpurgavelPorData
	agora      func() time.Time // injetável em teste
}

// NewExpurgoService monta o serviço. Os repositórios devem apagar, respectivamente,
// posicao_carteira, movimento_contabil, inconsistencia_processamento e movimento_execucao.
func NewExpurgoService(cfg expurgoConfig, posicoes, movimentos, inconsistencias, execucoes ExpurgavelPorData) *ExpurgoService {
	return &ExpurgoService{cfg: cfg, posicoes: posicoes, movimentos: movimentos, inconsist: inconsistencias, execucoes: execucoes, agora: time.Now}
}

// DiasRetencao lê o parâmetro de retenção. Retorna 0 (desativado) quando ausente,
// vazio, não numérico ou negativo.
func (s *ExpurgoService) DiasRetencao(ctx context.Context) int {
	v, _ := s.cfg.Obter(ctx, ConfigRetencaoDiasMovimento)
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// IntervaloHoras lê a periodicidade do job de expurgo (default 24, mínimo 1).
func (s *ExpurgoService) IntervaloHoras(ctx context.Context) int {
	v, _ := s.cfg.Obter(ctx, ConfigRetencaoIntervaloHoras)
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return 24
	}
	return n
}

// DataCorte devolve a data de corte (hoje − N dias, só a data) e se o expurgo está
// habilitado (N>0). Movimentos com data_lote <= corte são apagados; os últimos N dias
// permanecem. Ex.: hoje 29/10, N=10 → corte 19/10 → retém 20/10..29/10 (10 dias).
func (s *ExpurgoService) DataCorte(ctx context.Context) (time.Time, bool) {
	n := s.DiasRetencao(ctx)
	if n <= 0 {
		return time.Time{}, false
	}
	t := s.agora()
	hoje := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return hoje.AddDate(0, 0, -n), true
}

// Executar aplica o expurgo conforme a retenção configurada. É seguro quando desativado
// (N<=0): não apaga nada. logf acumula o progresso (pode ser nil).
func (s *ExpurgoService) Executar(ctx context.Context, logf func(string)) error {
	if logf == nil {
		logf = func(string) {}
	}
	corte, ok := s.DataCorte(ctx)
	if !ok {
		logf("retenção desativada (" + ConfigRetencaoDiasMovimento + " <= 0): nada a expurgar")
		return nil
	}
	n := s.DiasRetencao(ctx)
	logf(fmt.Sprintf("retenção de %d dia(s): apagando lotes com data <= %s", n, corte.Format("2006-01-02")))

	nm, err := s.movimentos.ExpurgarAteData(ctx, corte)
	if err != nil {
		return fmt.Errorf("expurgo de movimentos: %w", err)
	}
	logf(fmt.Sprintf("movimentos contábeis apagados: %d", nm))

	ni, err := s.inconsist.ExpurgarAteData(ctx, corte)
	if err != nil {
		return fmt.Errorf("expurgo de inconsistências: %w", err)
	}
	logf(fmt.Sprintf("inconsistências apagadas: %d", ni))

	ne, err := s.execucoes.ExpurgarAteData(ctx, corte)
	if err != nil {
		return fmt.Errorf("expurgo de execuções: %w", err)
	}
	logf(fmt.Sprintf("execuções apagadas: %d", ne))

	np, err := s.posicoes.ExpurgarAteData(ctx, corte)
	if err != nil {
		return fmt.Errorf("expurgo de posições: %w", err)
	}
	logf(fmt.Sprintf("posições apagadas: %d", np))
	return nil
}
