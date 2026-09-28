package file

import (
	"context"
	"strings"
	"time"

	"srcoff/internal/model"
)

// MovimentoContabilRepo implementa MovimentoContabilRepository usando arquivo JSON.
type MovimentoContabilRepo struct {
	st *store[model.LancamentoContabil]
}

func NewMovimentoContabilRepo(dir string) *MovimentoContabilRepo {
	return &MovimentoContabilRepo{st: newStore[model.LancamentoContabil](dir, "movimento_contabil.json")}
}

func (r *MovimentoContabilRepo) BulkInsert(_ context.Context, lancamentos []model.LancamentoContabil) error {
	all, err := r.st.load()
	if err != nil {
		return err
	}
	maxID := int64(0)
	for _, l := range all {
		if l.ID > maxID {
			maxID = l.ID
		}
	}
	for i := range lancamentos {
		maxID++
		lancamentos[i].ID = maxID
		all = append(all, lancamentos[i])
	}
	return r.st.save(all)
}

// comboKeyMov identifica a combinação (data, produto, domínio) de um lançamento.
func comboKeyMov(l model.LancamentoContabil) string {
	return l.DataLoteContabil.Format("2006-01-02") + "\x00" + l.Produto + "\x00" + l.Dominio
}

// BuscarPorDataEIndicador — vigente (MAX versão) por (data, produto, domínio).
func (r *MovimentoContabilRepo) BuscarPorDataEIndicador(_ context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}
	dataStr := data.Format("2006-01-02")

	// Versão vigente por combinação (data, produto, domínio)
	maxPorCombo := map[string]int{}
	for _, l := range all {
		if l.DataLoteContabil.Format("2006-01-02") != dataStr {
			continue
		}
		k := comboKeyMov(l)
		if l.CodigoVersaoConteudo > maxPorCombo[k] {
			maxPorCombo[k] = l.CodigoVersaoConteudo
		}
	}

	var result []model.LancamentoContabil
	for _, l := range all {
		if l.DataLoteContabil.Format("2006-01-02") == dataStr &&
			l.IndicadorReversao == indicadorReversao &&
			l.CodigoVersaoConteudo == maxPorCombo[comboKeyMov(l)] {
			result = append(result, l)
		}
	}
	return result, nil
}

func (r *MovimentoContabilRepo) ObterProximaVersao(_ context.Context, data time.Time) (int, error) {
	all, err := r.st.load()
	if err != nil {
		return 0, err
	}
	dataStr := data.Format("2006-01-02")
	max := 0
	for _, l := range all {
		if l.DataLoteContabil.Format("2006-01-02") == dataStr && l.CodigoVersaoConteudo > max {
			max = l.CodigoVersaoConteudo
		}
	}
	return max + 1, nil
}

func (r *MovimentoContabilRepo) ObterVersaoAtual(_ context.Context, data time.Time) (int, error) {
	all, err := r.st.load()
	if err != nil {
		return 0, err
	}
	dataStr := data.Format("2006-01-02")
	max := 1
	for _, l := range all {
		if l.DataLoteContabil.Format("2006-01-02") == dataStr && l.CodigoVersaoConteudo > max {
			max = l.CodigoVersaoConteudo
		}
	}
	return max, nil
}

func (r *MovimentoContabilRepo) ConsultarPaginado(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return r.ConsultarPaginadoFiltrado(ctx, data, data, "", 0, "todas", pagina, tamanho)
}

func (r *MovimentoContabilRepo) ConsultarPaginadoFiltrado(_ context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}
	return paginarFiltrado(all, dataInicio, dataFim, boleto, versao, versaoModo, pagina, tamanho), nil
}

func (r *MovimentoContabilRepo) ConsultarPaginadoFiltradoSemCancelados(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}

	// Calcular versão vigente por (data, produto, domínio)
	maxVersaoPorCombo := map[string]int{}
	inicioStr := dataInicio.Format("2006-01-02")
	fimStr := dataFim.Format("2006-01-02")
	for _, l := range all {
		d := l.DataLoteContabil.Format("2006-01-02")
		if d >= inicioStr && d <= fimStr && l.CodigoVersaoConteudo > maxVersaoPorCombo[comboKeyMov(l)] {
			maxVersaoPorCombo[comboKeyMov(l)] = l.CodigoVersaoConteudo
		}
	}

	// Calcular saldo líquido usando SEMPRE a versão vigente por combinação
	// Chave: boleto + regra + conta_debito + conta_credito
	type chave struct {
		data         string
		boleto       string
		regra        int64
		contaDebito  string
		contaCredito string
	}
	saldo := map[chave]float64{}
	for _, l := range all {
		d := l.DataLoteContabil.Format("2006-01-02")
		if d < inicioStr || d > fimStr {
			continue
		}
		if l.CodigoVersaoConteudo != maxVersaoPorCombo[comboKeyMov(l)] {
			continue
		}

		var k chave

		if l.IndicadorReversao {
			k = chave{d, l.CodigoIdentificadorBoleto, l.IDRegraContabil, l.ContaCredito, l.ContaDebito}
		} else {
			k = chave{d, l.CodigoIdentificadorBoleto, l.IDRegraContabil, l.ContaDebito, l.ContaCredito}
		}

		//k := chave{d, l.CodigoIdentificadorBoleto, l.IDRegraContabil, l.ContaDebito, l.ContaCredito}

		if l.IndicadorReversao {
			saldo[k] -= l.ValorLancamentoContabil
		} else {
			saldo[k] += l.ValorLancamentoContabil
		}
	}

	// Aplicar filtros normais (período, boleto, versaoModo)
	result := paginarFiltrado(all, dataInicio, dataFim, boleto, versao, versaoModo, 1, 999999)

	// Remover lançamentos com saldo zero
	var filtered []model.LancamentoContabil
	for _, l := range result.Lancamentos {
		d := l.DataLoteContabil.Format("2006-01-02")

		var k chave

		if l.IndicadorReversao {
			k = chave{d, l.CodigoIdentificadorBoleto, l.IDRegraContabil, l.ContaCredito, l.ContaDebito}
		} else {
			k = chave{d, l.CodigoIdentificadorBoleto, l.IDRegraContabil, l.ContaDebito, l.ContaCredito}
		}

		//k := chave{d, l.CodigoIdentificadorBoleto, l.IDRegraContabil, l.ContaDebito, l.ContaCredito}
		if saldo[k] != 0 {
			filtered = append(filtered, l)
		}
	}

	total := len(filtered)
	offset := (pagina - 1) * tamanho
	if offset >= total {
		return &model.PaginaLancamentos{Total: total, Pagina: pagina, Tamanho: tamanho, Lancamentos: []model.LancamentoContabil{}}, nil
	}
	end := offset + tamanho
	if end > total {
		end = total
	}
	return &model.PaginaLancamentos{Total: total, Pagina: pagina, Tamanho: tamanho, Lancamentos: filtered[offset:end]}, nil
}

func (r *MovimentoContabilRepo) ExcluirPorDataEVersao(_ context.Context, data time.Time, versao int) error {
	all, err := r.st.load()
	if err != nil {
		return err
	}
	dataStr := data.Format("2006-01-02")
	var filtered []model.LancamentoContabil
	for _, l := range all {
		if l.DataLoteContabil.Format("2006-01-02") == dataStr {
			if versao == 0 || l.CodigoVersaoConteudo == versao {
				continue
			}
		}
		filtered = append(filtered, l)
	}
	return r.st.save(filtered)
}

// paginarFiltrado aplica filtros e paginação em memória.
func paginarFiltrado(all []model.LancamentoContabil, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) *model.PaginaLancamentos {
	inicioStr := dataInicio.Format("2006-01-02")
	fimStr := dataFim.Format("2006-01-02")

	// Calcular versão vigente por (data, produto, domínio) para modo vigente
	maxVersaoPorCombo := map[string]int{}
	if versaoModo == "vigente" {
		for _, l := range all {
			d := l.DataLoteContabil.Format("2006-01-02")
			if d >= inicioStr && d <= fimStr && l.CodigoVersaoConteudo > maxVersaoPorCombo[comboKeyMov(l)] {
				maxVersaoPorCombo[comboKeyMov(l)] = l.CodigoVersaoConteudo
			}
		}
	}

	var filtered []model.LancamentoContabil
	for _, l := range all {
		d := l.DataLoteContabil.Format("2006-01-02")
		if d < inicioStr || d > fimStr {
			continue
		}
		if boleto != "" && !strings.Contains(l.CodigoIdentificadorBoleto, boleto) {
			continue
		}
		switch versaoModo {
		case "especifica":
			if l.CodigoVersaoConteudo != versao {
				continue
			}
		case "vigente":
			if l.CodigoVersaoConteudo != maxVersaoPorCombo[comboKeyMov(l)] {
				continue
			}
		}
		filtered = append(filtered, l)
	}

	total := len(filtered)
	offset := (pagina - 1) * tamanho
	if offset >= total {
		return &model.PaginaLancamentos{Total: total, Pagina: pagina, Tamanho: tamanho, Lancamentos: []model.LancamentoContabil{}}
	}
	end := offset + tamanho
	if end > total {
		end = total
	}
	return &model.PaginaLancamentos{Total: total, Pagina: pagina, Tamanho: tamanho, Lancamentos: filtered[offset:end]}
}
