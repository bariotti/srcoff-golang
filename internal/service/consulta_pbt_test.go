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
// Fake para testes de paginação
// ---------------------------------------------------------------------------

// fakeMovimentoRepoPaginado simulates a repository with a fixed set of lancamentos
// and implements real pagination logic in memory.
type fakeMovimentoRepoPaginado struct {
	todos []model.LancamentoContabil
}

func (f *fakeMovimentoRepoPaginado) BulkInsert(ctx context.Context, lancamentos []model.LancamentoContabil) error {
	f.todos = append(f.todos, lancamentos...)
	return nil
}

func (f *fakeMovimentoRepoPaginado) ObterProximaVersao(ctx context.Context, data time.Time) (int, error) {
	return 1, nil
}

func (f *fakeMovimentoRepoPaginado) BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error) {
	var result []model.LancamentoContabil
	for _, l := range f.todos {
		if l.DataLoteContabil.Equal(data) && l.IndicadorReversao == indicadorReversao {
			result = append(result, l)
		}
	}
	return result, nil
}

func (f *fakeMovimentoRepoPaginado) ConsultarPaginado(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	// Filter by date
	var filtered []model.LancamentoContabil
	for _, l := range f.todos {
		if l.DataLoteContabil.Equal(data) {
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
	return &model.PaginaLancamentos{
		Total:       total,
		Pagina:      pagina,
		Tamanho:     tamanho,
		Lancamentos: filtered[offset:end],
	}, nil
}

// ---------------------------------------------------------------------------
// Property 8: Lote consolidado contém exatamente todos os lançamentos e estornos da data
// ---------------------------------------------------------------------------

// Feature: srcoff-roteirizacao-contabil-offshore, Property 8: Lote consolidado contém exatamente todos os lançamentos e estornos da data
//
// Valida: Requisitos 6.1, 6.2
func TestP8_LoteConsolidadoContemExatamenteTodosLancamentosEEstornos(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	eval := evaluator.New()

	properties.Property("consulta retorna exatamente lançamentos + estornos sem omissões ou duplicatas", prop.ForAll(
		func(n, m int) bool {
			numLanc := (n % 10) + 1  // 1-10 lançamentos
			numEstorno := m % 6      // 0-5 estornos

			repo := &fakeMovimentoRepoPaginado{}

			// Pre-populate with lançamentos (indicadorReversao=false)
			idCounter := int64(1)
			insertedIDs := make(map[int64]bool)

			for i := 0; i < numLanc; i++ {
				l := model.LancamentoContabil{
					ID:               idCounter,
					DataLoteContabil: baseDate,
					IndicadorReversao: false,
					CodigoIdentificadorBoleto: "BOLETO",
					ValorLancamentoContabil:   float64(i + 1),
				}
				repo.todos = append(repo.todos, l)
				insertedIDs[idCounter] = true
				idCounter++
			}

			// Pre-populate with estornos (indicadorReversao=true)
			for i := 0; i < numEstorno; i++ {
				l := model.LancamentoContabil{
					ID:               idCounter,
					DataLoteContabil: baseDate,
					IndicadorReversao: true,
					CodigoIdentificadorBoleto: "BOLETO",
					ValorLancamentoContabil:   float64(i + 1),
				}
				repo.todos = append(repo.todos, l)
				insertedIDs[idCounter] = true
				idCounter++
			}

			svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, repo, eval)

			// Use a large page size to get all records at once
			result, err := svc.ConsultarLancamentos(context.Background(), baseDate, 1, 1000)
			if err != nil {
				return false
			}

			expectedTotal := numLanc + numEstorno

			// Total must match
			if result.Total != expectedTotal {
				return false
			}

			// Number of returned lancamentos must match
			if len(result.Lancamentos) != expectedTotal {
				return false
			}

			// No duplicates: all IDs must be unique
			seenIDs := make(map[int64]bool)
			for _, l := range result.Lancamentos {
				if seenIDs[l.ID] {
					return false // duplicate
				}
				seenIDs[l.ID] = true
			}

			// Set of returned IDs must equal set of inserted IDs
			if len(seenIDs) != len(insertedIDs) {
				return false
			}
			for id := range insertedIDs {
				if !seenIDs[id] {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 10),
		gen.IntRange(0, 5),
	))

	properties.TestingRun(t)
}

// ---------------------------------------------------------------------------
// Property 9: Paginação retorna subconjunto correto e total consistente
// ---------------------------------------------------------------------------

// Feature: srcoff-roteirizacao-contabil-offshore, Property 9: Paginação retorna subconjunto correto e total consistente
//
// Valida: Requisitos 9.2, 9.3
func TestP9_PaginacaoRetornaSubconjuntoCorretoETotalConsistente(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	eval := evaluator.New()

	properties.Property("união de todas as páginas == conjunto completo, sem duplicatas, total consistente", prop.ForAll(
		func(n, pageSize int) bool {
			numLanc := (n % 20) + 1  // 1-20
			tamPagina := (pageSize % 5) + 1 // 1-5

			repo := &fakeMovimentoRepoPaginado{}

			// Pre-populate with N lançamentos with unique IDs
			insertedIDs := make(map[int64]bool)
			for i := 0; i < numLanc; i++ {
				id := int64(i + 1)
				l := model.LancamentoContabil{
					ID:               id,
					DataLoteContabil: baseDate,
					IndicadorReversao: false,
					CodigoIdentificadorBoleto: "BOLETO",
					ValorLancamentoContabil:   float64(i + 1),
				}
				repo.todos = append(repo.todos, l)
				insertedIDs[id] = true
			}

			svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, repo, eval)

			// Iterate all pages
			var allLancamentos []model.LancamentoContabil
			consistentTotal := -1
			pagina := 1

			for {
				result, err := svc.ConsultarLancamentos(context.Background(), baseDate, pagina, tamPagina)
				if err != nil {
					return false
				}

				// Total must be consistent across all pages
				if consistentTotal == -1 {
					consistentTotal = result.Total
				} else if result.Total != consistentTotal {
					return false
				}

				if len(result.Lancamentos) == 0 {
					break
				}

				allLancamentos = append(allLancamentos, result.Lancamentos...)
				pagina++

				// Safety: avoid infinite loop
				if pagina > numLanc+2 {
					break
				}
			}

			// Total must equal N
			if consistentTotal != numLanc {
				return false
			}

			// Union of all pages must equal complete set (no omissions)
			if len(allLancamentos) != numLanc {
				return false
			}

			// No duplicates across pages
			seenIDs := make(map[int64]bool)
			for _, l := range allLancamentos {
				if seenIDs[l.ID] {
					return false // duplicate
				}
				seenIDs[l.ID] = true
			}

			// All inserted IDs must be present
			for id := range insertedIDs {
				if !seenIDs[id] {
					return false
				}
			}

			return true
		},
		gen.IntRange(1, 20),
		gen.IntRange(1, 5),
	))

	properties.TestingRun(t)
}
