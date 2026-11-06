package service

import (
	"context"
	"testing"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
)

// fakeMovRepoAnalise implementa movimentoContabilRepo filtrando por PERÍODO (di<=data<=df)
// + produto/domínio — para testar Balancete e Série de saldo.
type fakeMovRepoAnalise struct{ todos []model.LancamentoContabil }

func (f *fakeMovRepoAnalise) BulkInsert(context.Context, []model.LancamentoContabil) error { return nil }
func (f *fakeMovRepoAnalise) ObterProximaVersao(context.Context, time.Time) (int, error)   { return 1, nil }
func (f *fakeMovRepoAnalise) ObterVersaoAtual(context.Context, time.Time) (int, error)      { return 1, nil }
func (f *fakeMovRepoAnalise) BuscarPorDataEIndicador(context.Context, time.Time, bool) ([]model.LancamentoContabil, error) {
	return nil, nil
}
func (f *fakeMovRepoAnalise) ConsultarPaginado(context.Context, time.Time, int, int) (*model.PaginaLancamentos, error) {
	return &model.PaginaLancamentos{}, nil
}
func (f *fakeMovRepoAnalise) ConsultarPaginadoFiltradoSemCancelados(ctx context.Context, filtro model.FiltroMovimento) (*model.PaginaLancamentos, error) {
	return f.ConsultarPaginadoFiltrado(ctx, filtro)
}
func (f *fakeMovRepoAnalise) ExcluirPorDataEVersao(context.Context, time.Time, int) error { return nil }
func (f *fakeMovRepoAnalise) VersoesMovimento(context.Context, time.Time, string, string) ([]int, error) {
	return nil, nil
}
func (f *fakeMovRepoAnalise) ConsultarPaginadoFiltrado(_ context.Context, filtro model.FiltroMovimento) (*model.PaginaLancamentos, error) {
	var out []model.LancamentoContabil
	for _, l := range f.todos {
		d := l.DataLoteContabil
		if d.Before(filtro.DataInicio) || d.After(filtro.DataFim) {
			continue
		}
		if filtro.Produto != "" && l.Produto != filtro.Produto {
			continue
		}
		if filtro.Dominio != "" && l.Dominio != filtro.Dominio {
			continue
		}
		out = append(out, l)
	}
	return &model.PaginaLancamentos{Total: len(out), Lancamentos: out}, nil
}

func TestBalancete(t *testing.T) {
	dia := d("2024-01-08")
	repo := &fakeMovRepoAnalise{todos: []model.LancamentoContabil{
		{DataLoteContabil: dia, ContaDebito: "1001", ContaCredito: "2001", ValorLancamentoContabil: 100},
		{DataLoteContabil: dia, ContaDebito: "1001", ContaCredito: "3001", ValorLancamentoContabil: 50},
	}}
	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, repo, evaluator.New())
	b, err := svc.Balancete(context.Background(), dia, dia, "", "", "vigente")
	if err != nil {
		t.Fatalf("Balancete: %v", err)
	}
	if b.TotalDebito != 150 || b.TotalCredito != 150 || !b.Balanceado {
		t.Fatalf("totais: déb=%v créd=%v balanceado=%v", b.TotalDebito, b.TotalCredito, b.Balanceado)
	}
	porConta := map[string]model.BalanceteConta{}
	for _, c := range b.Contas {
		porConta[c.Conta] = c
	}
	if c := porConta["1001"]; c.Debito != 150 || c.Credito != 0 || c.Saldo != 150 {
		t.Errorf("1001 inesperada: %+v", c)
	}
	if c := porConta["2001"]; c.Credito != 100 {
		t.Errorf("2001 crédito esperado 100, obteve %v", c.Credito)
	}
}

func TestSerieSaldo(t *testing.T) {
	d1, d2 := d("2024-01-08"), d("2024-01-09")
	repo := &fakeMovRepoAnalise{todos: []model.LancamentoContabil{
		{DataLoteContabil: d1, ContaDebito: "1001", ContaCredito: "2001", ValorLancamentoContabil: 100}, // 1001 +100
		{DataLoteContabil: d2, ContaDebito: "2001", ContaCredito: "1001", ValorLancamentoContabil: 30},  // 1001 -30
	}}
	svc := NewMovimentoContabilService(&fakePosicaoRepo{}, &fakeRegraRepo{}, repo, evaluator.New())
	s, err := svc.SerieSaldoConta(context.Background(), d1, d2, "1001", "", "")
	if err != nil {
		t.Fatalf("SerieSaldoConta: %v", err)
	}
	if len(s.Pontos) != 2 {
		t.Fatalf("esperava 2 pontos, obteve %d", len(s.Pontos))
	}
	if s.Pontos[0].Net != 100 || s.Pontos[0].Acumulado != 100 {
		t.Errorf("ponto 1 inesperado: %+v", s.Pontos[0])
	}
	if s.Pontos[1].Net != -30 || s.Pontos[1].Acumulado != 70 {
		t.Errorf("ponto 2 inesperado: %+v", s.Pontos[1])
	}
}

func TestCoberturaPosicoes(t *testing.T) {
	dia := d("2024-01-08")
	pos := func(boleto string, campos map[string]interface{}) model.PosicaoCarteira {
		base := map[string]interface{}{"produto": "NDF", "dominio": "Posicao", "codigo_identificador_boleto": boleto}
		for k, v := range campos {
			base[k] = v
		}
		return model.PosicaoCarteira{ID: 1, DataPosicaoCarteira: dia, CodigoVersaoConteudo: 1, Campos: base}
	}
	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{
		pos("B-A", map[string]interface{}{"valor_mtm": 100.0}), // condição verdadeira → coberto
		pos("B-B", map[string]interface{}{"valor_mtm": -5.0}),  // condição falsa → órfão
		pos("B-C", map[string]interface{}{}),                   // campo ausente → inconsistente
	}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{{
		ID: 1, Descricao: "R", CodigoProdutoCorporativo: "NDF", Dominio: "Posicao", TipoLancamento: model.TipoReverte,
		Condicoes: []model.CondicaoRegra{{Condicao: "valor_mtm > 0", ContaDebito: "1001", ContaCredito: "2001", CampoValor: "valor_mtm", CampoBoleto: "codigo_identificador_boleto"}},
	}}}
	svc := NewMovimentoContabilService(posRepo, regraRepo, &fakeMovimentoRepo{}, evaluator.New())
	c, err := svc.CoberturaPosicoes(context.Background(), dia, "", "")
	if err != nil {
		t.Fatalf("CoberturaPosicoes: %v", err)
	}
	if c.TotalPosicoes != 3 || c.Cobertos != 1 || c.Orfaos != 1 || c.Inconsistentes != 1 {
		t.Fatalf("cobertura inesperada: total=%d cob=%d orf=%d inc=%d", c.TotalPosicoes, c.Cobertos, c.Orfaos, c.Inconsistentes)
	}
	if len(c.BoletosOrfaos) != 1 || c.BoletosOrfaos[0].Boleto != "B-B" {
		t.Errorf("órfão inesperado: %+v", c.BoletosOrfaos)
	}
}

// fakeRegraFull implementa regraContabilRepoFull para testar o Lint (só ListarRegrasAtivas importa).
type fakeRegraFull struct{ regras []model.RegraContabil }

func (f *fakeRegraFull) ListarRegrasAtivas(context.Context) ([]model.RegraContabil, error) {
	return f.regras, nil
}
func (f *fakeRegraFull) ListarRegras(context.Context) ([]model.RegraContabil, error) {
	return f.regras, nil
}
func (f *fakeRegraFull) CriarRegra(context.Context, model.RegraContabil) (int64, error) { return 0, nil }
func (f *fakeRegraFull) EditarRegra(context.Context, model.RegraContabil) error         { return nil }
func (f *fakeRegraFull) ExcluirRegra(context.Context, int64) error                      { return nil }
func (f *fakeRegraFull) ListarCondicoes(context.Context, int64) ([]model.CondicaoRegra, error) {
	return nil, nil
}
func (f *fakeRegraFull) ListarCondicoesVisiveis(context.Context, int64) ([]model.CondicaoRegra, error) {
	return nil, nil
}
func (f *fakeRegraFull) CriarCondicao(context.Context, model.CondicaoRegra) (int64, error) { return 0, nil }
func (f *fakeRegraFull) EditarCondicao(context.Context, model.CondicaoRegra) error         { return nil }
func (f *fakeRegraFull) ExcluirCondicao(context.Context, int64) error                      { return nil }

// capturaRegraRepo captura a última regra/condição recebida, para verificar a normalização.
type capturaRegraRepo struct {
	fakeRegraFull
	regra model.RegraContabil
	cond  model.CondicaoRegra
}

func (c *capturaRegraRepo) CriarRegra(_ context.Context, r model.RegraContabil) (int64, error) {
	c.regra = r
	return 1, nil
}
func (c *capturaRegraRepo) CriarCondicao(_ context.Context, cd model.CondicaoRegra) (int64, error) {
	c.cond = cd
	return 1, nil
}

func TestCadastroNormalizaCampos(t *testing.T) {
	repo := &capturaRegraRepo{}
	svc := NewRegraContabilService(repo)

	// Regra: campo_produto (nome) e pré-condição (expressão) com acento/maiúsculas.
	_, err := svc.CriarRegra(context.Background(), model.RegraContabil{
		Descricao: "R", CampoProduto: "Produto Corp", PreCondicao: "TIPO == \"X\"",
	})
	if err != nil {
		t.Fatalf("CriarRegra: %v", err)
	}
	if repo.regra.CampoProduto != "produto_corp" {
		t.Errorf("campo_produto não normalizado: %q", repo.regra.CampoProduto)
	}
	if repo.regra.PreCondicao != "tipo == \"X\"" {
		t.Errorf("pré-condição não normalizada: %q", repo.regra.PreCondicao)
	}

	// Condição: expressões (condicao/campo_valor) e nomes de coluna (moeda/boleto) normalizados.
	_, err = svc.CriarCondicao(context.Background(), model.CondicaoRegra{
		Condicao: "VALOR_MTM > 0", CampoValor: "Valor_MTM", ContaDebito: "1", ContaCredito: "2",
		CampoMoeda: "Moeda", CampoBoleto: "Codigo Boleto", BaseNaPosicao: true,
		CampoValorMoedaBase: "Valor_Base", CampoMoedaBase: "Moeda Base",
	})
	if err != nil {
		t.Fatalf("CriarCondicao: %v", err)
	}
	if repo.cond.Condicao != "valor_mtm > 0" || repo.cond.CampoValor != "valor_mtm" {
		t.Errorf("expressões não normalizadas: cond=%q campo_valor=%q", repo.cond.Condicao, repo.cond.CampoValor)
	}
	if repo.cond.CampoMoeda != "moeda" || repo.cond.CampoBoleto != "codigo_boleto" {
		t.Errorf("nomes não normalizados: moeda=%q boleto=%q", repo.cond.CampoMoeda, repo.cond.CampoBoleto)
	}
	if repo.cond.CampoValorMoedaBase != "valor_base" || repo.cond.CampoMoedaBase != "moeda_base" {
		t.Errorf("base não normalizada: valor_base=%q moeda_base=%q", repo.cond.CampoValorMoedaBase, repo.cond.CampoMoedaBase)
	}
}

func TestLintRegras(t *testing.T) {
	repo := &fakeRegraFull{regras: []model.RegraContabil{
		{ID: 1, Descricao: "Sem condição", CodigoProdutoCorporativo: "NDF", Dominio: "Posicao"}, // aviso: sem_condicao
		{ID: 2, Descricao: "Contas iguais", CodigoProdutoCorporativo: "NDF", Dominio: "Posicao", Condicoes: []model.CondicaoRegra{
			{Condicao: "valor_mtm > 0", ContaDebito: "9999", ContaCredito: "9999", CampoValor: "valor_mtm"}, // erro: contas_iguais
		}},
		{ID: 3, Descricao: "Expressão inválida", CodigoProdutoCorporativo: "NDF", Dominio: "Posicao", Condicoes: []model.CondicaoRegra{
			{Condicao: "valor_mtm >", ContaDebito: "1001", ContaCredito: "2001", CampoValor: "valor_mtm"}, // erro: condicao_invalida
		}},
	}}
	svc := NewRegraContabilService(repo)
	res, err := svc.Lint(context.Background())
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	tipos := map[string]bool{}
	for _, a := range res.Achados {
		tipos[a.Tipo] = true
	}
	for _, esperado := range []string{"sem_condicao", "contas_iguais", "condicao_invalida"} {
		if !tipos[esperado] {
			t.Errorf("esperava achado %q; achados: %+v", esperado, res.Achados)
		}
	}
}

type fakeSaldoRepo struct{ saldos []model.SaldoConta }

func (f *fakeSaldoRepo) AplicarDeltas(context.Context, []model.SaldoConta) error { return nil }
func (f *fakeSaldoRepo) SubstituirCombos(context.Context, []model.ProdutoDominio, []model.SaldoConta) error {
	return nil
}
func (f *fakeSaldoRepo) Listar(context.Context, model.SaldoFiltro) ([]model.SaldoConta, error) {
	return f.saldos, nil
}
func (f *fakeSaldoRepo) ListarPaginado(context.Context, model.SaldoFiltro, int, int) (*model.SaldoPagina, error) {
	return &model.SaldoPagina{}, nil
}
func (f *fakeSaldoRepo) EstaVazio(context.Context) (bool, error) { return len(f.saldos) == 0, nil }

// TestConciliar valida a conciliação única (RF-131): Saldo×Posição (ok/divergente/sem_posicao)
// e "na posição sem contábil" (órfãos), numa execução só.
func TestConciliar(t *testing.T) {
	dia := d("2024-01-08")
	pos := func(boleto string, vmtm float64) model.PosicaoCarteira {
		return model.PosicaoCarteira{ID: 1, DataPosicaoCarteira: dia, CodigoVersaoConteudo: 1, Campos: map[string]interface{}{
			"produto": "NDF", "dominio": "Posicao", "codigo_identificador_boleto": boleto, "valor_mtm": vmtm}}
	}
	// B-PRE (produto SWAP, só a regra 2 aplica) está na posição mas a pré-condição é falsa (tipo != "X") → nada a lançar.
	posPre := model.PosicaoCarteira{ID: 1, DataPosicaoCarteira: dia, CodigoVersaoConteudo: 1, Campos: map[string]interface{}{
		"produto": "SWAP", "dominio": "Posicao", "codigo_identificador_boleto": "B-PRE", "valor_mtm": 300.0, "tipo": "Y"}}
	// B-OK/B-DIV/B-ORFAO estão na posição; B-SEMPOS só tem saldo.
	posRepo := &fakePosicaoRepo{registros: []model.PosicaoCarteira{pos("B-OK", 100), pos("B-DIV", 200), pos("B-ORFAO", 80), posPre}}
	regraRepo := &fakeRegraRepo{regras: []model.RegraContabil{{
		ID: 1, CodigoProdutoCorporativo: "NDF", Dominio: "Posicao", TipoLancamento: model.TipoReverte,
		Condicoes: []model.CondicaoRegra{{Condicao: "valor_mtm > 0", ContaDebito: "1001", ContaCredito: "2001", CampoValor: "valor_mtm", CampoBoleto: "codigo_identificador_boleto"}},
	}, {
		ID: 2, CodigoProdutoCorporativo: "SWAP", Dominio: "Posicao", TipoLancamento: model.TipoReverte, PreCondicao: "tipo == \"X\"",
		Condicoes: []model.CondicaoRegra{{Condicao: "valor_mtm > 0", ContaDebito: "3001", ContaCredito: "4001", CampoValor: "valor_mtm", CampoBoleto: "codigo_identificador_boleto"}},
	}}}
	// Movimento tem B-OK e B-DIV (logo não são órfãos); B-ORFAO não tem contábil.
	movRepo := &fakeMovRepoAnalise{todos: []model.LancamentoContabil{
		{DataLoteContabil: dia, Produto: "NDF", Dominio: "Posicao", CodigoIdentificadorBoleto: "B-OK", ValorLancamentoContabil: 100},
		{DataLoteContabil: dia, Produto: "NDF", Dominio: "Posicao", CodigoIdentificadorBoleto: "B-DIV", ValorLancamentoContabil: 150},
	}}
	saldoRepo := &fakeSaldoRepo{saldos: []model.SaldoConta{
		{Produto: "NDF", Dominio: "Posicao", CodigoIdentificadorBoleto: "B-OK", IDRegraContabil: 1, ContaDebito: "1001", ContaCredito: "2001", Saldo: 100},    // == posição → ok
		{Produto: "NDF", Dominio: "Posicao", CodigoIdentificadorBoleto: "B-DIV", IDRegraContabil: 1, ContaDebito: "1001", ContaCredito: "2001", Saldo: 150},   // 150 ≠ 200 → divergente
		{Produto: "NDF", Dominio: "Posicao", CodigoIdentificadorBoleto: "B-SEMPOS", IDRegraContabil: 1, ContaDebito: "1001", ContaCredito: "2001", Saldo: 50}, // sem posição
		{Produto: "SWAP", Dominio: "Posicao", CodigoIdentificadorBoleto: "B-PRE", IDRegraContabil: 2, ContaDebito: "3001", ContaCredito: "4001", Saldo: 0},   // pré-condição falsa → esperado 0 → ok
	}}
	svc := NewMovimentoContabilService(posRepo, regraRepo, movRepo, evaluator.New()).ComSaldoRepo(saldoRepo)
	c, err := svc.Conciliar(context.Background(), dia, "", "")
	if err != nil {
		t.Fatalf("Conciliar: %v", err)
	}
	// Saldo × Posição: B-OK e B-PRE (pré-condição falsa, saldo 0) são ok; B-DIV divergente; B-SEMPOS sem posição.
	if c.TotValorOk != 2 || c.TotDivergentes != 1 || c.TotSemPosicao != 1 {
		t.Fatalf("saldo×posição: ok=%d div=%d sem_pos=%d", c.TotValorOk, c.TotDivergentes, c.TotSemPosicao)
	}
	st := map[string]string{}
	for _, l := range c.Divergencias {
		st[l.Boleto] = l.Status
	}
	if st["B-DIV"] != "divergente" || st["B-SEMPOS"] != "sem_posicao" {
		t.Errorf("status saldo inesperados: %+v", st)
	}
	if _, found := st["B-PRE"]; found {
		t.Errorf("B-PRE não deveria ser divergência (pré-condição falsa): %+v", st)
	}
	// Na posição, sem contábil.
	if c.TotOrfaos != 1 || len(c.Orfaos) != 1 || c.Orfaos[0].Boleto != "B-ORFAO" {
		t.Errorf("órfãos inesperados: tot=%d %+v", c.TotOrfaos, c.Orfaos)
	}
}
