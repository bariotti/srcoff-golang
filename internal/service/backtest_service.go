package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

// BacktestRegra roda a regra candidata (mesma avaliação do What-if) sobre as últimas N datas COM
// posição, retrocedendo da última data importada, e devolve o impacto agregado por dia e por par
// de contas (RF-135). Não persiste nada. A lógica de avaliação por posição espelha SimularRegra
// (fonte da verdade); aqui apenas AGREGA (sem listar detalhe), por isso não há truncamento.
func (s *MovimentoContabilService) BacktestRegra(ctx context.Context, in model.BacktestInput) (*model.BacktestResultado, error) {
	n := in.Dias
	if n <= 0 {
		n = 10
	}
	if n > 60 {
		n = 60
	}

	// Monta a regra candidata (igual ao What-if) e normaliza nomes/expressões.
	regra := model.RegraContabil{
		Descricao:      "(backtest)",
		PreCondicao:    in.Regra.PreCondicao,
		TipoLancamento: model.NormalizaTipoLancamento(in.Regra.TipoLancamento),
		Ativo:          true,
	}
	for _, c := range in.Regra.Condicoes {
		if strings.TrimSpace(c.Condicao) == "" && strings.TrimSpace(c.CampoValor) == "" {
			continue
		}
		regra.Condicoes = append(regra.Condicoes, model.CondicaoRegra{
			Condicao: c.Condicao, ContaDebito: c.ContaDebito, ContaCredito: c.ContaCredito,
			CampoValor: c.CampoValor, CampoMoeda: c.CampoMoeda, CampoBoleto: c.CampoBoleto, Ativo: true,
			BaseNaPosicao: c.BaseNaPosicao, CampoValorMoedaBase: c.CampoValorMoedaBase,
			CampoMoedaBase: c.CampoMoedaBase, CampoDataConversao: c.CampoDataConversao,
		})
	}
	if len(regra.Condicoes) == 0 {
		return nil, fmt.Errorf("informe ao menos uma condição com expressão e campo_valor")
	}
	normalizarCamposRegra(&regra)

	ultima, ok, err := s.posicaoRepo.UltimaDataPosicao(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter última data com posição: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("não há posição importada para o backtest")
	}

	// Dependências de moeda base (iguais à simulação), carregadas uma vez.
	moedaBasePorVeiculo := map[string]string{}
	if s.moedaBaseRepo != nil {
		if m, err := s.moedaBaseRepo.MapaVeiculoMoedaBase(ctx); err == nil {
			moedaBasePorVeiculo = m
		}
	}
	cotacoes := map[string]model.Cotacao{}
	if s.cotacaoRepo != nil {
		if m, err := s.cotacaoRepo.MapaCotacoes(ctx); err == nil {
			cotacoes = m
		}
	}
	var padroes []model.PadraoArquivo
	if s.padraoRepo != nil {
		padroes, _ = s.padraoRepo.Listar(ctx)
	}
	padraoDe := func(produto, dominio string) (string, string) {
		for _, p := range padroes {
			if strings.TrimSpace(p.Produto) == produto && strings.TrimSpace(p.Dominio) == dominio {
				return evaluator.NormalizarNome(p.ColunaVeiculo), evaluator.LayoutData(p.FormatoData)
			}
		}
		return "", ""
	}

	res := &model.BacktestResultado{DiasSolicitados: n}
	if regra.UsaCalculoIncremental() {
		res.Aviso = "Regra incremental: o backtest mostra o valor BRUTO de campo_valor por dia (o valor diário real |D0 − D-N| depende do histórico persistido)."
	}
	porConta := map[string]*model.BacktestConta{}

	// Parte da última data útil <= última data com posição e retrocede por dias úteis.
	d := ultima
	for !EhDiaUtil(d) {
		d = DiaUtilAnterior(d)
	}
	maxIter := n*6 + 400 // trava de segurança: datas sem posição no escopo não contam
	for res.DiasComPosicao < n && maxIter > 0 {
		maxIter--
		if dia, contou := s.backtestUmDia(ctx, d, regra, in.Regra, moedaBasePorVeiculo, cotacoes, padraoDe, porConta); contou {
			res.Dias = append(res.Dias, dia)
			res.TotalLancamentos += dia.TotalLancamentos
			res.TotalInconsistencias += dia.TotalInconsistencias
			res.SomaValor += dia.SomaValor
			res.SomaValorBase += dia.SomaValorBase
			res.DiasComPosicao++
		}
		d = DiaUtilAnterior(d)
	}

	if len(res.Dias) > 0 {
		res.DataFim = res.Dias[0].Data
		res.DataInicio = res.Dias[len(res.Dias)-1].Data
	}
	for _, c := range porConta {
		res.PorConta = append(res.PorConta, *c)
	}
	sort.Slice(res.PorConta, func(i, j int) bool {
		return math.Abs(res.PorConta[i].SomaValor) > math.Abs(res.PorConta[j].SomaValor)
	})
	return res, nil
}

// backtestUmDia avalia a regra candidata sobre a posição vigente de uma data e devolve o
// agregado. contou=false quando não há posição no escopo (a data não conta para o N).
func (s *MovimentoContabilService) backtestUmDia(
	ctx context.Context, d time.Time, regra model.RegraContabil, in model.SimulacaoRegraInput,
	moedaBasePorVeiculo map[string]string, cotacoes map[string]model.Cotacao,
	padraoDe func(string, string) (string, string), porConta map[string]*model.BacktestConta,
) (model.BacktestDia, bool) {
	posicoes, err := s.posicaoRepo.BuscarPorDataEVersaoMaxima(ctx, d)
	if err != nil || len(posicoes) == 0 {
		return model.BacktestDia{}, false
	}
	boletoFiltro := strings.TrimSpace(in.Boleto)
	prodFiltro := strings.TrimSpace(in.Produto)
	domFiltro := strings.TrimSpace(in.Dominio)
	dia := model.BacktestDia{Data: d.Format("2006-01-02")}
	avaliouAlgo := false

	for _, posicao := range posicoes {
		env := evaluator.PosicaoToEnv(posicao)
		produto := evaluator.CampoString(env, "produto")
		dominio := evaluator.CampoString(env, "dominio")
		if prodFiltro != "" && produto != prodFiltro {
			continue
		}
		if domFiltro != "" && dominio != domFiltro {
			continue
		}
		if boletoFiltro != "" && boletoDaPosicao(env, regra.Condicoes) != boletoFiltro {
			continue
		}
		avaliouAlgo = true
		dia.PosicoesAvaliadas++

		if strings.TrimSpace(regra.PreCondicao) != "" {
			if len(camposFaltantes(env, regra.PreCondicao)) > 0 {
				dia.TotalInconsistencias++
				continue
			}
			ok, err := s.evaluator.EvaluateCondition(regra.PreCondicao, env)
			if err != nil {
				dia.TotalInconsistencias++
				continue
			}
			if !ok {
				continue
			}
		}

		for _, condicao := range regra.Condicoes {
			if len(camposFaltantes(env, condicao.Condicao)) > 0 {
				dia.TotalInconsistencias++
				continue
			}
			if strings.TrimSpace(condicao.Condicao) != "" {
				ok, err := s.evaluator.EvaluateCondition(condicao.Condicao, env)
				if err != nil {
					dia.TotalInconsistencias++
					continue
				}
				if !ok {
					continue
				}
			}
			if len(camposFaltantes(env, condicao.CampoValor)) > 0 {
				dia.TotalInconsistencias++
				continue
			}
			valor, err := s.evaluator.EvaluateValue(condicao.CampoValor, env)
			if err != nil {
				dia.TotalInconsistencias++
				continue
			}
			if len(camposFaltantes(env, condicao.CampoMoeda)) > 0 {
				dia.TotalInconsistencias++
				continue
			}
			moeda := evaluator.CampoString(env, condicao.CampoMoeda)
			if strings.TrimSpace(moeda) == "" {
				dia.TotalInconsistencias++
				continue
			}
			boletoLanc := evaluator.CampoString(env, campoBoletoOuPadrao(condicao.CampoBoleto))
			colVeiculo, layoutData := padraoDe(produto, dominio)
			veiculo := evaluator.CampoString(env, colVeiculo)
			valorBase, _, incMB := s.resolverMoedaBaseVeiculo(d, boletoLanc, produto, dominio, regra, condicao, env, veiculo, valor, moeda, moedaBasePorVeiculo, cotacoes, layoutData)
			if incMB != nil {
				dia.TotalInconsistencias++
				continue
			}
			dia.TotalLancamentos++
			dia.SomaValor += valor
			dia.SomaValorBase += valorBase
			key := condicao.ContaDebito + "|" + condicao.ContaCredito
			pc := porConta[key]
			if pc == nil {
				pc = &model.BacktestConta{ContaDebito: condicao.ContaDebito, ContaCredito: condicao.ContaCredito}
				porConta[key] = pc
			}
			pc.Qtd++
			pc.SomaValor += valor
		}
	}
	return dia, avaliouAlgo
}
