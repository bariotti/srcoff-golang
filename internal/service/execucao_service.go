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
	Produto            string `json:"produto"`
	Dominio            string `json:"dominio"`
	Processado         bool   `json:"processado"`
	QtdLancamentos     int    `json:"qtd_lancamentos"`
	QtdEstornos        int    `json:"qtd_estornos"`
	QtdInconsistencias int    `json:"qtd_inconsistencias"`
}

// DiaCalendario descreve o status AGREGADO de um dia do mês, considerando TODAS as
// combinações (produto, domínio) esperadas (padrões de arquivo). Status possíveis:
//   - "completo": todos os combos esperados tiveram contábil executado nesse dia;
//   - "parcial":  parte dos combos foi executada (falta algum);
//   - "nenhum":   dia útil, até hoje (inclusive), sem nenhum combo executado;
//   - "inconsistencia": dia processado cuja última versão tem inconsistências;
//   - "nao_util": sábado, domingo ou feriado (ver EhDiaUtil);
//   - "futuro":   dia útil posterior a hoje (ainda não devido), ou sem combos cadastrados.
type DiaCalendario struct {
	Dia    int    `json:"dia"`
	Data   string `json:"data"` // YYYY-MM-DD
	Status string `json:"status"`
	// Executados/Esperados quantificam os combos (produto, domínio) do dia.
	Executados int `json:"executados"`
	Esperados  int `json:"esperados"`
	// Inconsistencias conta as inconsistências da última versão processada no dia.
	Inconsistencias int `json:"inconsistencias"`
	// Hoje marca o dia corrente (apenas quando o mês/ano exibido é o atual).
	Hoje bool `json:"hoje,omitempty"`
	// FechamentoMensal marca o último dia útil do mês (ponto de fechamento contábil).
	FechamentoMensal bool `json:"fechamento_mensal,omitempty"`
}

// ExecucaoService monta a visão de processados/pendentes por data.
type ExecucaoService struct {
	execRepo   execucaoRepoReader
	padraoRepo padroesParaEscopo
	incRepo    inconsistenciaRepoReader
}

func NewExecucaoService(execRepo execucaoRepoReader, padraoRepo padroesParaEscopo) *ExecucaoService {
	return &ExecucaoService{execRepo: execRepo, padraoRepo: padraoRepo}
}

// ComInconsistenciaRepo injeta o repositório de inconsistências para expor a contagem
// por combinação (status) e por dia (calendário). Opcional: sem ele, a contagem é zero.
func (s *ExecucaoService) ComInconsistenciaRepo(r inconsistenciaRepoReader) *ExecucaoService {
	s.incRepo = r
	return s
}

// inconsistenciasPorCombo lê as inconsistências da data e conta por (produto, domínio).
// Como as inconsistências são substituídas a cada processamento (SubstituirPorEscopo),
// a contagem reflete sempre a última versão do lote para cada combinação.
func (s *ExecucaoService) inconsistenciasPorCombo(ctx context.Context, data time.Time) (map[string]int, error) {
	if s.incRepo == nil {
		return map[string]int{}, nil
	}
	itens, err := s.incRepo.ListarPorData(ctx, data)
	if err != nil {
		return nil, err
	}
	m := map[string]int{}
	for _, i := range itens {
		m[i.Produto+"\x00"+i.Dominio]++
	}
	return m, nil
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

	incPorCombo, err := s.inconsistenciasPorCombo(ctx, data)
	if err != nil {
		return nil, err
	}

	var status []StatusCombinacao
	for _, c := range esperadas {
		k := c.Produto + "\x00" + c.Dominio
		e, ok := porCombo[k]
		status = append(status, StatusCombinacao{
			Produto:            c.Produto,
			Dominio:            c.Dominio,
			Processado:         ok,
			QtdLancamentos:     e.QtdLancamentos,
			QtdEstornos:        e.QtdEstornos,
			QtdInconsistencias: incPorCombo[k],
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

		// Conta quantos combos esperados foram executados nesse dia e quantas
		// inconsistências a última versão gerou (apenas dias úteis já vencidos —
		// não faz sentido avaliar completude de dias futuros).
		execExpected := 0
		incCount := 0
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
			incPorCombo, err := s.inconsistenciasPorCombo(ctx, d)
			if err != nil {
				return nil, err
			}
			for k, n := range incPorCombo {
				if esperadas[k] {
					incCount += n
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
		case incCount > 0:
			st = "inconsistencia" // processado, mas a última versão tem inconsistências
		case execExpected == 0:
			st = "nenhum"
		case execExpected == totalEsperado:
			st = "completo"
		default:
			st = "parcial"
		}

		dias = append(dias, DiaCalendario{
			Dia: d.Day(), Data: ds, Status: st,
			Executados: execExpected, Esperados: totalEsperado,
			Inconsistencias: incCount,
			Hoje:            ds == hojeStr,
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
