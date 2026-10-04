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
	DatasExecutadas(ctx context.Context, produto, dominio string) ([]time.Time, error)
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

// DiaCalendario descreve o status AGREGADO de um dia do mês, considerando TODAS as
// combinações (produto, domínio) esperadas (padrões de arquivo). Status possíveis:
//   - "completo": todos os combos esperados tiveram contábil executado nesse dia;
//   - "parcial":  parte dos combos foi executada (falta algum);
//   - "nenhum":   dia útil, até hoje (inclusive), sem nenhum combo executado;
//   - "nao_util": sábado, domingo ou feriado (ver EhDiaUtil);
//   - "futuro":   dia útil posterior a hoje (ainda não devido), ou sem combos cadastrados.
type DiaCalendario struct {
	Dia    int    `json:"dia"`
	Data   string `json:"data"` // YYYY-MM-DD
	Status string `json:"status"`
	// Executados/Esperados quantificam os combos (produto, domínio) do dia.
	Executados int `json:"executados"`
	Esperados  int `json:"esperados"`
	// Hoje marca o dia corrente (apenas quando o mês/ano exibido é o atual).
	Hoje bool `json:"hoje,omitempty"`
	// FechamentoMensal marca o último dia útil do mês (ponto de fechamento contábil).
	FechamentoMensal bool `json:"fechamento_mensal,omitempty"`
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

// CalendarioMes retorna o status AGREGADO de cada dia do mês/ano, considerando todas as
// combinações (produto, domínio) dos padrões de arquivo: "completo" (todos executados),
// "parcial" (falta algum), "nenhum" (dia útil até hoje sem execução), "nao_util" e
// "futuro". Também marca o dia corrente (Hoje) e o último dia útil do mês (FechamentoMensal).
// A classificação de dia útil/feriado usa EhDiaUtil (fonte única).
func (s *ExecucaoService) CalendarioMes(ctx context.Context, ano int, mes time.Month) ([]DiaCalendario, error) {
	// Combinações esperadas = pares (produto, domínio) distintos dos padrões de arquivo.
	padroes, err := s.padraoRepo.Listar(ctx)
	if err != nil {
		return nil, err
	}
	esperadas := map[string]bool{}
	for _, p := range padroes {
		esperadas[strings.TrimSpace(p.Produto)+"\x00"+strings.TrimSpace(p.Dominio)] = true
	}
	totalEsperado := len(esperadas)

	hojeStr := time.Now().Format("2006-01-02")
	primeiro := time.Date(ano, mes, 1, 0, 0, 0, 0, time.UTC)

	var dias []DiaCalendario
	ultimoUtil := -1 // índice do último dia útil do mês (fechamento mensal)
	for d := primeiro; d.Month() == mes; d = d.AddDate(0, 0, 1) {
		ds := d.Format("2006-01-02")
		util := EhDiaUtil(d)

		// Conta quantos combos esperados foram executados nesse dia (apenas dias úteis
		// já vencidos — não faz sentido avaliar completude de dias futuros).
		execExpected := 0
		if util && totalEsperado > 0 && ds <= hojeStr {
			execs, err := s.execRepo.ListarPorData(ctx, d)
			if err != nil {
				return nil, err
			}
			vistos := map[string]bool{}
			for _, e := range execs {
				k := e.Produto + "\x00" + e.Dominio
				if esperadas[k] && !vistos[k] {
					vistos[k] = true
					execExpected++
				}
			}
		}

		var st string
		switch {
		case !util:
			st = "nao_util"
		case ds > hojeStr: // comparação lexicográfica válida para YYYY-MM-DD
			st = "futuro"
		case totalEsperado == 0:
			st = "futuro" // sem combos cadastrados: nada a avaliar
		case execExpected == totalEsperado:
			st = "completo"
		case execExpected > 0:
			st = "parcial"
		default:
			st = "nenhum"
		}

		dias = append(dias, DiaCalendario{
			Dia: d.Day(), Data: ds, Status: st,
			Executados: execExpected, Esperados: totalEsperado,
			Hoje: ds == hojeStr,
		})
		if util {
			ultimoUtil = len(dias) - 1
		}
	}
	if ultimoUtil >= 0 {
		dias[ultimoUtil].FechamentoMensal = true
	}
	return dias, nil
}
