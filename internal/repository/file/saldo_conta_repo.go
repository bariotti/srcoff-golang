package file

import (
	"context"
	"sort"

	"srcoff/internal/model"
)

// SaldoContaRepo implementa a projeção de saldo (RN-170) em arquivo JSON.
// É um cache derivado do movimento_contabil; nunca é fonte da verdade.
type SaldoContaRepo struct {
	st *store[model.SaldoConta]
}

func NewSaldoContaRepo(dir string) *SaldoContaRepo {
	return &SaldoContaRepo{st: newStore[model.SaldoConta](dir, "saldo_conta.json")}
}

func chaveSaldo(s model.SaldoConta) [6]interface{} {
	return [6]interface{}{s.Produto, s.Dominio, s.CodigoIdentificadorBoleto, s.IDRegraContabil, s.ContaDebito, s.ContaCredito}
}

func mesmaChave(a, b model.SaldoConta) bool {
	return a.Produto == b.Produto && a.Dominio == b.Dominio &&
		a.CodigoIdentificadorBoleto == b.CodigoIdentificadorBoleto &&
		a.IDRegraContabil == b.IDRegraContabil &&
		a.ContaDebito == b.ContaDebito && a.ContaCredito == b.ContaCredito
}

func (r *SaldoContaRepo) AplicarDeltas(_ context.Context, deltas []model.SaldoConta) error {
	if len(deltas) == 0 {
		return nil
	}
	itens, err := r.st.load()
	if err != nil {
		return err
	}
	idx := map[[6]interface{}]int{}
	for i, s := range itens {
		idx[chaveSaldo(s)] = i
	}
	for _, d := range deltas {
		k := chaveSaldo(d)
		if i, ok := idx[k]; ok {
			itens[i].Saldo += d.Saldo
			itens[i].MoedaLancamento = d.MoedaLancamento
			itens[i].DescricaoRegra = d.DescricaoRegra
			itens[i].DescricaoCondicao = d.DescricaoCondicao
		} else {
			itens = append(itens, d)
			idx[k] = len(itens) - 1
		}
	}
	return r.st.save(itens)
}

func (r *SaldoContaRepo) SubstituirCombos(_ context.Context, combos []model.ProdutoDominio, saldos []model.SaldoConta) error {
	itens, err := r.st.load()
	if err != nil {
		return err
	}
	remover := map[string]bool{}
	for _, c := range combos {
		remover[c.Produto+"\x00"+c.Dominio] = true
	}
	var filtrado []model.SaldoConta
	for _, s := range itens {
		if !remover[s.Produto+"\x00"+s.Dominio] {
			filtrado = append(filtrado, s)
		}
	}
	filtrado = append(filtrado, saldos...)
	return r.st.save(filtrado)
}

func (r *SaldoContaRepo) Listar(_ context.Context, f model.SaldoFiltro) ([]model.SaldoConta, error) {
	itens, err := r.st.load()
	if err != nil {
		return nil, err
	}
	regrasIn := map[int64]bool{}
	for _, id := range f.RegrasIn {
		regrasIn[id] = true
	}
	var out []model.SaldoConta
	for _, s := range itens {
		if f.Produto != "" && s.Produto != f.Produto {
			continue
		}
		if f.Dominio != "" && s.Dominio != f.Dominio {
			continue
		}
		if f.Boleto != "" && s.CodigoIdentificadorBoleto != f.Boleto {
			continue
		}
		if f.IDRegra != 0 && s.IDRegraContabil != f.IDRegra {
			continue
		}
		if f.ContaDebito != "" && s.ContaDebito != f.ContaDebito {
			continue
		}
		if f.ContaCredito != "" && s.ContaCredito != f.ContaCredito {
			continue
		}
		if f.Conta != "" && s.ContaDebito != f.Conta && s.ContaCredito != f.Conta {
			continue
		}
		if len(regrasIn) > 0 && !regrasIn[s.IDRegraContabil] {
			continue
		}
		if !f.IncluiZero && s.Saldo == 0 {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *SaldoContaRepo) ListarPaginado(ctx context.Context, f model.SaldoFiltro, pagina, tamanho int) (*model.SaldoPagina, error) {
	todos, err := r.Listar(ctx, f)
	if err != nil {
		return nil, err
	}
	sort.Slice(todos, func(i, j int) bool {
		a, b := todos[i], todos[j]
		if a.Produto != b.Produto {
			return a.Produto < b.Produto
		}
		if a.Dominio != b.Dominio {
			return a.Dominio < b.Dominio
		}
		if a.CodigoIdentificadorBoleto != b.CodigoIdentificadorBoleto {
			return a.CodigoIdentificadorBoleto < b.CodigoIdentificadorBoleto
		}
		return a.IDRegraContabil < b.IDRegraContabil
	})
	total := len(todos)
	if pagina < 1 {
		pagina = 1
	}
	if tamanho < 1 {
		tamanho = 50
	}
	ini := (pagina - 1) * tamanho
	if ini > total {
		ini = total
	}
	fim := ini + tamanho
	if fim > total {
		fim = total
	}
	return &model.SaldoPagina{Total: total, Pagina: pagina, Tamanho: tamanho, Saldos: todos[ini:fim]}, nil
}

func (r *SaldoContaRepo) EstaVazio(_ context.Context) (bool, error) {
	itens, err := r.st.load()
	if err != nil {
		return false, err
	}
	return len(itens) == 0, nil
}
