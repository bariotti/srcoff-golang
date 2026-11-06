package service

import (
	"context"
	"testing"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

// fakeMovRepoVersoes é um repositório de movimento ciente de versão, para testar o diff.
type fakeMovRepoVersoes struct{ todos []model.LancamentoContabil }

func (f *fakeMovRepoVersoes) BulkInsert(context.Context, []model.LancamentoContabil) error { return nil }
func (f *fakeMovRepoVersoes) ObterProximaVersao(context.Context, time.Time) (int, error)  { return 1, nil }
func (f *fakeMovRepoVersoes) ObterVersaoAtual(context.Context, time.Time) (int, error)     { return 1, nil }
func (f *fakeMovRepoVersoes) BuscarPorDataEIndicador(context.Context, time.Time, bool) ([]model.LancamentoContabil, error) {
	return nil, nil
}
func (f *fakeMovRepoVersoes) ConsultarPaginado(context.Context, time.Time, int, int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{}, nil
}
func (f *fakeMovRepoVersoes) ConsultarPaginadoFiltradoSemCancelados(ctx context.Context, filtro model.FiltroMovimento) (*model.PaginaLancamentos, error) {
	return f.ConsultarPaginadoFiltrado(ctx, filtro)
}
func (f *fakeMovRepoVersoes) ExcluirPorDataEVersao(context.Context, time.Time, int) error { return nil }

func (f *fakeMovRepoVersoes) ConsultarPaginadoFiltrado(_ context.Context, filtro model.FiltroMovimento) (*model.PaginaLancamentos, error) {
	var out []model.LancamentoContabil
	for _, l := range f.todos {
		if !l.DataLoteContabil.Equal(filtro.DataInicio) {
			continue
		}
		if filtro.Produto != "" && l.Produto != filtro.Produto {
			continue
		}
		if filtro.Dominio != "" && l.Dominio != filtro.Dominio {
			continue
		}
		if filtro.VersaoModo == "especifica" && l.CodigoVersaoConteudo != filtro.Versao {
			continue
		}
		out = append(out, l)
	}
	return &model.PaginaLancamentos{Total: len(out), Pagina: 1, Tamanho: len(out), Lancamentos: out}, nil
}

func (f *fakeMovRepoVersoes) VersoesMovimento(_ context.Context, data time.Time, produto, dominio string) ([]int, error) {
	set := map[int]bool{}
	var vs []int
	for _, l := range f.todos {
		if l.DataLoteContabil.Equal(data) && l.Produto == produto && l.Dominio == dominio && !set[l.CodigoVersaoConteudo] {
			set[l.CodigoVersaoConteudo] = true
			vs = append(vs, l.CodigoVersaoConteudo)
		}
	}
	// ordena (poucos itens)
	for i := range vs {
		for j := i + 1; j < len(vs); j++ {
			if vs[j] < vs[i] {
				vs[i], vs[j] = vs[j], vs[i]
			}
		}
	}
	return vs, nil
}

// TestDiffVersoes_VigenteXAnterior valida o diff padrão (RF-124): adicionados, removidos e
// alterados entre a versão vigente (2) e a anterior (1) de um (data, produto, domínio).
func TestDiffVersoes_VigenteXAnterior(t *testing.T) {
	dia := d("2024-01-08")
	lan := func(v int, boleto, cd, cc string, valor float64) model.LancamentoContabil {
		return model.LancamentoContabil{
			DataLoteContabil: dia, CodigoVersaoConteudo: v, CodigoIdentificadorBoleto: boleto,
			ValorLancamentoContabil: valor, MoedaLancamentoContabil: "BRL", ContaDebito: cd, ContaCredito: cc,
			Produto: "NDF", Dominio: "Posicao", IDRegraContabil: 1,
		}
	}
	repo := &fakeMovRepoVersoes{todos: []model.LancamentoContabil{
		// versão 1
		lan(1, "B1", "1001", "2001", 100),
		lan(1, "B2", "1003", "2003", 50),
		// versão 2: B1 alterado (150), B2 removido, B3 adicionado
		lan(2, "B1", "1001", "2001", 150),
		lan(2, "B3", "1005", "2005", 70),
	}}
	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, repo, evaluator.New())

	res, err := svc.DiffVersoes(context.Background(), model.DiffVersoesInput{Data: "2024-01-08", Produto: "NDF", Dominio: "Posicao"})
	if err != nil {
		t.Fatalf("DiffVersoes: %v", err)
	}
	if res.VersaoA != 1 || res.VersaoB != 2 {
		t.Fatalf("versões: esperava A=1 B=2, obteve A=%d B=%d", res.VersaoA, res.VersaoB)
	}
	if res.TotAdicionados != 1 || res.Adicionados[0].Boleto != "B3" || res.Adicionados[0].ValorB != 70 {
		t.Errorf("adicionados inesperado: %+v", res.Adicionados)
	}
	if res.TotRemovidos != 1 || res.Removidos[0].Boleto != "B2" || res.Removidos[0].ValorA != 50 {
		t.Errorf("removidos inesperado: %+v", res.Removidos)
	}
	if res.TotAlterados != 1 || res.Alterados[0].Boleto != "B1" || res.Alterados[0].ValorA != 100 || res.Alterados[0].ValorB != 150 {
		t.Errorf("alterados inesperado: %+v", res.Alterados)
	}
}

// TestDiffVersoes_UmaVersao: com uma única versão, não há o que comparar.
func TestDiffVersoes_UmaVersao(t *testing.T) {
	dia := d("2024-01-08")
	repo := &fakeMovRepoVersoes{todos: []model.LancamentoContabil{
		{DataLoteContabil: dia, CodigoVersaoConteudo: 1, CodigoIdentificadorBoleto: "B1", ValorLancamentoContabil: 100, MoedaLancamentoContabil: "BRL", ContaDebito: "1001", ContaCredito: "2001", Produto: "NDF", Dominio: "Posicao", IDRegraContabil: 1},
	}}
	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, repo, evaluator.New())
	res, err := svc.DiffVersoes(context.Background(), model.DiffVersoesInput{Data: "2024-01-08", Produto: "NDF", Dominio: "Posicao"})
	if err != nil {
		t.Fatalf("DiffVersoes: %v", err)
	}
	if res.Aviso == "" || res.TotAdicionados != 0 || res.TotRemovidos != 0 || res.TotAlterados != 0 {
		t.Errorf("esperava aviso e zero diffs, obteve %+v", res)
	}
}

// TestDiffVersoes_SemProdutoDominio: produto/domínio são obrigatórios.
func TestDiffVersoes_SemProdutoDominio(t *testing.T) {
	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, &fakeMovRepoVersoes{}, evaluator.New())
	if _, err := svc.DiffVersoes(context.Background(), model.DiffVersoesInput{Data: "2024-01-08"}); err == nil {
		t.Fatal("esperava erro sem produto/domínio")
	}
}
