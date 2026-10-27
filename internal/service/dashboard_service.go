package service

import (
	"context"
	"sort"
	"time"

	"srcoff/internal/model"
	"srcoff/internal/repository"
)

// maxDiasDashboard limita a janela do dashboard para evitar varreduras longas
// (o dashboard itera dia a dia sobre execução/inconsistências).
const maxDiasDashboard = 120

// DashboardService monta a visão consolidada do menu Dashboard (RF-120) compondo
// os repositórios já existentes — sem novas consultas de agregação nos backends.
type DashboardService struct {
	execucaoRepo       repository.ExecucaoRepository
	inconsistenciaRepo repository.InconsistenciaRepository
	saldoRepo          repository.SaldoContaRepository // pode ser nil quando o saldo está desabilitado
	cotacaoRepo        repository.CotacaoRepository
}

func NewDashboardService(e repository.ExecucaoRepository, i repository.InconsistenciaRepository, s repository.SaldoContaRepository, c repository.CotacaoRepository) *DashboardService {
	return &DashboardService{execucaoRepo: e, inconsistenciaRepo: i, saldoRepo: s, cotacaoRepo: c}
}

// Gerar consolida os indicadores do período [inicio, fim] (datas inclusivas).
func (s *DashboardService) Gerar(ctx context.Context, inicio, fim time.Time) (*model.Dashboard, error) {
	inicio = truncarDia(inicio)
	fim = truncarDia(fim)
	if fim.Before(inicio) {
		inicio, fim = fim, inicio
	}
	// Limita a janela: mantém os últimos maxDiasDashboard dias do intervalo pedido.
	if fim.Sub(inicio).Hours()/24 > float64(maxDiasDashboard) {
		inicio = fim.AddDate(0, 0, -maxDiasDashboard)
	}

	d := &model.Dashboard{
		Inicio:                 inicio.Format("2006-01-02"),
		Fim:                    fim.Format("2006-01-02"),
		VolumesPorDia:          []model.DashboardVolumeDia{},
		VolumesPorProduto:      []model.DashboardVolumeProduto{},
		InconsistenciasPorDia:  []model.DashboardContagem{},
		InconsistenciasPorTipo: []model.DashboardContagem{},
		Cambio:                 []model.DashboardCambio{},
	}

	// Volumes (execução) e inconsistências por dia, iterando o intervalo.
	porProduto := map[string]*model.DashboardVolumeProduto{}
	porTipo := map[string]int{}
	for dia := inicio; !dia.After(fim); dia = dia.AddDate(0, 0, 1) {
		dataStr := dia.Format("2006-01-02")

		execs, err := s.execucaoRepo.ListarPorData(ctx, dia)
		if err != nil {
			return nil, err
		}
		vd := model.DashboardVolumeDia{Data: dataStr}
		for _, e := range execs {
			vd.Lancamentos += e.QtdLancamentos
			vd.Estornos += e.QtdEstornos
			vd.Movimento += e.QtdMovimento
			k := e.Produto + "\x00" + e.Dominio
			p := porProduto[k]
			if p == nil {
				p = &model.DashboardVolumeProduto{Produto: e.Produto, Dominio: e.Dominio}
				porProduto[k] = p
			}
			p.Lancamentos += e.QtdLancamentos
			p.Movimento += e.QtdMovimento
		}
		if vd.Lancamentos != 0 || vd.Estornos != 0 || vd.Movimento != 0 {
			d.VolumesPorDia = append(d.VolumesPorDia, vd)
			d.TotalMovimento += vd.Movimento
			d.TotalEstornos += vd.Estornos
		}

		incs, err := s.inconsistenciaRepo.ListarPorData(ctx, dia)
		if err != nil {
			return nil, err
		}
		if len(incs) > 0 {
			d.InconsistenciasPorDia = append(d.InconsistenciasPorDia, model.DashboardContagem{Rotulo: dataStr, Total: len(incs)})
			d.TotalInconsistencias += len(incs)
			for _, ic := range incs {
				t := ic.Tipo
				if t == "" {
					t = "(sem tipo)"
				}
				porTipo[t]++
			}
		}
	}

	// Volumes por produto: ordena por movimento desc.
	for _, p := range porProduto {
		d.VolumesPorProduto = append(d.VolumesPorProduto, *p)
	}
	sort.Slice(d.VolumesPorProduto, func(i, j int) bool {
		if d.VolumesPorProduto[i].Movimento != d.VolumesPorProduto[j].Movimento {
			return d.VolumesPorProduto[i].Movimento > d.VolumesPorProduto[j].Movimento
		}
		return d.VolumesPorProduto[i].Produto < d.VolumesPorProduto[j].Produto
	})

	// Inconsistências por tipo: ordena por total desc.
	for t, n := range porTipo {
		d.InconsistenciasPorTipo = append(d.InconsistenciasPorTipo, model.DashboardContagem{Rotulo: t, Total: n})
	}
	sort.Slice(d.InconsistenciasPorTipo, func(i, j int) bool {
		return d.InconsistenciasPorTipo[i].Total > d.InconsistenciasPorTipo[j].Total
	})

	// Saldos: total de contas com saldo != 0 (projeção RN-170). Pode estar desabilitado.
	if s.saldoRepo != nil {
		pag, err := s.saldoRepo.ListarPaginado(ctx, model.SaldoFiltro{}, 1, 1)
		if err == nil && pag != nil {
			d.TotalContasComSaldo = pag.Total
		}
	}

	// Câmbio: cotações mais recentes (até 12).
	if s.cotacaoRepo != nil {
		cots, err := s.cotacaoRepo.Listar(ctx)
		if err == nil {
			sort.Slice(cots, func(i, j int) bool { return cots[i].Data.After(cots[j].Data) })
			for i, c := range cots {
				if i >= 12 {
					break
				}
				d.Cambio = append(d.Cambio, model.DashboardCambio{
					Data:    c.Data.Format("2006-01-02"),
					Moeda1:  c.Moeda1,
					Moeda2:  c.Moeda2,
					Direcao: c.Direcao,
					Taxa:    c.Taxa,
				})
			}
		}
	}

	return d, nil
}

func truncarDia(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
