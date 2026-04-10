package service

import (
	"context"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

// ---------------------------------------------------------------------------
// Fake específico para testes de estorno
// ---------------------------------------------------------------------------

// fakeMovimentoRepoEstorno allows configuring lancamentos returned per (data, indicadorReversao)
type fakeMovimentoRepoEstorno struct {
	lancamentosD1 []model.LancamentoContabil // returned for D-1, indicadorReversao=false
	lancamentosD  []model.LancamentoContabil // returned for D, indicadorReversao=false
	inseridos     []model.LancamentoContabil // captured by BulkInsert
	versaoAtual   int
	dataD         time.Time
}

func (f *fakeMovimentoRepoEstorno) BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error) {
	if data.Equal(f.dataD) {
		return f.lancamentosD, nil
	}
	return f.lancamentosD1, nil
}

func (f *fakeMovimentoRepoEstorno) BulkInsert(ctx context.Context, lancamentos []model.LancamentoContabil) error {
	f.inseridos = append(f.inseridos, lancamentos...)
	return nil
}

func (f *fakeMovimentoRepoEstorno) ObterProximaVersao(ctx context.Context, data time.Time) (int, error) {
	f.versaoAtual++
	return f.versaoAtual, nil
}

func (f *fakeMovimentoRepoEstorno) ConsultarPaginado(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{}, nil
}

// ---------------------------------------------------------------------------
// Property 6: Invariantes do estorno — inversão de contas e indicador de reversão
// ---------------------------------------------------------------------------

// Feature: srcoff-roteirizacao-contabil-offshore, Property 6: Invariantes do estorno — inversão de contas e indicador de reversão
//
// Valida: Requisitos 5.4, 5.5
func TestP6_InvariantesEstorno(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	eval := evaluator.New()

	properties.Property("estorno inverte contas e tem indicador_reversao=true", prop.ForAll(
		func(n int, boletos []string, idRegras []int, contasDebito []string, contasCredito []string, valores []float64) bool {
			numLanc := (n % 5) + 1

			// Build unique (boleto, idRegra) keys for D-1 lancamentos
			lancamentosD1 := make([]model.LancamentoContabil, numLanc)
			for i := 0; i < numLanc; i++ {
				lancamentosD1[i] = model.LancamentoContabil{
					DataLoteContabil:          baseDate.AddDate(0, 0, -1),
					CodigoIdentificadorBoleto: boletos[i%len(boletos)],
					IDRegraContabil:           int64(idRegras[i%len(idRegras)]),
					ContaDebito:               contasDebito[i%len(contasDebito)],
					ContaCredito:              contasCredito[i%len(contasCredito)],
					ValorLancamentoContabil:   valores[i%len(valores)],
					IndicadorReversao:         false,
				}
			}

			// lancamentosD is empty — all D-1 lancamentos have no correspondent in D
			repo := &fakeMovimentoRepoEstorno{
				lancamentosD1: lancamentosD1,
				lancamentosD:  nil,
				dataD:         baseDate,
			}

			svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, repo, eval)
			err := svc.GerarEstorno(context.Background(), baseDate)
			if err != nil {
				return false
			}

			if len(repo.inseridos) == 0 {
				return false
			}

			// Build lookup: (boleto, idRegra) -> original D-1 lancamento
			type chave struct {
				boleto  string
				idRegra int64
			}
			mapaOriginal := make(map[chave]model.LancamentoContabil)
			for _, l := range lancamentosD1 {
				mapaOriginal[chave{l.CodigoIdentificadorBoleto, l.IDRegraContabil}] = l
			}

			// Verify each estorno
			for _, estorno := range repo.inseridos {
				k := chave{estorno.CodigoIdentificadorBoleto, estorno.IDRegraContabil}
				original, found := mapaOriginal[k]
				if !found {
					return false
				}

				// Contas devem estar invertidas
				if estorno.ContaDebito != original.ContaCredito {
					return false
				}
				if estorno.ContaCredito != original.ContaDebito {
					return false
				}

				// IndicadorReversao deve ser true
				if !estorno.IndicadorReversao {
					return false
				}

				// Valor deve ser igual ao original
				if estorno.ValorLancamentoContabil != original.ValorLancamentoContabil {
					return false
				}
			}

			return true
		},
		gen.IntRange(0, 4),
		gen.SliceOfN(5, gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 })),
		gen.SliceOfN(5, gen.IntRange(1, 10)),
		gen.SliceOfN(5, gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 })),
		gen.SliceOfN(5, gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 })),
		gen.SliceOfN(5, gen.Float64Range(1, 1000)),
	))

	properties.TestingRun(t)
}

// ---------------------------------------------------------------------------
// Property 7: Estorno é gerado se e somente se há divergência ou ausência de correspondente
// ---------------------------------------------------------------------------

// Feature: srcoff-roteirizacao-contabil-offshore, Property 7: Estorno é gerado se e somente se há divergência ou ausência de correspondente
//
// Valida: Requisitos 5.4, 5.6, 5.7
func TestP7_EstornoSeSomenteSeHaDivergenciaOuAusencia(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	eval := evaluator.New()

	properties.Property("estorno gerado sse divergência ou ausência de correspondente", prop.ForAll(
		func(n int, boletos []string, idRegras []int, valores []float64, cenarios []int) bool {
			numLanc := (n % 4) + 1

			// Build D-1 lancamentos with unique (boleto, idRegra) keys
			lancamentosD1 := make([]model.LancamentoContabil, numLanc)
			for i := 0; i < numLanc; i++ {
				// Use index to ensure unique keys
				boleto := boletos[i%len(boletos)]
				idRegra := int64(idRegras[i%len(idRegras)])
				// Make keys unique by appending index if needed
				_ = boleto
				lancamentosD1[i] = model.LancamentoContabil{
					DataLoteContabil:          baseDate.AddDate(0, 0, -1),
					CodigoIdentificadorBoleto: boletos[i%len(boletos)],
					IDRegraContabil:           int64(i + 1), // unique idRegra per lancamento
					ContaDebito:               "DEBITO",
					ContaCredito:              "CREDITO",
					ValorLancamentoContabil:   valores[i%len(valores)],
					IndicadorReversao:         false,
				}
				_ = idRegra
			}

			// Build D lancamentos based on scenario per lancamento
			// cenario 0 = "equal": same key + same value → NO estorno
			// cenario 1 = "divergent": same key + different value → estorno
			// cenario 2 = "absent": no entry for this key → estorno
			var lancamentosD []model.LancamentoContabil
			expectedEstornos := 0

			for i, l1 := range lancamentosD1 {
				cenario := cenarios[i%len(cenarios)] % 3
				switch cenario {
				case 0: // equal — no estorno
					lancamentosD = append(lancamentosD, model.LancamentoContabil{
						DataLoteContabil:          baseDate,
						CodigoIdentificadorBoleto: l1.CodigoIdentificadorBoleto,
						IDRegraContabil:           l1.IDRegraContabil,
						ValorLancamentoContabil:   l1.ValorLancamentoContabil, // same value
						IndicadorReversao:         false,
					})
				case 1: // divergent — estorno expected
					lancamentosD = append(lancamentosD, model.LancamentoContabil{
						DataLoteContabil:          baseDate,
						CodigoIdentificadorBoleto: l1.CodigoIdentificadorBoleto,
						IDRegraContabil:           l1.IDRegraContabil,
						ValorLancamentoContabil:   l1.ValorLancamentoContabil + 1.0, // different value
						IndicadorReversao:         false,
					})
					expectedEstornos++
				case 2: // absent — estorno expected
					// do not add to lancamentosD
					expectedEstornos++
				}
			}

			repo := &fakeMovimentoRepoEstorno{
				lancamentosD1: lancamentosD1,
				lancamentosD:  lancamentosD,
				dataD:         baseDate,
			}

			svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, repo, eval)
			err := svc.GerarEstorno(context.Background(), baseDate)

			if expectedEstornos == 0 {
				// No estornos expected — GerarEstorno returns nil without inserting
				return err == nil && len(repo.inseridos) == 0
			}

			if err != nil {
				return false
			}

			return len(repo.inseridos) == expectedEstornos
		},
		gen.IntRange(0, 3),
		gen.SliceOfN(4, gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 })),
		gen.SliceOfN(4, gen.IntRange(1, 10)),
		gen.SliceOfN(4, gen.Float64Range(1, 1000)),
		gen.SliceOfN(4, gen.IntRange(0, 2)),
	))

	properties.TestingRun(t)
}
