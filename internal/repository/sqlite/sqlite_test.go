package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"srcoff/internal/model"
)

func abrirTeste(t *testing.T) *sqliteDBs {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "teste.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &sqliteDBs{
		regra:  NewRegraContabilRepo(db),
		pos:    NewPosicaoCarteiraRepo(db),
		mov:    NewMovimentoContabilRepo(db),
		exec:   NewExecucaoRepo(db),
		inc:    NewInconsistenciaRepo(db),
		padrao: NewPadraoArquivoRepo(db),
		param:  NewParametrizacaoRepo(db),
		conf:   NewConfiguracaoRepo(db),
		notif:  NewNotificacaoRepo(db),
	}
}

type sqliteDBs struct {
	regra  *RegraContabilRepo
	pos    *PosicaoCarteiraRepo
	mov    *MovimentoContabilRepo
	exec   *ExecucaoRepo
	inc    *InconsistenciaRepo
	padrao *PadraoArquivoRepo
	param  *ParametrizacaoRepo
	conf   *ConfiguracaoRepo
	notif  *NotificacaoRepo
}

func TestRegraCRUD(t *testing.T) {
	ctx := context.Background()
	s := abrirTeste(t)
	id, err := s.regra.CriarRegra(ctx, model.RegraContabil{Descricao: "R1", CodigoProdutoCorporativo: "NDF", Dominio: "Posição", TipoLancamento: model.TipoReverte})
	if err != nil || id == 0 {
		t.Fatalf("CriarRegra: id=%d err=%v", id, err)
	}
	if _, err := s.regra.CriarCondicao(ctx, model.CondicaoRegra{IDRegra: id, Condicao: "valor_mtm > 0", ContaDebito: "1001", ContaCredito: "2001", CampoValor: "valor_principal", CampoMoeda: "moeda", CampoBoleto: "codigo_identificador_boleto"}); err != nil {
		t.Fatalf("CriarCondicao: %v", err)
	}
	regras, err := s.regra.ListarRegrasAtivas(ctx)
	if err != nil || len(regras) != 1 {
		t.Fatalf("ListarRegrasAtivas: n=%d err=%v", len(regras), err)
	}
	if !regras[0].EhReverte() || len(regras[0].Condicoes) != 1 || regras[0].Condicoes[0].CampoBoleto != "codigo_identificador_boleto" {
		t.Fatalf("regra inesperada: %+v", regras[0])
	}
	// Editar condição não desativa.
	c := regras[0].Condicoes[0]
	c.ContaDebito = "9999"
	if err := s.regra.EditarCondicao(ctx, c); err != nil {
		t.Fatalf("EditarCondicao: %v", err)
	}
	// Excluir regra (lógico) → não listada.
	if err := s.regra.ExcluirRegra(ctx, id); err != nil {
		t.Fatalf("ExcluirRegra: %v", err)
	}
	if regras, _ := s.regra.ListarRegrasAtivas(ctx); len(regras) != 0 {
		t.Fatalf("regra excluída não deveria aparecer, n=%d", len(regras))
	}
}

func TestPosicaoImportEVersao(t *testing.T) {
	ctx := context.Background()
	s := abrirTeste(t)
	d := time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC)
	// versão 1 (um registro), versão 2 (um registro) para NDF/Posição.
	if err := s.pos.ImportarLote(ctx, d, 1, []map[string]interface{}{{"produto": "NDF", "dominio": "Posição", "codigo_identificador_boleto": "0001", "valor_mtm": 10.0}}); err != nil {
		t.Fatalf("ImportarLote v1: %v", err)
	}
	if err := s.pos.ImportarLote(ctx, d, 2, []map[string]interface{}{{"produto": "NDF", "dominio": "Posição", "codigo_identificador_boleto": "0002", "valor_mtm": 20.0}}); err != nil {
		t.Fatalf("ImportarLote v2: %v", err)
	}
	todas, _ := s.pos.ListarPorData(ctx, d)
	if len(todas) != 2 {
		t.Fatalf("ListarPorData esperado 2, obteve %d", len(todas))
	}
	vig, _ := s.pos.BuscarPorDataEVersaoMaxima(ctx, d)
	if len(vig) != 1 || vig[0].CodigoVersaoConteudo != 2 {
		t.Fatalf("versão máxima esperada 1 registro v2, obteve %+v", vig)
	}
	// Campos (string) preservados e data round-trip.
	if vig[0].Campos["codigo_identificador_boleto"] != "0002" {
		t.Fatalf("boleto (texto) não preservado: %v", vig[0].Campos["codigo_identificador_boleto"])
	}
	if !vig[0].DataPosicaoCarteira.Equal(d) {
		t.Fatalf("data round-trip falhou: %v != %v", vig[0].DataPosicaoCarteira, d)
	}
}

func lanc(d time.Time, boleto string, deb, cred string, valor float64, reversao bool) model.LancamentoContabil {
	return model.LancamentoContabil{
		DataLoteContabil: d, CodigoVersaoConteudo: 1, CodigoIdentificadorBoleto: boleto,
		ValorLancamentoContabil: valor, MoedaLancamentoContabil: "USD", ContaDebito: deb, ContaCredito: cred,
		Produto: "NDF", Dominio: "Posição", IndicadorReversao: reversao, DescricaoRegraContabil: "R", DescricaoCondicaoContabil: "C", IDRegraContabil: 1,
	}
}

func TestMovimentoBulkEConsulta(t *testing.T) {
	ctx := context.Background()
	s := abrirTeste(t)
	d := time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC)

	prox, _ := s.mov.ObterProximaVersao(ctx, d)
	if prox != 1 {
		t.Fatalf("próxima versão inicial esperada 1, obteve %d", prox)
	}
	// Boleto A: movimento + estorno que se anulam (saldo zero). Boleto B: só movimento.
	err := s.mov.BulkInsert(ctx, []model.LancamentoContabil{
		lanc(d, "A", "1001", "2001", 100, false),
		lanc(d, "A", "2001", "1001", 100, true), // estorno com contas invertidas
		lanc(d, "B", "1001", "2001", 50, false),
	})
	if err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}
	// ConsultarPaginado (todos) → 3.
	pg, _ := s.mov.ConsultarPaginado(ctx, d, 1, 100)
	if pg.Total != 3 {
		t.Fatalf("total esperado 3, obteve %d", pg.Total)
	}
	// Data round-trip no scan.
	if !pg.Lancamentos[0].DataLoteContabil.Equal(d) {
		t.Fatalf("data do lançamento não bate: %v", pg.Lancamentos[0].DataLoteContabil)
	}
	// Sem cancelados: par A (saldo zero) some, resta só B.
	sc, _ := s.mov.ConsultarPaginadoFiltradoSemCancelados(ctx, d, d, "", 0, "vigente", 1, 100)
	if sc.Total != 1 || sc.Lancamentos[0].CodigoIdentificadorBoleto != "B" {
		t.Fatalf("sem cancelados esperado apenas B, obteve total=%d %+v", sc.Total, sc.Lancamentos)
	}
	// Buscar movimento (não-estorno) vigente.
	mov, _ := s.mov.BuscarPorDataEIndicador(ctx, d, false)
	if len(mov) != 2 { // A e B (não-estorno)
		t.Fatalf("movimento não-estorno esperado 2, obteve %d", len(mov))
	}
	// Exclusão por data.
	if err := s.mov.ExcluirPorDataEVersao(ctx, d, 0); err != nil {
		t.Fatalf("ExcluirPorDataEVersao: %v", err)
	}
	if pg, _ := s.mov.ConsultarPaginado(ctx, d, 1, 100); pg.Total != 0 {
		t.Fatalf("após exclusão total deveria ser 0, obteve %d", pg.Total)
	}
}

func TestExecucaoDatas(t *testing.T) {
	ctx := context.Background()
	s := abrirTeste(t)
	d1 := time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC)
	// d1: execução sem movimento (qtd_movimento 0). d2: com movimento.
	_ = s.exec.RegistrarExecucao(ctx, model.MovimentoExecucao{DataLote: d1, Produto: "NDF", Dominio: "Posição", QtdLancamentos: 0, QtdMovimento: 0})
	_ = s.exec.RegistrarExecucao(ctx, model.MovimentoExecucao{DataLote: d2, Produto: "NDF", Dominio: "Posição", QtdLancamentos: 2, QtdMovimento: 2})
	todas, _ := s.exec.DatasExecutadas(ctx, "NDF", "Posição")
	if len(todas) != 2 {
		t.Fatalf("DatasExecutadas esperado 2, obteve %d", len(todas))
	}
	comMov, _ := s.exec.DatasComMovimento(ctx, "NDF", "Posição")
	if len(comMov) != 1 || !comMov[0].Equal(d2) {
		t.Fatalf("DatasComMovimento esperado apenas d2, obteve %v", comMov)
	}
	// Reprocessar d2 sobrescreve (não duplica).
	_ = s.exec.RegistrarExecucao(ctx, model.MovimentoExecucao{DataLote: d2, Produto: "NDF", Dominio: "Posição", QtdLancamentos: 3, QtdMovimento: 3})
	lst, _ := s.exec.ListarPorData(ctx, d2)
	if len(lst) != 1 || lst[0].QtdMovimento != 3 {
		t.Fatalf("execução de d2 deveria ser única e atualizada, obteve %+v", lst)
	}
}

func TestInconsistenciaSubstitui(t *testing.T) {
	ctx := context.Background()
	s := abrirTeste(t)
	d := time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC)
	combos := []model.ProdutoDominio{{Produto: "NDF", Dominio: "Posição"}}
	_ = s.inc.SubstituirPorEscopo(ctx, d, combos, []model.InconsistenciaProcessamento{
		{CodigoIdentificadorBoleto: "A", Produto: "NDF", Dominio: "Posição", Tipo: "CAMPO_VALOR", Expressao: "x", CamposFaltantes: "x", Detalhe: "d"},
		{CodigoIdentificadorBoleto: "B", Produto: "NDF", Dominio: "Posição", Tipo: "CONDICAO", Expressao: "y", CamposFaltantes: "y", Detalhe: "d"},
	})
	if itens, _ := s.inc.ListarPorData(ctx, d); len(itens) != 2 {
		t.Fatalf("esperado 2 inconsistências, obteve %d", len(itens))
	}
	// Reprocessar o mesmo combo com 1 item substitui (não acumula).
	_ = s.inc.SubstituirPorEscopo(ctx, d, combos, []model.InconsistenciaProcessamento{
		{CodigoIdentificadorBoleto: "C", Produto: "NDF", Dominio: "Posição", Tipo: "PRE_CONDICAO"},
	})
	itens, _ := s.inc.ListarPorData(ctx, d)
	if len(itens) != 1 || itens[0].CodigoIdentificadorBoleto != "C" {
		t.Fatalf("substituição falhou, obteve %+v", itens)
	}
}

func TestPadraoParamConfigNotif(t *testing.T) {
	ctx := context.Background()
	s := abrirTeste(t)

	// Padrão com coluna_boleto e obrigatorio_mov_d1=false.
	naoObrig := false
	id, err := s.padrao.Criar(ctx, model.PadraoArquivo{Padrao: "ndf_*.csv", Produto: "NDF", Dominio: "Posição", ColunaBoleto: "codigo_identificador_boleto", ColunaData: "data", FormatoData: "DD/MM/AAAA", ObrigatorioMovD1: &naoObrig})
	if err != nil || id == 0 {
		t.Fatalf("padrao.Criar: %v", err)
	}
	ps, _ := s.padrao.Listar(ctx)
	if len(ps) != 1 || ps[0].ColunaBoleto != "codigo_identificador_boleto" || ps[0].ExigeMovimentoD1() {
		t.Fatalf("padrão inesperado: %+v", ps)
	}
	_ = s.padrao.Excluir(ctx, id)
	if ps, _ := s.padrao.Listar(ctx); len(ps) != 0 {
		t.Fatalf("padrão deveria ter sido excluído")
	}

	// Defaults semeados no Open (paridade com os outros backends).
	if prod, _ := s.param.ListarOpcoes(ctx, "produto"); len(prod) != 3 {
		t.Fatalf("defaults de produto esperados (NDF,SWAP,FXO), obteve %v", prod)
	}
	// Parametrização: duplicata ignorada (categoria não-semeada).
	_ = s.param.AdicionarOpcao(ctx, "teste", "X")
	_ = s.param.AdicionarOpcao(ctx, "teste", "X")
	if op, _ := s.param.ListarOpcoes(ctx, "teste"); len(op) != 1 {
		t.Fatalf("duplicata deveria ser ignorada, obteve %v", op)
	}
	_ = s.param.RemoverOpcao(ctx, "teste", "X")
	if op, _ := s.param.ListarOpcoes(ctx, "teste"); len(op) != 0 {
		t.Fatalf("remoção falhou, obteve %v", op)
	}

	// Configuração: upsert.
	_ = s.conf.Definir(ctx, "pasta", "/a")
	_ = s.conf.Definir(ctx, "pasta", "/b")
	if v, _ := s.conf.Obter(ctx, "pasta"); v != "/b" {
		t.Fatalf("upsert falhou, obteve %q", v)
	}
	if v, _ := s.conf.Obter(ctx, "inexistente"); v != "" {
		t.Fatalf("chave inexistente deveria retornar vazio, obteve %q", v)
	}

	// Notificação.
	if _, err := s.notif.Criar(ctx, model.Notificacao{Tipo: "SUCESSO", Mensagem: "ok", DataLote: "2026-01-08"}); err != nil {
		t.Fatalf("notif.Criar: %v", err)
	}
	if n, _ := s.notif.ContarNaoLidas(ctx); n != 1 {
		t.Fatalf("não lidas esperado 1, obteve %d", n)
	}
	if lst, _ := s.notif.Listar(ctx, 10); len(lst) != 1 || lst[0].Mensagem != "ok" {
		t.Fatalf("listar notificações inesperado: %+v", lst)
	}
	_ = s.notif.MarcarTodasLidas(ctx)
	if n, _ := s.notif.ContarNaoLidas(ctx); n != 0 {
		t.Fatalf("após marcar lidas, não lidas deveria ser 0, obteve %d", n)
	}
}
