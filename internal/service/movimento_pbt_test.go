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
// Fakes / stubs
// ---------------------------------------------------------------------------

type fakePosicaoRepo struct {
	registros []model.PosicaoCarteira
}

func (f *fakePosicaoRepo) BuscarPorDataEVersaoMaxima(ctx context.Context, data time.Time) ([]model.PosicaoCarteira, error) {
	maxVersao := 0
	for _, r := range f.registros {
		if r.DataPosicaoCarteira.Equal(data) && r.CodigoVersaoConteudo > maxVersao {
			maxVersao = r.CodigoVersaoConteudo
		}
	}
	var result []model.PosicaoCarteira
	for _, r := range f.registros {
		if r.DataPosicaoCarteira.Equal(data) && r.CodigoVersaoConteudo == maxVersao {
			result = append(result, r)
		}
	}
	return result, nil
}

type fakeRegraRepo struct {
	regras []model.RegraContabil
}

func (f *fakeRegraRepo) ListarRegrasAtivas(ctx context.Context) ([]model.RegraContabil, error) {
	return f.regras, nil
}

type fakeMovimentoRepo struct {
	lancamentos []model.LancamentoContabil
	versaoAtual int
}

func (f *fakeMovimentoRepo) BulkInsert(ctx context.Context, lancamentos []model.LancamentoContabil) error {
	f.lancamentos = append(f.lancamentos, lancamentos...)
	return nil
}

func (f *fakeMovimentoRepo) ObterProximaVersao(ctx context.Context, data time.Time) (int, error) {
	f.versaoAtual++
	return f.versaoAtual, nil
}

func (f *fakeMovimentoRepo) BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error) {
	return nil, nil
}

func (f *fakeMovimentoRepo) ConsultarPaginado(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{}, nil
}

func (f *fakeMovimentoRepo) ObterVersaoAtual(ctx context.Context, data time.Time) (int, error) {
	return f.versaoAtual, nil
}

func (f *fakeMovimentoRepo) ConsultarPaginadoFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{}, nil
}

func (f *fakeMovimentoRepo) ConsultarPaginadoFiltradoSemCancelados(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{}, nil
}

func (f *fakeMovimentoRepo) ExcluirPorDataEVersao(ctx context.Context, data time.Time, versao int) error {
	return nil
}

// fakeMovimentoRepoTracked records the versao assigned to each BulkInsert call.
type fakeMovimentoRepoTracked struct {
	versaoAtual   int
	versoesUsadas []int
	ultimaVersao  int
	armazenados   []model.LancamentoContabil
}

func (f *fakeMovimentoRepoTracked) BulkInsert(ctx context.Context, lancamentos []model.LancamentoContabil) error {
	if len(lancamentos) > 0 {
		f.versoesUsadas = append(f.versoesUsadas, lancamentos[0].CodigoVersaoConteudo)
	}
	f.armazenados = append(f.armazenados, lancamentos...)
	return nil
}

func (f *fakeMovimentoRepoTracked) ObterProximaVersao(ctx context.Context, data time.Time) (int, error) {
	f.versaoAtual++
	f.ultimaVersao = f.versaoAtual
	return f.versaoAtual, nil
}

func (f *fakeMovimentoRepoTracked) BuscarPorDataEIndicador(ctx context.Context, data time.Time, indicadorReversao bool) ([]model.LancamentoContabil, error) {
	dataStr := data.Format("2006-01-02")
	maxPorCombo := map[string]int{}
	for _, l := range f.armazenados {
		if l.DataLoteContabil.Format("2006-01-02") != dataStr {
			continue
		}
		k := l.Produto + "\x00" + l.Dominio
		if l.CodigoVersaoConteudo > maxPorCombo[k] {
			maxPorCombo[k] = l.CodigoVersaoConteudo
		}
	}
	var out []model.LancamentoContabil
	for _, l := range f.armazenados {
		if l.DataLoteContabil.Format("2006-01-02") == dataStr &&
			l.IndicadorReversao == indicadorReversao &&
			l.CodigoVersaoConteudo == maxPorCombo[l.Produto+"\x00"+l.Dominio] {
			out = append(out, l)
		}
	}
	return out, nil
}

func (f *fakeMovimentoRepoTracked) ConsultarPaginado(ctx context.Context, data time.Time, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{}, nil
}

func (f *fakeMovimentoRepoTracked) ObterVersaoAtual(ctx context.Context, data time.Time) (int, error) {
	return f.versaoAtual, nil
}

func (f *fakeMovimentoRepoTracked) ConsultarPaginadoFiltrado(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{}, nil
}

func (f *fakeMovimentoRepoTracked) ConsultarPaginadoFiltradoSemCancelados(ctx context.Context, dataInicio, dataFim time.Time, boleto string, versao int, versaoModo string, pagina, tamanho int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{}, nil
}

func (f *fakeMovimentoRepoTracked) ExcluirPorDataEVersao(ctx context.Context, data time.Time, versao int) error {
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

var baseDate = time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

func dateOffset(days int) time.Time {
	return baseDate.AddDate(0, 0, days)
}

// campoStr lê um campo da posição dinâmica como string.
func campoStr(p model.PosicaoCarteira, campo string) string {
	if v, ok := p.Campos[campo]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// campoFloat lê um campo da posição dinâmica como float64.
func campoFloat(p model.PosicaoCarteira, campo string) float64 {
	if v, ok := p.Campos[campo]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Property 1: Seleção da versão máxima da posição de carteira
// ---------------------------------------------------------------------------

// Feature: srcoff-roteirizacao-contabil-offshore, Property 1: Seleção da versão máxima da posição de carteira
//
// Valida: Requisitos 1.1, 1.2
func TestP1_SelecaoVersaoMaxima(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	properties.Property("apenas registros da data alvo com versão máxima são selecionados", prop.ForAll(
		func(versoes []int, dateOffsets []int) bool {
			if len(versoes) == 0 {
				return true
			}

			// Build records: pair versoes[i] with dateOffsets[i%len(dateOffsets)]
			registros := make([]model.PosicaoCarteira, len(versoes))
			for i, v := range versoes {
				offset := dateOffsets[i%len(dateOffsets)]
				registros[i] = model.PosicaoCarteira{
					ID:                   int64(i + 1),
					DataPosicaoCarteira:  dateOffset(offset),
					CodigoVersaoConteudo: v,
					Campos: map[string]interface{}{
						"codigo_identificador_boleto": "BOLETO",
					},
				}
			}

			repo := &fakePosicaoRepo{registros: registros}
			result, err := repo.BuscarPorDataEVersaoMaxima(context.Background(), baseDate)
			if err != nil {
				return false
			}

			// Compute expected max versao for baseDate
			maxVersao := 0
			hasTarget := false
			for _, r := range registros {
				if r.DataPosicaoCarteira.Equal(baseDate) {
					hasTarget = true
					if r.CodigoVersaoConteudo > maxVersao {
						maxVersao = r.CodigoVersaoConteudo
					}
				}
			}

			if !hasTarget {
				return len(result) == 0
			}

			// All returned records must have baseDate and maxVersao
			for _, r := range result {
				if !r.DataPosicaoCarteira.Equal(baseDate) {
					return false
				}
				if r.CodigoVersaoConteudo != maxVersao {
					return false
				}
			}

			// Count expected records
			expectedCount := 0
			for _, r := range registros {
				if r.DataPosicaoCarteira.Equal(baseDate) && r.CodigoVersaoConteudo == maxVersao {
					expectedCount++
				}
			}
			return len(result) == expectedCount
		},
		gen.SliceOfN(5, gen.IntRange(1, 5)),
		gen.SliceOfN(3, gen.IntRange(0, 2)),
	))

	properties.TestingRun(t)
}

// ---------------------------------------------------------------------------
// Property 3: Lançamentos gerados correspondem às condições satisfeitas
// ---------------------------------------------------------------------------

// Feature: srcoff-roteirizacao-contabil-offshore, Property 3: Lançamentos gerados correspondem às condições satisfeitas
//
// Valida: Requisitos 3.1, 4.5
func TestP3_LancamentosCorrespondemCondicoesSatisfeitas(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	eval := evaluator.New()

	properties.Property("número de lançamentos == pares (posição, condição) satisfeitos", prop.ForAll(
		func(valorMTMs []float64, conditionFlags []bool) bool {
			if len(valorMTMs) == 0 {
				return true
			}

			// Build posicoes
			posicoes := make([]model.PosicaoCarteira, len(valorMTMs))
			for i, v := range valorMTMs {
				posicoes[i] = model.PosicaoCarteira{
					ID:                   int64(i + 1),
					DataPosicaoCarteira:  baseDate,
					CodigoVersaoConteudo: 1,
					Campos: map[string]interface{}{
						"codigo_identificador_boleto":  "BOLETO",
						"valor_mtm":                    v,
						"moeda_principal_remanescente": "USD",
					},
				}
			}

			// Build condicoes: alternate between "valor_mtm > 0" and "valor_mtm < 0"
			numCondicoes := (len(conditionFlags) % 3) + 1
			condicoes := make([]model.CondicaoRegra, numCondicoes)
			for i := 0; i < numCondicoes; i++ {
				expr := "valor_mtm > 0"
				if conditionFlags[i%len(conditionFlags)] {
					expr = "valor_mtm < 0"
				}
				condicoes[i] = model.CondicaoRegra{
					ID:           int64(i + 1),
					Condicao:     expr,
					ContaDebito:  "1001",
					ContaCredito: "2001",
					CampoValor:   "valor_mtm",
					CampoMoeda:   "moeda_principal_remanescente",
					Ativo:        true,
				}
			}

			regra := model.RegraContabil{
				ID:        1,
				Descricao: "Regra Teste",
				Ativo:     true,
				Condicoes: condicoes,
			}

			// Count expected pairs
			expected := 0
			for _, p := range posicoes {
				env := evaluator.PosicaoToEnv(p)
				for _, c := range condicoes {
					ok, err := eval.EvaluateCondition(c.Condicao, env)
					if err == nil && ok {
						expected++
					}
				}
			}

			posRepo := &fakePosicaoRepo{registros: posicoes}
			regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regra}}
			movRepo := &fakeMovimentoRepo{}

			svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval)
			_, err := svc.GerarMovimento(context.Background(), baseDate)
			if err != nil && expected == 0 {
				// No posicoes for date or no lancamentos — acceptable if posicoes exist
				// but GerarMovimento returns error only when posicoes is empty
				return false
			}
			if err != nil {
				return false
			}

			return len(movRepo.lancamentos) == expected
		},
		gen.SliceOfN(4, gen.Float64Range(-1000, 1000)),
		gen.SliceOfN(3, gen.Bool()),
	))

	properties.TestingRun(t)
}

// ---------------------------------------------------------------------------
// Property 4: Campos do lançamento contábil são preenchidos corretamente
// ---------------------------------------------------------------------------

// Feature: srcoff-roteirizacao-contabil-offshore, Property 4: Campos do lançamento contábil são preenchidos corretamente
//
// Valida: Requisitos 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 3.8, 3.9, 3.10
func TestP4_CamposLancamentoPreenchidosCorretamente(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	eval := evaluator.New()

	properties.Property("campos do lançamento correspondem à posição e condição de origem", prop.ForAll(
		func(valorMTMs []float64, boletos []string, moedas []string, contasDebito []string, contasCredito []string) bool {
			if len(valorMTMs) == 0 || len(boletos) == 0 || len(moedas) == 0 {
				return true
			}

			// Build posicoes with positive ValorMTM so condition "valor_mtm > 0" is always true
			numPos := (len(valorMTMs) % 3) + 1
			posicoes := make([]model.PosicaoCarteira, numPos)
			for i := 0; i < numPos; i++ {
				v := valorMTMs[i%len(valorMTMs)]
				if v <= 0 {
					v = -v + 1 // ensure positive
				}
				posicoes[i] = model.PosicaoCarteira{
					ID:                   int64(i + 1),
					DataPosicaoCarteira:  baseDate,
					CodigoVersaoConteudo: 1,
					Campos: map[string]interface{}{
						"codigo_identificador_boleto":  boletos[i%len(boletos)],
						"valor_mtm":                    v,
						"moeda_principal_remanescente": moedas[i%len(moedas)],
					},
				}
			}

			// Build condicoes with condition always true
			numCond := (len(contasDebito) % 2) + 1
			condicoes := make([]model.CondicaoRegra, numCond)
			for i := 0; i < numCond; i++ {
				condicoes[i] = model.CondicaoRegra{
					ID:           int64(i + 1),
					Condicao:     "valor_mtm > 0",
					ContaDebito:  contasDebito[i%len(contasDebito)],
					ContaCredito: contasCredito[i%len(contasCredito)],
					CampoValor:   "valor_mtm",
					CampoMoeda:   "moeda_principal_remanescente",
					Ativo:        true,
				}
			}

			regra := model.RegraContabil{
				ID:        1,
				Descricao: "Regra P4",
				Ativo:     true,
				Condicoes: condicoes,
			}

			posRepo := &fakePosicaoRepo{registros: posicoes}
			regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regra}}
			movRepo := &fakeMovimentoRepo{}

			svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval)
			if _, err := svc.GerarMovimento(context.Background(), baseDate); err != nil {
				return false
			}

			// Build lookup: (boleto, condID) -> (posicao, condicao)
			type key struct {
				boleto string
				condID int64
			}
			type pair struct {
				pos  model.PosicaoCarteira
				cond model.CondicaoRegra
			}
			lookup := make(map[key]pair)
			for _, p := range posicoes {
				for _, c := range condicoes {
					lookup[key{campoStr(p, "codigo_identificador_boleto"), c.ID}] = pair{p, c}
				}
			}

			// Verify each lancamento
			for _, l := range movRepo.lancamentos {
				// Find matching posicao
				var matchPos *model.PosicaoCarteira
				for i := range posicoes {
					if campoStr(posicoes[i], "codigo_identificador_boleto") == l.CodigoIdentificadorBoleto {
						matchPos = &posicoes[i]
						break
					}
				}
				if matchPos == nil {
					return false
				}

				// Find matching condicao by conta_debito + conta_credito
				var matchCond *model.CondicaoRegra
				for i := range condicoes {
					if condicoes[i].ContaDebito == l.ContaDebito && condicoes[i].ContaCredito == l.ContaCredito {
						matchCond = &condicoes[i]
						break
					}
				}
				if matchCond == nil {
					return false
				}

				if l.ContaDebito != matchCond.ContaDebito {
					return false
				}
				if l.ContaCredito != matchCond.ContaCredito {
					return false
				}
				if l.ValorLancamentoContabil != campoFloat(*matchPos, "valor_mtm") {
					return false
				}
				if l.MoedaLancamentoContabil != campoStr(*matchPos, "moeda_principal_remanescente") {
					return false
				}
				if l.CodigoIdentificadorBoleto != campoStr(*matchPos, "codigo_identificador_boleto") {
					return false
				}
				if l.IndicadorReversao != false {
					return false
				}
			}

			return true
		},
		gen.SliceOfN(3, gen.Float64Range(1, 1000)),
		gen.SliceOfN(3, gen.AlphaString()),
		gen.SliceOfN(3, gen.AlphaString()),
		gen.SliceOfN(2, gen.AlphaString()),
		gen.SliceOfN(2, gen.AlphaString()),
	))

	properties.TestingRun(t)
}

// ---------------------------------------------------------------------------
// Property 5: Versão do lote é sempre incrementada monotonicamente
// ---------------------------------------------------------------------------

// Feature: srcoff-roteirizacao-contabil-offshore, Property 5: Versão do lote é sempre incrementada monotonicamente
//
// Valida: Requisito 3.11
func TestP5_VersaoLoteIncrementadaMonotonicamente(t *testing.T) {
	params := gopter.DefaultTestParameters()
	params.MinSuccessfulTests = 100

	properties := gopter.NewProperties(params)

	eval := evaluator.New()

	properties.Property("versões dos lotes são estritamente crescentes e a primeira é 1", prop.ForAll(
		func(numChamadas int) bool {
			n := (numChamadas % 4) + 2 // 2 to 5 calls

			posicao := model.PosicaoCarteira{
				ID:                   1,
				DataPosicaoCarteira:  baseDate,
				CodigoVersaoConteudo: 1,
				Campos: map[string]interface{}{
					"codigo_identificador_boleto":  "BOLETO001",
					"valor_mtm":                    100.0,
					"moeda_principal_remanescente": "USD",
				},
			}

			condicao := model.CondicaoRegra{
				ID:           1,
				Condicao:     "valor_mtm > 0",
				ContaDebito:  "1001",
				ContaCredito: "2001",
				CampoValor:   "valor_mtm",
				CampoMoeda:   "moeda_principal_remanescente",
				Ativo:        true,
			}

			regra := model.RegraContabil{
				ID:        1,
				Descricao: "Regra P5",
				Ativo:     true,
				Condicoes: []model.CondicaoRegra{condicao},
			}

			posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{posicao}}
			regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regra}}
			movRepo := &fakeMovimentoRepoTracked{}

			svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval)

			for i := 0; i < n; i++ {
				if _, err := svc.GerarMovimento(context.Background(), baseDate); err != nil {
					return false
				}
			}

			if len(movRepo.versoesUsadas) != n {
				return false
			}

			// First versao must be 1
			if movRepo.versoesUsadas[0] != 1 {
				return false
			}

			// Each subsequent versao must be strictly greater than the previous
			for i := 1; i < len(movRepo.versoesUsadas); i++ {
				if movRepo.versoesUsadas[i] <= movRepo.versoesUsadas[i-1] {
					return false
				}
			}

			return true
		},
		gen.IntRange(0, 10),
	))

	properties.TestingRun(t)
}

// TestVersaoIndependentePorCombinacao garante que a versão do movimento contábil é
// por (produto, domínio): a primeira execução de um novo produto/domínio na data começa
// na versão 1, mesmo que outras combinações já tenham versões maiores na mesma data.
func TestVersaoIndependentePorCombinacao(t *testing.T) {
	eval := evaluator.New()

	novaPosicao := func(produto, dominio string) model.PosicaoCarteira {
		return model.PosicaoCarteira{
			DataPosicaoCarteira:  baseDate,
			CodigoVersaoConteudo: 1,
			Campos: map[string]interface{}{
				"produto":                      produto,
				"dominio":                      dominio,
				"codigo_identificador_boleto":  "BOL-" + produto,
				"valor_mtm":                    100.0,
				"moeda_principal_remanescente": "USD",
			},
		}
	}

	regra := model.RegraContabil{
		ID: 1, Descricao: "Regra geral", Ativo: true,
		Condicoes: []model.CondicaoRegra{{
			ID: 1, Condicao: "valor_mtm > 0",
			ContaDebito: "1001", ContaCredito: "2001",
			CampoValor: "valor_mtm", CampoMoeda: "moeda_principal_remanescente", Ativo: true,
		}},
	}

	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		novaPosicao("NDF", "Posição"),
		novaPosicao("SWAP", "Posição"),
	}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{regra}}
	movRepo := &fakeMovimentoRepoTracked{}
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval)

	ctx := context.Background()
	// NDF processado duas vezes → versões 1 e 2 para NDF.
	if _, err := svc.GerarMovimentoEscopo(ctx, baseDate, "NDF", "Posição"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GerarMovimentoEscopo(ctx, baseDate, "NDF", "Posição"); err != nil {
		t.Fatal(err)
	}
	// SWAP processado pela primeira vez → deve ser versão 1 (e não 3).
	if _, err := svc.GerarMovimentoEscopo(ctx, baseDate, "SWAP", "Posição"); err != nil {
		t.Fatal(err)
	}

	maxVersao := func(produto string) int {
		m := 0
		for _, l := range movRepo.armazenados {
			if l.Produto == produto && l.CodigoVersaoConteudo > m {
				m = l.CodigoVersaoConteudo
			}
		}
		return m
	}
	if got := maxVersao("SWAP"); got != 1 {
		t.Fatalf("SWAP na primeira execução deveria ser versão 1, obteve %d", got)
	}
	if got := maxVersao("NDF"); got != 2 {
		t.Fatalf("NDF deveria estar na versão 2 após dois processamentos, obteve %d", got)
	}
}
