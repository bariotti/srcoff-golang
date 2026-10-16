// Command massa_testes gera uma MASSA DE TESTES determinística e EXECUTA o motor contábil
// real sobre ela, produzindo um relatório boleto -> cenário -> resultado. A massa é gravada
// num diretório de backend "file" (default ./data-massa), para validação posterior na própria
// aplicação (STORAGE_BACKEND=file FILE_STORAGE_DIR=./data-massa).
//
// Uso:
//
//	go run ./cmd/massa_testes            # grava em ./data-massa
//	MASSA_DIR=/tmp/m go run ./cmd/massa_testes
//
// Spec: cobre os cenários de docs/cenarios-teste.md que envolvem boleto + lançamento contábil.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"time"

	"srcoff/internal/evaluator"
	"srcoff/internal/model"
	filerepo "srcoff/internal/repository/file"
	"srcoff/internal/service"
)

const (
	produto = "NDF"
	moeda   = "moeda_principal_remanescente"
	cboleto = "codigo_identificador_boleto"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// datas (dias úteis consecutivos de 2026)
var (
	D1 = d("2026-10-05") // seg
	D2 = d("2026-10-06") // ter
	D3 = d("2026-10-07") // qua
	D4 = d("2026-10-08") // qui
	D5 = d("2026-10-09") // sex
)

type condSpec struct {
	cond, deb, cred, campoValor string
}
type regraSpec struct {
	descricao, dominio, tipo string
	exigeD1                  bool
	conds                    []condSpec
}

// Regras da massa — um domínio por tipo, para isolar o comportamento de cada boleto.
var regras = []regraSpec{
	{"Reverte", "Reverte", model.TipoReverte, false, []condSpec{{"valor_mtm > 0", "1001", "2001", "valor_mtm"}}},
	{"Nao Reverte", "NaoReverte", model.TipoNaoReverte, false, []condSpec{{"valor_mtm > 0", "1003", "2003", "valor_mtm"}}},
	{"Incremental", "Incremental", model.TipoIncremental, false, []condSpec{{"valor_mtm > 0", "1005", "2005", "valor_mtm"}}},
	{"Incremental Reverte", "IncReverte", model.TipoIncrementalReverte, false, []condSpec{{"valor_mtm > 0", "1007", "2007", "valor_mtm"}}},
	{"Inc Reverte Some", "IncRevSome", model.TipoIncrementalReverte, false, []condSpec{{"valor_mtm > 0", "1017", "2017", "valor_mtm"}}},
	{"Multi Conta", "MultiConta", model.TipoIncrementalReverte, false, []condSpec{
		{"valor_a > 0", "1009", "2009", "valor_a"},
		{"valor_b > 0", "3009", "4009", "valor_b"},
	}},
	{"D-1 OK", "D1Ok", model.TipoReverte, true, []condSpec{{"valor_mtm > 0", "1011", "2011", "valor_mtm"}}},
	{"D-1 Bloqueio", "D1Block", model.TipoReverte, true, []condSpec{{"valor_mtm > 0", "1013", "2013", "valor_mtm"}}},
}

// posição: um registro (map de campos) para um boleto numa data.
func pos(dom, boleto string, campos map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{
		"produto": produto,
		"dominio": dom,
		cboleto:   boleto,
		moeda:     "USD",
	}
	for k, v := range campos {
		m[k] = v
	}
	return m
}

func mtm(v float64) map[string]interface{} { return map[string]interface{}{"valor_mtm": v} }

// posições por data (o que existe na POSIÇÃO em cada dia).
func posicoesPorData() map[time.Time][]map[string]interface{} {
	m := map[time.Time][]map[string]interface{}{}
	add := func(dt time.Time, r map[string]interface{}) { m[dt] = append(m[dt], r) }

	// Reverte: 70001 (valores distintos → lançamento+estorno visíveis); 70002 (mesmo valor → par cancela, oculto)
	add(D1, pos("Reverte", "70001", mtm(1000)))
	add(D2, pos("Reverte", "70001", mtm(1500)))
	add(D1, pos("Reverte", "70002", mtm(800)))
	add(D2, pos("Reverte", "70002", mtm(800)))

	// Nao Reverte: 70101 posta valor cheio todo dia, sem estorno
	add(D1, pos("NaoReverte", "70101", mtm(1000)))
	add(D2, pos("NaoReverte", "70101", mtm(1200)))

	// Incremental: 70201 série 1000->1300->900 ; 70202 some em D2 (mantém saldo, sem baixa)
	add(D1, pos("Incremental", "70201", mtm(1000)))
	add(D2, pos("Incremental", "70201", mtm(1300)))
	add(D3, pos("Incremental", "70201", mtm(900)))
	add(D1, pos("Incremental", "70202", mtm(1000)))
	// 70202 ausente de D2 em diante

	// Incremental Reverte:
	//  70301 série 1000->1300->900, some em D4 (baixa = 1700), reaparece em D5 (500, série nova)
	//  70302 filler presente todos os dias (série limpa, nunca some)
	//  70303 presente todos os dias, condição falsa a partir de D2 (não baixa — gatilho é a posição)
	add(D1, pos("IncReverte", "70301", mtm(1000)))
	add(D2, pos("IncReverte", "70301", mtm(1300)))
	add(D3, pos("IncReverte", "70301", mtm(900)))
	// 70301 ausente em D4
	add(D5, pos("IncReverte", "70301", mtm(500)))
	add(D1, pos("IncReverte", "70302", mtm(500)))
	add(D2, pos("IncReverte", "70302", mtm(600)))
	add(D3, pos("IncReverte", "70302", mtm(700)))
	add(D4, pos("IncReverte", "70302", mtm(800)))
	add(D5, pos("IncReverte", "70302", mtm(900)))
	add(D1, pos("IncReverte", "70303", mtm(1000)))
	add(D2, pos("IncReverte", "70303", mtm(-5)))
	add(D3, pos("IncReverte", "70303", mtm(-5)))
	add(D4, pos("IncReverte", "70303", mtm(-5)))
	add(D5, pos("IncReverte", "70303", mtm(-5)))

	// Inc Reverte Some: 70401 presente D1/D2, combo some inteiro em D3 (baixa = 1300)
	add(D1, pos("IncRevSome", "70401", mtm(1000)))
	add(D2, pos("IncRevSome", "70401", mtm(1300)))
	// combo IncRevSome ausente de D3 em diante

	// Multi Conta: 70501 duas contas (valor_a/valor_b), some em D3 (baixa dos 2 pares)
	add(D1, pos("MultiConta", "70501", map[string]interface{}{"valor_a": 1000.0, "valor_b": 500.0}))
	add(D2, pos("MultiConta", "70501", map[string]interface{}{"valor_a": 1200.0, "valor_b": 700.0}))
	// combo MultiConta ausente de D3 em diante

	// D-1 OK: 70601 presente D1/D2/D3 (nunca bloqueia; estorna D-1 útil)
	add(D1, pos("D1Ok", "70601", mtm(1000)))
	add(D2, pos("D1Ok", "70601", mtm(1200)))
	add(D3, pos("D1Ok", "70601", mtm(1000)))

	// D-1 Bloqueio: 70611 presente D1, ausente D2, presente D3 → bloqueado em D3
	add(D1, pos("D1Block", "70611", mtm(1000)))
	// 70611 ausente em D2
	add(D3, pos("D1Block", "70611", mtm(1000)))

	return m
}

func main() {
	log.SetOutput(io.Discard) // silencia os logs do motor; o relatório é impresso abaixo

	dir := os.Getenv("MASSA_DIR")
	if dir == "" {
		dir = "./data-massa"
	}
	ctx := context.Background()

	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, "erro ao limpar dir:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "erro ao criar dir:", err)
		os.Exit(1)
	}

	posRepo := filerepo.NewPosicaoCarteiraRepo(dir)
	regraRepo := filerepo.NewRegraContabilRepo(dir)
	movRepo := filerepo.NewMovimentoContabilRepo(dir)
	incRepo := filerepo.NewInconsistenciaRepo(dir)
	execRepo := filerepo.NewExecucaoRepo(dir)
	padraoRepo := filerepo.NewPadraoArquivoRepo(dir)
	eval := evaluator.New()
	svc := service.NewMovimentoContabilService(posRepo, regraRepo, movRepo, eval).
		ComInconsistenciaRepo(incRepo).ComExecucaoRepo(execRepo).ComPadraoRepo(padraoRepo)

	// 1) Padrões (define obrigatoriedade de D-1 por domínio) e 2) Regras + Condições
	for _, rs := range regras {
		od1 := rs.exigeD1
		if _, err := padraoRepo.Criar(ctx, model.PadraoArquivo{
			Padrao: produto + "_" + rs.dominio, Produto: produto, Dominio: rs.dominio, ObrigatorioMovD1: &od1,
		}); err != nil {
			panic(err)
		}
		id, err := regraRepo.CriarRegra(ctx, model.RegraContabil{
			Descricao: rs.descricao, CodigoProdutoCorporativo: produto, Dominio: rs.dominio,
			CampoProduto: "produto", TipoLancamento: rs.tipo,
		})
		if err != nil {
			panic(err)
		}
		for _, c := range rs.conds {
			if _, err := regraRepo.CriarCondicao(ctx, model.CondicaoRegra{
				IDRegra: id, Condicao: c.cond, ContaDebito: c.deb, ContaCredito: c.cred,
				CampoValor: c.campoValor, CampoMoeda: moeda, CampoBoleto: cboleto, Ativo: true,
			}); err != nil {
				panic(err)
			}
		}
	}

	// 3) Posições por data
	for dt, regs := range posicoesPorData() {
		if err := posRepo.ImportarLote(ctx, dt, 1, regs); err != nil {
			panic(err)
		}
	}

	// 4) Executa o contábil em ordem cronológica (escopo total)
	datas := []time.Time{D1, D2, D3, D4, D5}
	for _, dt := range datas {
		if _, err := svc.GerarMovimento(ctx, dt); err != nil {
			fmt.Printf("[gerar %s] %v\n", dt.Format("2006-01-02"), err)
		}
	}
	// Reprocesso de D2/Incremental para demonstrar versionamento (CT-K01/K02)
	if _, err := svc.GerarMovimentoEscopo(ctx, D2, produto, "Incremental"); err != nil {
		fmt.Printf("[reprocesso D2 Incremental] %v\n", err)
	}

	// 5) Relatório boleto -> lançamentos/estornos vigentes (com cancelados, para enxergar estornos)
	pag, err := movRepo.ConsultarPaginadoFiltrado(ctx, d("2000-01-01"), d("2999-12-31"), "", 0, "vigente", 1, 999999)
	if err != nil {
		panic(err)
	}
	type linha struct {
		data, tipo, contas string
		valor              float64
		versao             int
	}
	porBoleto := map[string][]linha{}
	for _, l := range pag.Lancamentos {
		tipo := "LANC"
		if l.IndicadorReversao {
			tipo = "ESTORNO"
		}
		porBoleto[l.CodigoIdentificadorBoleto] = append(porBoleto[l.CodigoIdentificadorBoleto], linha{
			data: l.DataLoteContabil.Format("2006-01-02"), tipo: tipo,
			contas: l.ContaDebito + "/" + l.ContaCredito, valor: l.ValorLancamentoContabil, versao: l.CodigoVersaoConteudo,
		})
	}

	boletos := make([]string, 0, len(porBoleto))
	for b := range porBoleto {
		boletos = append(boletos, b)
	}
	sort.Strings(boletos)

	fmt.Printf("=== MASSA DE TESTES — backend file em %q ===\n", dir)
	fmt.Printf("Para validar na aplicação: STORAGE_BACKEND=file FILE_STORAGE_DIR=%s (api + frontend)\n\n", dir)
	for _, b := range boletos {
		ls := porBoleto[b]
		sort.Slice(ls, func(i, j int) bool {
			if ls[i].data != ls[j].data {
				return ls[i].data < ls[j].data
			}
			return ls[i].tipo < ls[j].tipo
		})
		fmt.Printf("BOLETO %s — %s\n", b, cenarioDoBoleto[b])
		for _, l := range ls {
			fmt.Printf("   %s v%d %-7s %10.2f  (%s)\n", l.data, l.versao, l.tipo, l.valor, l.contas)
		}
	}

	// Inconsistências e execuções por data (resumo)
	fmt.Printf("\n=== EXECUÇÕES (Processados/Estornos/Bruto) ===\n")
	for _, dt := range datas {
		execs, _ := execRepo.ListarPorData(ctx, dt)
		for _, e := range execs {
			fmt.Printf("   %s %s/%s: lanc=%d estornos=%d bruto=%d\n", dt.Format("2006-01-02"), e.Produto, e.Dominio, e.QtdLancamentos, e.QtdEstornos, e.QtdMovimento)
		}
	}
}

// cenarioDoBoleto mapeia cada boleto da massa aos cenários de docs/cenarios-teste.md.
var cenarioDoBoleto = map[string]string{
	"70001": "Reverte: lançamento+estorno visíveis (CT-H01, J01, J02)",
	"70002": "Reverte mesmo valor: par de saldo zero OCULTO na consulta (CT-L07)",
	"70101": "Não Reverte: posta cheio, sem estorno (CT-H02)",
	"70201": "Incremental: 1000→300→400; reprocesso D2 vira v2 (CT-H03/04/05/08, K01/K02)",
	"70202": "Incremental: some em D2, mantém saldo sem baixa (CT-H09)",
	"70301": "Inc/Reverte: série, baixa 1700 em D4, reaparece D5 (CT-H10/11/12/13/16)",
	"70302": "Inc/Reverte: filler, série limpa sem desaparecimento (CT-H10)",
	"70303": "Inc/Reverte: presente mas condição falsa → NÃO baixa (CT-H14)",
	"70401": "Inc/Reverte: combo some inteiro em D3 → baixa 1300 + versão/execução (CT-H15/O04)",
	"70501": "Inc/Reverte multi-conta: baixa dos 2 pares de contas (CT-H18)",
	"70601": "D-1 OK (reverte): processa todo dia, estorna D-1 útil (CT-I02/I03/J02)",
	"70611": "D-1 Bloqueio (reverte): bloqueado em D3 por falta de D-1 (CT-I01)",
}
