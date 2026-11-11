package service

// [LAB] Features experimentais (Diagnóstico de dupla contabilização + Linhagem).
// Arquivo isolado: reusa os helpers da geração (regraAplicaAPosicao, camposFaltantes,
// boletoDaPosicao, campoBoletoOuPadrao) sem tocar no motor. Para remover a feature,
// apague este arquivo, handler/lab_handler.go, model/lab.go, as linhas "[LAB]" do main
// e a seção "[LAB]" do app.html.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

type labPosRepo interface {
	BuscarPorDataEVersaoMaxima(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error)
	UltimaDataPosicao(ctx context.Context) (time.Time, bool, error)
}

type labRegraRepo interface {
	ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error)
}

// LabService implementa o diagnóstico de dupla contabilização e a linhagem de lançamento.
type LabService struct {
	pos   labPosRepo
	regra labRegraRepo
	eval  evaluator.Evaluator
}

func NewLabService(pos labPosRepo, regra labRegraRepo, eval evaluator.Evaluator) *LabService {
	return &LabService{pos: pos, regra: regra, eval: eval}
}

// Diagnosticar avalia as regras ATIVAS contra a posição da data (vazia = última com posição)
// e aponta: (1) boletos atingidos por mais de uma regra distinta — possível lançamento em
// dobro; (2) condições que não casaram nenhuma posição; (3) condições que referenciam campos
// ausentes. Reusa exatamente a mesma avaliação da geração.
func (s *LabService) Diagnosticar(ctx context.Context, dataStr, produto, dominio string) (*model.DiagnosticoRegras, error) {
	var data time.Time
	if strings.TrimSpace(dataStr) == "" {
		d, ok, err := s.pos.UltimaDataPosicao(ctx)
		if err != nil {
			return nil, fmt.Errorf("erro ao obter a última data com posição: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("não há posição importada para diagnosticar")
		}
		data = d
	} else {
		d, err := time.Parse("2006-01-02", strings.TrimSpace(dataStr))
		if err != nil {
			return nil, fmt.Errorf("data inválida (use AAAA-MM-DD)")
		}
		data = d
	}

	regras, err := s.regra.ListarRegrasAtivas(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar regras: %w", err)
	}
	posicoes, err := s.pos.BuscarPorDataEVersaoMaxima(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar posição: %w", err)
	}

	prodF, domF := strings.TrimSpace(produto), strings.TrimSpace(dominio)

	firesPorBoleto := map[string][]model.RegraCondRef{}
	boletoMeta := map[string][2]string{}
	condFired := map[int64]bool{}
	camposAusentes := map[int64]*model.CampoAusenteItem{}
	condIndex := map[int64]model.RegraCondRef{}
	for _, r := range regras {
		for _, c := range r.Condicoes {
			condIndex[c.ID] = model.RegraCondRef{IDRegra: r.ID, Regra: r.Descricao, Condicao: c.Condicao, ContaDebito: c.ContaDebito, ContaCredito: c.ContaCredito}
		}
	}

	total := 0
	for _, pos := range posicoes {
		env := evaluator.PosicaoToEnv(pos)
		pprod := evaluator.CampoString(env, "produto")
		pdom := evaluator.CampoString(env, "dominio")
		if prodF != "" && pprod != prodF {
			continue
		}
		if domF != "" && pdom != domF {
			continue
		}
		total++
		for _, regra := range regras {
			if !regraAplicaAPosicao(regra, env) {
				continue
			}
			if pc := strings.TrimSpace(regra.PreCondicao); pc != "" {
				if len(camposFaltantes(env, pc)) > 0 {
					continue
				}
				ok, err := s.eval.EvaluateCondition(pc, env)
				if err != nil || !ok {
					continue
				}
			}
			for _, c := range regra.Condicoes {
				if falt := camposFaltantes(env, c.Condicao); len(falt) > 0 {
					labRegistrarAusentes(camposAusentes, regra, c, falt)
					continue
				}
				if strings.TrimSpace(c.Condicao) != "" {
					ok, err := s.eval.EvaluateCondition(c.Condicao, env)
					if err != nil || !ok {
						continue
					}
				}
				if faltV := camposFaltantes(env, c.CampoValor); len(faltV) > 0 {
					labRegistrarAusentes(camposAusentes, regra, c, faltV)
					continue
				}
				condFired[c.ID] = true
				boleto := evaluator.CampoString(env, campoBoletoOuPadrao(c.CampoBoleto))
				if boleto == "" {
					boleto = boletoDaPosicao(env, regra.Condicoes)
				}
				if boleto == "" {
					boleto = fmt.Sprintf("pos#%d", pos.ID)
				}
				firesPorBoleto[boleto] = append(firesPorBoleto[boleto], model.RegraCondRef{IDRegra: regra.ID, Regra: regra.Descricao, Condicao: c.Condicao, ContaDebito: c.ContaDebito, ContaCredito: c.ContaCredito})
				boletoMeta[boleto] = [2]string{pprod, pdom}
			}
		}
	}

	res := &model.DiagnosticoRegras{
		Data:               data.Format("2006-01-02"),
		TotalPosicoes:      total,
		TotalBoletos:       len(firesPorBoleto),
		Conflitos:          []model.ConflitoBoleto{},
		SemCorrespondencia: []model.RegraCondRef{},
		CamposAusentes:     []model.CampoAusenteItem{},
	}
	const limite = 500

	boletos := make([]string, 0, len(firesPorBoleto))
	for b := range firesPorBoleto {
		boletos = append(boletos, b)
	}
	sort.Strings(boletos)
	for _, b := range boletos {
		refs := firesPorBoleto[b]
		regrasDistintas := map[int64]bool{}
		for _, r := range refs {
			regrasDistintas[r.IDRegra] = true
		}
		if len(regrasDistintas) < 2 {
			continue
		}
		if len(res.Conflitos) >= limite {
			res.Truncado = true
			break
		}
		meta := boletoMeta[b]
		res.Conflitos = append(res.Conflitos, model.ConflitoBoleto{Boleto: b, Produto: meta[0], Dominio: meta[1], Regras: labDedupRefs(refs)})
	}

	condIDs := make([]int64, 0, len(condIndex))
	for id := range condIndex {
		condIDs = append(condIDs, id)
	}
	sort.Slice(condIDs, func(i, j int) bool { return condIDs[i] < condIDs[j] })
	for _, id := range condIDs {
		if condFired[id] {
			continue
		}
		if _, temAusente := camposAusentes[id]; temAusente {
			continue // já reportado em campos_ausentes
		}
		if len(res.SemCorrespondencia) >= limite {
			res.Truncado = true
			break
		}
		res.SemCorrespondencia = append(res.SemCorrespondencia, condIndex[id])
	}

	ausIDs := make([]int64, 0, len(camposAusentes))
	for id := range camposAusentes {
		ausIDs = append(ausIDs, id)
	}
	sort.Slice(ausIDs, func(i, j int) bool { return ausIDs[i] < ausIDs[j] })
	for _, id := range ausIDs {
		res.CamposAusentes = append(res.CamposAusentes, *camposAusentes[id])
	}

	return res, nil
}

// Linhagem reconstrói, para um lançamento, a regra/condição que o originou e os campos da
// posição vigente que casaram. Honesto quando a posição/regra mudou desde a geração.
func (s *LabService) Linhagem(ctx context.Context, in model.LinhagemInput) (*model.Linhagem, error) {
	data, err := time.Parse("2006-01-02", strings.TrimSpace(in.Data))
	if err != nil {
		return nil, fmt.Errorf("data inválida (use AAAA-MM-DD)")
	}
	boleto := strings.TrimSpace(in.Boleto)
	regras, err := s.regra.ListarRegrasAtivas(ctx)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar regras: %w", err)
	}

	out := &model.Linhagem{Boleto: boleto, Data: data.Format("2006-01-02"), IDRegra: in.IDRegra, Produto: in.Produto, Dominio: in.Dominio}

	var regra *model.RegraContabil
	for i := range regras {
		if regras[i].ID == in.IDRegra {
			regra = &regras[i]
			break
		}
	}
	if regra == nil {
		out.Observacao = fmt.Sprintf("A regra #%d não está mais entre as regras ativas (pode ter sido alterada ou excluída após a geração). Não é possível reconstruir a linhagem.", in.IDRegra)
		return out, nil
	}
	out.Regra = regra.Descricao
	out.TipoLancamento = labRotuloTipo(regra.TipoLancamento)
	out.PreCondicao = regra.PreCondicao

	var cond *model.CondicaoRegra
	descCond := strings.TrimSpace(in.DescricaoCondicao)
	for i := range regra.Condicoes {
		if strings.TrimSpace(regra.Condicoes[i].Condicao) == descCond {
			cond = &regra.Condicoes[i]
			break
		}
	}
	if cond == nil && len(regra.Condicoes) > 0 {
		cond = &regra.Condicoes[0]
	}
	if cond == nil {
		out.Observacao = "A regra não possui condição ativa."
		return out, nil
	}
	out.Condicao = cond.Condicao
	out.ContaDebito = cond.ContaDebito
	out.ContaCredito = cond.ContaCredito
	out.CampoValor = cond.CampoValor
	out.CampoMoeda = cond.CampoMoeda

	posicoes, err := s.pos.BuscarPorDataEVersaoMaxima(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar posição: %w", err)
	}
	campoBol := campoBoletoOuPadrao(cond.CampoBoleto)
	var envMatch map[string]interface{}
	var versaoMatch int
	for i := range posicoes {
		env := evaluator.PosicaoToEnv(posicoes[i])
		if !regraAplicaAPosicao(*regra, env) {
			continue
		}
		if evaluator.CampoString(env, campoBol) == boleto {
			envMatch = env
			versaoMatch = posicoes[i].CodigoVersaoConteudo
			break
		}
	}
	if envMatch == nil {
		out.Observacao = fmt.Sprintf("Não encontrei a posição do boleto %q em %s (versão vigente). A posição pode ter mudado de versão ou sido expurgada; ainda assim, a regra/condição acima é a que originou o lançamento.", boleto, out.Data)
		return out, nil
	}

	out.Encontrada = true
	out.Versao = versaoMatch
	if out.Produto == "" {
		out.Produto = evaluator.CampoString(envMatch, "produto")
	}
	if out.Dominio == "" {
		out.Dominio = evaluator.CampoString(envMatch, "dominio")
	}
	if pc := strings.TrimSpace(regra.PreCondicao); pc != "" {
		ok, _ := s.eval.EvaluateCondition(pc, envMatch)
		out.PreCondicaoOK = ok
	} else {
		out.PreCondicaoOK = true
	}
	if strings.TrimSpace(cond.Condicao) != "" {
		ok, _ := s.eval.EvaluateCondition(cond.Condicao, envMatch)
		out.CondicaoOK = ok
	} else {
		out.CondicaoOK = true
	}
	if !out.CondicaoOK {
		out.Observacao = "Atenção: com a posição vigente, a condição NÃO casa mais. A regra pode ter sido alterada após a geração deste lançamento."
	}
	if v, err := s.eval.EvaluateValue(cond.CampoValor, envMatch); err == nil {
		out.Valor = v
	}
	out.Moeda = evaluator.CampoString(envMatch, cond.CampoMoeda)

	nomes := map[string]bool{}
	naCond := map[string]bool{}
	var ordem []string
	addCampos := func(expr string, naCondicao bool) {
		campos, err := evaluator.CamposReferenciados(expr)
		if err != nil {
			return
		}
		sort.Strings(campos)
		for _, c := range campos {
			if !nomes[c] {
				nomes[c] = true
				ordem = append(ordem, c)
			}
			if naCondicao {
				naCond[c] = true
			}
		}
	}
	addCampos(regra.PreCondicao, true)
	addCampos(cond.Condicao, true)
	addCampos(cond.CampoValor, false)
	for _, nome := range ordem {
		_, existe := envMatch[nome]
		out.Campos = append(out.Campos, model.LinhagemCampo{Nome: nome, Valor: evaluator.CampoString(envMatch, nome), Ausente: !existe, NaCondicao: naCond[nome]})
	}
	return out, nil
}

// labRegistrarAusentes acumula (por condição) os campos referenciados mas ausentes na posição.
func labRegistrarAusentes(m map[int64]*model.CampoAusenteItem, r model.RegraContabil, c model.CondicaoRegra, campos []string) {
	it, ok := m[c.ID]
	if !ok {
		it = &model.CampoAusenteItem{IDRegra: r.ID, Regra: r.Descricao, Condicao: c.Condicao, Campos: []string{}}
		m[c.ID] = it
	}
	for _, campo := range campos {
		jaTem := false
		for _, x := range it.Campos {
			if x == campo {
				jaTem = true
				break
			}
		}
		if !jaTem {
			it.Campos = append(it.Campos, campo)
		}
	}
}

// labDedupRefs remove referências (regra, condição, contas) repetidas de um mesmo boleto.
func labDedupRefs(refs []model.RegraCondRef) []model.RegraCondRef {
	seen := map[string]bool{}
	out := make([]model.RegraCondRef, 0, len(refs))
	for _, r := range refs {
		k := fmt.Sprintf("%d|%s|%s|%s", r.IDRegra, r.Condicao, r.ContaDebito, r.ContaCredito)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, r)
	}
	return out
}

// labRotuloTipo devolve o rótulo legível do tipo de lançamento da regra.
func labRotuloTipo(t string) string {
	switch model.NormalizaTipoLancamento(t) {
	case model.TipoNaoReverte:
		return "Não reverte"
	case model.TipoIncremental:
		return "Incremental / Não reverte"
	case model.TipoIncrementalReverte:
		return "Incremental / Reverte"
	default:
		return "Reverte"
	}
}
