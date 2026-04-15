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

// BuscarPorDataEIndicador — equivalente ao SQL Server: filtra pela versão vigente (MAX).
func (r *MovimentoContabilRepo) BuscarPorDataEIndicador(_ context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error) {
	all, err := r.st.load()
	if err != nil {
		return nil, err
	}
	dataStr := data.Format("2006-01-02")

	// Calcular versão vigente para a data
	maxVersao := 0
	for _, l := range all {
		if l.DataLoteContabil.Format("2006-01-02") == dataStr && l.CodigoVersaoConteudo > maxVersao {
			maxVersao = l.CodigoVersaoConteudo
		}
	}

	var result []model.LancamentoContabil
	for _, l := range all {
		if l.DataLoteContabil.Format("2006-01-02") == dataStr &&
			l.IndicadorReversao == indicadorReversao &&
			l.CodigoVersaoConteudo == maxVersao {
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

	// Calcular versão vigente por data (equivalente ao SQL Server que sempre usa MAX para o filtro de saldo zero)
	maxVersaoPorData := map[string]int{}
	inicioStr := dataInicio.Format("2006-01-02")
	fimStr := dataFim.Format("2006-01-02")
	for _, l := range all {
		d := l.DataLoteContabil.Format("2006-01-02")
		if d >= inicioStr && d <= fimStr && l.CodigoVersaoConteudo > maxVersaoPorData[d] {
			maxVersaoPorData[d] = l.CodigoVersaoConteudo
		}
	}

	// Calcular saldo líquido usando SEMPRE a versão vigente (equivalente ao SQL Server)
	type chave struct {
		data  string
		boleto string
		valor float64
		regra int64
	}
	saldo := map[chave]float64{}
	for _, l := range all {
		d := l.DataLoteContabil.Format("2006-01-02")
		if d < inicioStr || d > fimStr {
			continue
		}
		if l.CodigoVersaoConteudo != maxVersaoPorData[d] {
			continue
		}
		k := chave{d, l.CodigoIdentificadorBoleto, l.ValorLancamentoContabil, l.IDRegraContabil}
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
		k := chave{d, l.CodigoIdentificadorBoleto, l.ValorLancamentoContabil, l.IDRegraContabil}
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

	// Calcular versão vigente por data para modo vigente
	maxVersaoPorData := map[string]int{}
	if versaoModo == "vigente" {
		for _, l := range all {
			d := l.DataLoteContabil.Format("2006-01-02")
			if d >= inicioStr && d <= fimStr && l.CodigoVersaoConteudo > maxVersaoPorData[d] {
				maxVersaoPorData[d] = l.CodigoVersaoConteudo
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
			if l.CodigoVersaoConteudo != maxVersaoPorData[d] {
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
