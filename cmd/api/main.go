package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"srcoff/internal/ai"
	"srcoff/internal/db"
	"srcoff/internal/evaluator"
	"srcoff/internal/handler"
	"srcoff/internal/model"
	"srcoff/internal/repository"
	filerepo "srcoff/internal/repository/file"
	sqliterepo "srcoff/internal/repository/sqlite"
	"srcoff/internal/service"
	"srcoff/internal/webui"
)

func main() {
	// 1. Selecionar backend de armazenamento via STORAGE_BACKEND (sqlserver | file | sqlite)
	backend := os.Getenv("STORAGE_BACKEND")
	if backend == "" {
		backend = "sqlite"
	}

	var (
		posicaoRepo        repository.PosicaoCarteiraRepository
		regraRepo          repository.RegraContabilRepository
		movimentoRepo      repository.MovimentoContabilRepository
		parametrizacaoRepo repository.ParametrizacaoRepository
		inconsistenciaRepo repository.InconsistenciaRepository
		padraoArquivoRepo  repository.PadraoArquivoRepository
		configuracaoRepo   repository.ConfiguracaoRepository
		execucaoRepo       repository.ExecucaoRepository
		notificacaoRepo    repository.NotificacaoRepository
		saldoRepo          repository.SaldoContaRepository
		veiculoDeParaRepo  repository.VeiculoDeParaRepository
		cotacaoRepo        repository.CotacaoRepository
		jobRepo            repository.JobRepository // fila de jobs (por ora apenas SQLite)
	)

	var rawSQLDB *sql.DB
	// Controle de fechamento de competência (RF-132): apenas SQLite por ora (tipo concreto;
	// nil nos demais backends → feature indisponível).
	var fechamentoRepo *sqliterepo.ControleFechamentoRepo
	// Histórico de versões do cadastro de regras (RF-133): também sqlite-first.
	var regraVersaoRepo *sqliterepo.RegraVersaoRepo
	// Trilha de auditoria global (RF-136): persistência sqlite-first (a linha de log JSON é sempre emitida).
	var auditoriaRepo *sqliterepo.AuditoriaRepo

	switch backend {
	case "file":
		dir := os.Getenv("FILE_STORAGE_DIR")
		if dir == "" {
			dir = "./data"
		}
		log.Printf("Backend: arquivo (dir=%s)", dir)
		posicaoRepo = filerepo.NewPosicaoCarteiraRepo(dir)
		regraRepo = filerepo.NewRegraContabilRepo(dir)
		movimentoRepo = filerepo.NewMovimentoContabilRepo(dir)
		parametrizacaoRepo = filerepo.NewParametrizacaoRepo(dir)
		inconsistenciaRepo = filerepo.NewInconsistenciaRepo(dir)
		padraoArquivoRepo = filerepo.NewPadraoArquivoRepo(dir)
		configuracaoRepo = filerepo.NewConfiguracaoRepo(dir)
		execucaoRepo = filerepo.NewExecucaoRepo(dir)
		notificacaoRepo = filerepo.NewNotificacaoRepo(dir)
		saldoRepo = filerepo.NewSaldoContaRepo(dir)
		veiculoDeParaRepo = filerepo.NewVeiculoDeParaRepo(dir)
		cotacaoRepo = filerepo.NewCotacaoRepo(dir)

	case "sqlite":
		path := os.Getenv("SQLITE_PATH")
		if path == "" {
			path = "./srcoff.db"
		}
		sdb, err := sqliterepo.Open(path)
		if err != nil {
			log.Fatalf("erro ao abrir SQLite (%s): %v", path, err)
		}
		defer sdb.Close()
		log.Printf("Backend: SQLite (arquivo=%s)", path)
		posicaoRepo = sqliterepo.NewPosicaoCarteiraRepo(sdb)
		regraRepo = sqliterepo.NewRegraContabilRepo(sdb)
		movimentoRepo = sqliterepo.NewMovimentoContabilRepo(sdb)
		parametrizacaoRepo = sqliterepo.NewParametrizacaoRepo(sdb)
		inconsistenciaRepo = sqliterepo.NewInconsistenciaRepo(sdb)
		padraoArquivoRepo = sqliterepo.NewPadraoArquivoRepo(sdb)
		configuracaoRepo = sqliterepo.NewConfiguracaoRepo(sdb)
		execucaoRepo = sqliterepo.NewExecucaoRepo(sdb)
		notificacaoRepo = sqliterepo.NewNotificacaoRepo(sdb)
		saldoRepo = sqliterepo.NewSaldoContaRepo(sdb)
		veiculoDeParaRepo = sqliterepo.NewVeiculoDeParaRepo(sdb)
		cotacaoRepo = sqliterepo.NewCotacaoRepo(sdb)
		jobRepo = sqliterepo.NewJobRepo(sdb)
		fechamentoRepo = sqliterepo.NewControleFechamentoRepo(sdb)
		regraVersaoRepo = sqliterepo.NewRegraVersaoRepo(sdb)
		auditoriaRepo = sqliterepo.NewAuditoriaRepo(sdb)

	default: // sqlserver
		rawSQLDB = db.Connect()
		defer rawSQLDB.Close()
		log.Printf("Backend: SQL Server")
		// Auto-corrige o schema (colunas/tabelas adicionadas após a base), evitando erros
		// "Invalid column" quando uma migration não foi aplicada manualmente.
		if err := repository.GarantirSchema(context.Background(), rawSQLDB); err != nil {
			log.Printf("[schema] aviso ao garantir schema SQL Server: %v", err)
		}
		posicaoRepo = repository.NewPosicaoCarteiraRepo(rawSQLDB)
		regraRepo = repository.NewRegraContabilRepo(rawSQLDB)
		movimentoRepo = repository.NewMovimentoContabilRepo(rawSQLDB)
		parametrizacaoRepo = repository.NewParametrizacaoRepo(rawSQLDB)
		inconsistenciaRepo = repository.NewInconsistenciaRepo(rawSQLDB)
		padraoArquivoRepo = repository.NewPadraoArquivoRepo(rawSQLDB)
		configuracaoRepo = repository.NewConfiguracaoRepo(rawSQLDB)
		execucaoRepo = repository.NewExecucaoRepo(rawSQLDB)
		notificacaoRepo = repository.NewNotificacaoRepo(rawSQLDB)
		saldoRepo = repository.NewSaldoContaRepo(rawSQLDB)
		veiculoDeParaRepo = repository.NewVeiculoDeParaRepo(rawSQLDB)
		cotacaoRepo = repository.NewCotacaoRepo(rawSQLDB)
	}

	// 2. Instanciar avaliador de expressões
	eval := evaluator.New()

	// Trilha de auditoria global (RF-136). Sempre existe (emite a linha de log JSON); a
	// persistência/consulta só liga quando há repositório (sqlite). Evita o "typed nil":
	// só passa o repo quando não for nil.
	auditoriaSvc := service.NewAuditoriaService(nil)
	if auditoriaRepo != nil {
		auditoriaSvc = service.NewAuditoriaService(auditoriaRepo)
	}

	// 3. Instanciar serviços
	movimentoSvc := service.NewMovimentoContabilService(posicaoRepo, regraRepo, movimentoRepo, eval).
		ComInconsistenciaRepo(inconsistenciaRepo).
		ComExecucaoRepo(execucaoRepo).
		ComPadraoRepo(padraoArquivoRepo).
		ComAuditoria(auditoriaSvc)

	// Saldo em conta (RN-170) ATIVO por padrão. Para desligar, defina SALDO_CONTA_ENABLED=false
	// (ou 0). Quando desligado, o serviço não mantém nem lê a projeção de saldo: a baixa do
	// incremental_reverte usa o fallback (varredura do movimento vigente, resultado idêntico).
	saldoFlag := strings.ToLower(strings.TrimSpace(os.Getenv("SALDO_CONTA_ENABLED")))
	saldoAtivo := saldoFlag != "false" && saldoFlag != "0" && saldoFlag != "nao" && saldoFlag != "não"
	if saldoAtivo {
		movimentoSvc = movimentoSvc.ComSaldoRepo(saldoRepo)
		// Projeção de saldo (RN-170): se a tabela estiver vazia (base recém-criada/migrada),
		// reconstrói a partir do movimento vigente existente. Operação única.
		if vazio, err := movimentoSvc.SaldoVazio(context.Background()); err == nil && vazio {
			if err := movimentoSvc.ReconstruirSaldos(context.Background()); err != nil {
				log.Printf("[saldo] falha ao reconstruir projeção de saldo na inicialização: %v", err)
			}
		}
		log.Printf("[saldo] funcionalidade de saldo em conta HABILITADA")
	} else {
		log.Printf("[saldo] funcionalidade de saldo em conta DESABILITADA (SALDO_CONTA_ENABLED=%q)", saldoFlag)
	}
	inconsistenciaSvc := service.NewInconsistenciaService(inconsistenciaRepo)
	execucaoSvc := service.NewExecucaoService(execucaoRepo, padraoArquivoRepo).
		ComInconsistenciaRepo(inconsistenciaRepo)
	notificacaoSvc := service.NewNotificacaoService(notificacaoRepo)
	regraSvc := service.NewRegraContabilService(regraRepo).ComAuditoria(auditoriaSvc)
	// Histórico de versões do cadastro (RF-133): habilitado quando há repositório de versões
	// (atualmente só no SQLite). Sem ele, o cadastro funciona normalmente e a feature fica off.
	if regraVersaoRepo != nil {
		regraSvc = regraSvc.ComVersaoRepo(regraVersaoRepo)
	}
	posicaoSvc := service.NewPosicaoCarteiraService(posicaoRepo)
	parametrizacaoSvc := service.NewParametrizacaoService(parametrizacaoRepo)
	padraoArquivoSvc := service.NewPadraoArquivoService(padraoArquivoRepo)
	veiculoDeParaSvc := service.NewVeiculoDeParaService(veiculoDeParaRepo)
	cotacaoSvc := service.NewCotacaoService(cotacaoRepo)
	// O dashboard só lê a projeção de saldo quando ela está ativa (senão pode estar desatualizada).
	dashboardSaldoRepo := saldoRepo
	if !saldoAtivo {
		dashboardSaldoRepo = nil
	}
	dashboardSvc := service.NewDashboardService(execucaoRepo, inconsistenciaRepo, dashboardSaldoRepo, cotacaoRepo)
	// Conversão para a moeda base do veículo na geração (RF-036/RF-037).
	movimentoSvc = movimentoSvc.ComMoedaBaseRepo(veiculoDeParaSvc).ComCotacaoRepo(cotacaoSvc)
	// Trava de competência (RF-132): quando disponível (SQLite), a geração pula combinações
	// cuja competência está fechada e ainda não liberada.
	if fechamentoRepo != nil {
		movimentoSvc = movimentoSvc.ComFechamentoRepo(fechamentoRepo)
	}
	configuracaoSvc := service.NewConfiguracaoService(configuracaoRepo)

	// 4. Instanciar handlers
	movimentoHandler := handler.NewMovimentoContabilHandler(movimentoSvc)
	regraHandler := handler.NewRegraContabilHandler(regraSvc)
	regraHistoricoHandler := handler.NewRegraHistoricoHandler(regraSvc)
	catalogoRegrasHandler := handler.NewCatalogoRegrasHandler(regraSvc)
	auditoriaHandler := handler.NewAuditoriaHandler(auditoriaSvc)
	conciliacaoIAHandler := handler.NewConciliacaoIAHandler(movimentoSvc, posicaoSvc)
	posicaoHandler := handler.NewPosicaoCarteiraHandler(posicaoSvc, padraoArquivoSvc)
	parametrizacaoHandler := handler.NewParametrizacaoHandler(parametrizacaoSvc)
	padraoArquivoHandler := handler.NewPadraoArquivoHandler(padraoArquivoSvc)
	veiculoDeParaHandler := handler.NewVeiculoDeParaHandler(veiculoDeParaSvc)
	cotacaoHandler := handler.NewCotacaoHandler(cotacaoSvc)
	razoneteHandler := handler.NewRazoneteHandler(movimentoSvc)
	dashboardHandler := handler.NewDashboardHandler(dashboardSvc)
	configuracaoHandler := handler.NewConfiguracaoHandler(configuracaoSvc)
	inconsistenciaHandler := handler.NewInconsistenciaHandler(inconsistenciaSvc)
	execucaoHandler := handler.NewExecucaoHandler(execucaoSvc)
	notificacaoHandler := handler.NewNotificacaoHandler(notificacaoSvc)
	exportHandler := handler.NewExportHandler(movimentoSvc).ComVeiculoDePara(veiculoDeParaSvc)
	nlQueryHandler := handler.NewNLQueryHandler(rawSQLDB)
	// Controle de fechamento de competência (RF-132). nil quando o backend não suporta → 501.
	var fechamentoHandler *handler.FechamentoHandler
	var fechamentoSvc *service.FechamentoService // reusado pelo copiloto (nil = sem fechamento)
	if fechamentoRepo != nil {
		fechamentoSvc = service.NewFechamentoService(fechamentoRepo).ComAuditoria(auditoriaSvc)
		fechamentoHandler = handler.NewFechamentoHandler(fechamentoSvc)
	} else {
		fechamentoHandler = handler.NewFechamentoHandler(nil)
	}

	// Copiloto conversacional (RF-138): chat via API da Claude. A chave vem de ANTHROPIC_API_KEY
	// (ambiente). Habilita-se sozinho quando a chave está presente; sem chave, status indisponível.
	copilotSvc := service.NewCopilotService(ai.NewCliente(), regraSvc, movimentoSvc, posicaoSvc, fechamentoSvc)
	copilotHandler := handler.NewCopilotHandler(copilotSvc)

	// [LAB] Features experimentais: Diagnóstico de dupla contabilização + Linhagem de
	// lançamento. LIGADAS por padrão; desligar com FEATURES_LAB=0 (ou false/off/nao), sem
	// recompilar. Reusa o mesmo posicaoRepo/regraRepo da geração (funciona em qualquer backend).
	labFlag := strings.ToLower(strings.TrimSpace(os.Getenv("FEATURES_LAB")))
	labAtivo := labFlag != "0" && labFlag != "false" && labFlag != "off" && labFlag != "nao" && labFlag != "não"
	labHandler := handler.NewLabHandler(service.NewLabService(posicaoRepo, regraRepo, eval), labAtivo)
	if labAtivo {
		log.Printf("[lab] features experimentais (diagnóstico + linhagem) HABILITADAS")
	}

	// Watcher de pasta monitorada (varredura periódica em segundo plano).
	// O intervalo (em minutos) é parametrizável; sem parametrização, não varre.
	// A importação automática dispara o contábil automaticamente e gera notificações.
	pastaWatcher := handler.NewPastaWatcher(posicaoSvc, padraoArquivoSvc, configuracaoSvc, movimentoSvc, notificacaoSvc)
	pastaWatcher.Start(context.Background())

	// Importação automática de cotações (mesma mecânica, reusa o intervalo de varredura).
	cotacaoWatcher := handler.NewCotacaoWatcher(cotacaoSvc, configuracaoSvc)
	cotacaoWatcher.Start(context.Background())

	// Fila de jobs assíncronos (RF-121). Disponível quando há repositório de jobs
	// (atualmente só no backend SQLite). O worker roda em segundo plano.
	var jobHandler *handler.JobHandler
	if jobRepo != nil {
		jobService := service.NewJobService(jobRepo)
		jobService.Registrar("geracao_movimento", func(ctx context.Context, j *model.Job, logf func(string)) error {
			var p struct {
				Data    string `json:"data"`
				Produto string `json:"produto"`
				Dominio string `json:"dominio"`
			}
			if err := json.Unmarshal([]byte(j.Payload), &p); err != nil {
				return fmt.Errorf("payload inválido: %w", err)
			}
			data, err := time.Parse("2006-01-02", p.Data)
			if err != nil {
				return fmt.Errorf("data inválida: %w", err)
			}
			logf("iniciando geração de movimento para " + p.Data)
			bloqueios, err := movimentoSvc.GerarMovimentoEscopo(ctx, data, p.Produto, p.Dominio)
			if err != nil {
				return err
			}
			if len(bloqueios) > 0 {
				logf(fmt.Sprintf("%d combinação(ões) bloqueada(s) por inconsistência", len(bloqueios)))
			}
			return nil
		})
		// Expurgo/retenção de movimentos (RF-123): job recorrente que remove movimentos
		// contábeis (e as inconsistências/execuções correspondentes) além da retenção
		// configurada. Disponível quando os repositórios sabem expurgar por data (SQLite).
		posExp, ok0 := posicaoRepo.(service.ExpurgavelPorData)
		movExp, ok1 := movimentoRepo.(service.ExpurgavelPorData)
		incExp, ok2 := inconsistenciaRepo.(service.ExpurgavelPorData)
		exeExp, ok3 := execucaoRepo.(service.ExpurgavelPorData)
		if ok0 && ok1 && ok2 && ok3 {
			expurgoSvc := service.NewExpurgoService(configuracaoSvc, posExp, movExp, incExp, exeExp)
			jobService.Registrar("expurgo_movimento", func(ctx context.Context, j *model.Job, logf func(string)) error {
				return expurgoSvc.Executar(ctx, logf)
			})
			go agendarExpurgo(context.Background(), jobService, expurgoSvc)
			log.Printf("[expurgo] agendador de retenção HABILITADO")
		}

		jobService.Start(context.Background())
		// Geração agendada do contábil (RF-126): enfileira a geração do escopo configurado
		// para a última data com posição, no intervalo configurado.
		go agendarGeracao(context.Background(), jobService, configuracaoSvc, posicaoRepo, execucaoRepo)
		jobHandler = handler.NewJobHandler(jobService)
		log.Printf("[job] fila de jobs HABILITADA")
	} else {
		jobHandler = handler.NewJobHandler(nil)
		log.Printf("[job] fila de jobs indisponível neste backend")
	}

	// 5. Registrar rotas
	http.HandleFunc("/api/v1/movimento-contabil", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			movimentoHandler.ConsultarMovimento(w, r)
		case http.MethodPost:
			movimentoHandler.GerarMovimento(w, r)
		case http.MethodDelete:
			movimentoHandler.ExcluirMovimento(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/api/v1/estorno", movimentoHandler.GerarEstorno)

	http.HandleFunc("/api/v1/regras", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			regraHandler.ListarRegras(w, r)
		} else {
			regraHandler.CriarRegra(w, r)
		}
	})

	// Exportação/importação em lote de regras+condições (um único CSV).
	// Padrões mais específicos vencem o catch-all "/api/v1/regras/" no ServeMux.
	http.HandleFunc("/api/v1/regras/export", regraHandler.ExportarRegrasCSV)
	http.HandleFunc("/api/v1/regras/import", regraHandler.ImportarRegrasCSV)

	http.HandleFunc("/api/v1/regras/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/clonar") {
			regraHandler.ClonarRegra(w, r)
		} else if strings.HasSuffix(path, "/condicoes") {
			if r.Method == http.MethodGet {
				regraHandler.ListarCondicoes(w, r)
			} else {
				regraHandler.CriarCondicao(w, r)
			}
		} else if r.Method == http.MethodDelete {
			regraHandler.ExcluirRegra(w, r)
		} else {
			regraHandler.EditarRegra(w, r)
		}
	})

	http.HandleFunc("/api/v1/condicoes/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/clonar") {
			regraHandler.ClonarCondicao(w, r)
		} else if r.Method == http.MethodDelete {
			regraHandler.ExcluirCondicao(w, r)
		} else {
			regraHandler.EditarCondicao(w, r)
		}
	})
	// Catálogo de regras (RF-134): documento HTML imprimível das regras ativas.
	http.HandleFunc("/api/v1/catalogo-regras", catalogoRegrasHandler.Catalogo)
	// Trilha de auditoria global (RF-136): consulta e exportação (sqlite-first).
	http.HandleFunc("/api/v1/auditoria", auditoriaHandler.Listar)
	http.HandleFunc("/api/v1/auditoria/export", auditoriaHandler.Export)
	// Histórico de versões do cadastro de regras (RF-133). Prefixo distinto de "/api/v1/regras/".
	http.HandleFunc("/api/v1/regra-historico", regraHistoricoHandler.Raiz)
	http.HandleFunc("/api/v1/regra-historico/diff", regraHistoricoHandler.Diff)
	http.HandleFunc("/api/v1/regra-historico/restaurar", regraHistoricoHandler.Restaurar)
	// Fechamento de competência (RF-132).
	http.HandleFunc("/api/v1/fechamento", fechamentoHandler.Raiz)
	http.HandleFunc("/api/v1/fechamento/modo", fechamentoHandler.Modo)
	http.HandleFunc("/api/v1/fechamento/escopo", fechamentoHandler.Escopo)
	http.HandleFunc("/api/v1/fechamento/escopo/", fechamentoHandler.Escopo)
	http.HandleFunc("/api/v1/fechamento/liberar", fechamentoHandler.Liberar)
	http.HandleFunc("/api/v1/fechamento/liberar/", fechamentoHandler.Liberar)
	http.HandleFunc("/api/v1/conciliacao-ia", conciliacaoIAHandler.Analisar)
	http.HandleFunc("/api/v1/conciliacao-ia/ajuste", conciliacaoIAHandler.AplicarAjuste)
	validarHandler := handler.NewValidarExpressaoHandler()
	http.HandleFunc("/api/v1/validar-expressao", validarHandler.Validar)
	previewExprHandler := handler.NewPreviewExpressaoHandler(movimentoSvc)
	http.HandleFunc("/api/v1/preview-expressao", previewExprHandler.Preview)
	simulacaoWhatIfHandler := handler.NewSimulacaoWhatIfHandler(movimentoSvc)
	http.HandleFunc("/api/v1/simulacao-whatif", simulacaoWhatIfHandler.Simular)
	backtestHandler := handler.NewBacktestHandler(movimentoSvc)
	http.HandleFunc("/api/v1/backtest-regra", backtestHandler.Backtest)
	reprocessamentoHandler := handler.NewReprocessamentoHandler(movimentoSvc)
	http.HandleFunc("/api/v1/reprocessar", reprocessamentoHandler.Reprocessar)
	// Copiloto conversacional (RF-138).
	http.HandleFunc("/api/v1/copilot", copilotHandler.Chat)
	http.HandleFunc("/api/v1/copilot/status", copilotHandler.Status)
	http.HandleFunc("/api/v1/copilot/acao", copilotHandler.Acao)
	// [LAB] Rotas experimentais (diagnóstico de dupla contabilização + linhagem).
	http.HandleFunc("/api/v1/lab/status", labHandler.Status)
	http.HandleFunc("/api/v1/lab/diagnostico", labHandler.Diagnostico)
	http.HandleFunc("/api/v1/lab/linhagem", labHandler.Linhagem)
	http.HandleFunc("/api/v1/nlquery", nlQueryHandler.Query)
	http.HandleFunc("/api/v1/movimento-contabil/export", exportHandler.ExportMovimentoCSV)
	http.HandleFunc("/api/v1/movimento-contabil/export-txt", exportHandler.ExportMovimentoTXT)
	diffVersoesHandler := handler.NewDiffVersoesHandler(movimentoSvc)
	http.HandleFunc("/api/v1/movimento-contabil/diff", diffVersoesHandler.Diff)
	// Análises contábeis: balancete (RF-127), cobertura (RF-128), lint (RF-129), série (RF-130).
	analisesHandler := handler.NewAnalisesHandler(movimentoSvc, regraSvc)
	http.HandleFunc("/api/v1/balancete", analisesHandler.Balancete)
	http.HandleFunc("/api/v1/cobertura", analisesHandler.Cobertura)
	http.HandleFunc("/api/v1/serie-saldo", analisesHandler.Serie)
	http.HandleFunc("/api/v1/regras/lint", analisesHandler.Lint)
	http.HandleFunc("/api/v1/conciliacao", analisesHandler.Conciliacao)
	http.HandleFunc("/api/v1/posicao", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			posicaoHandler.Listar(w, r)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	http.HandleFunc("/api/v1/posicao/upload", posicaoHandler.Upload)
	http.HandleFunc("/api/v1/posicao/upload-lote", posicaoHandler.UploadLote)
	http.HandleFunc("/api/v1/posicao/campos", posicaoHandler.Campos)
	http.HandleFunc("/api/v1/posicao/scan-pasta", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			pastaWatcher.Status(w, r)
		} else {
			pastaWatcher.ScanAgora(w, r)
		}
	})
	http.HandleFunc("/api/v1/parametrizacoes/opcoes", parametrizacaoHandler.Opcoes)
	http.HandleFunc("/api/v1/parametrizacoes/padroes", padraoArquivoHandler.Padroes)
	http.HandleFunc("/api/v1/parametrizacoes/veiculo-depara", veiculoDeParaHandler.VeiculoDePara)
	http.HandleFunc("/api/v1/parametrizacoes/cotacoes", cotacaoHandler.Cotacoes)
	http.HandleFunc("/api/v1/parametrizacoes/cotacoes/import", cotacaoHandler.ImportarCSV)
	http.HandleFunc("/api/v1/razonete", razoneteHandler.Razonete)
	http.HandleFunc("/api/v1/dashboard", dashboardHandler.Dashboard)
	http.HandleFunc("/api/v1/jobs", jobHandler.Raiz)
	http.HandleFunc("/api/v1/jobs/", jobHandler.Item)
	http.HandleFunc("/api/v1/cotacoes/scan-pasta", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			cotacaoWatcher.Status(w, r)
		} else {
			cotacaoWatcher.ScanAgora(w, r)
		}
	})
	http.HandleFunc("/api/v1/configuracoes", configuracaoHandler.Configuracoes)
	http.HandleFunc("/api/v1/inconsistencias", inconsistenciaHandler.Listar)
	http.HandleFunc("/api/v1/inconsistencias/export", inconsistenciaHandler.Export)
	http.HandleFunc("/api/v1/movimento-contabil/status", execucaoHandler.Status)
	http.HandleFunc("/api/v1/movimento-contabil/calendario", execucaoHandler.Calendario)
	http.HandleFunc("/api/v1/notificacoes", notificacaoHandler.Notificacoes)
	saldoHandler := handler.NewSaldoHandler(movimentoSvc)
	http.HandleFunc("/api/v1/saldos", saldoHandler.Listar)
	lancManualHandler := handler.NewLancamentoManualHandler(movimentoSvc)
	http.HandleFunc("/api/v1/lancamentos-manuais", lancManualHandler.Inserir)
	http.HandleFunc("/api/v1/lancamentos-manuais/import", lancManualHandler.ImportarCSV)

	// Interface web (SPA) servida pelo PRÓPRIO binário: um executável único sobe API + UI
	// na mesma porta, sem proxy e sem CORS. Os exports são chamados direto (antes passavam
	// pelo proxy /consulta/*). O app.html vai embutido (internal/webui), então o exe é
	// autocontido. Como a API não registra "/", a SPA é o catch-all das demais rotas.
	http.HandleFunc("/consulta/export", exportHandler.ExportMovimentoCSV)
	http.HandleFunc("/consulta/export-txt", exportHandler.ExportMovimentoTXT)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, webui.AppHTML())
	})

	// 6. Ler porta
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	// Abre a UI no navegador padrão automaticamente (desligar com SRCOFF_NO_BROWSER=1).
	if os.Getenv("SRCOFF_NO_BROWSER") == "" {
		go abrirNavegador("http://localhost:" + port)
	}

	log.Printf("SRCOff iniciado — API e UI em http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("erro ao iniciar servidor: %v", err)
	}
}

// abrirNavegador tenta abrir a URL no navegador padrão do SO após um pequeno atraso
// (tempo de o servidor subir). Falha silenciosamente — o usuário pode abrir manualmente.
func abrirNavegador(url string) {
	time.Sleep(800 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// duracaoAteProximoHorario devolve quanto falta até a próxima ocorrência de "HH:MM" a partir
// de `agora` (hoje se ainda no futuro, senão amanhã). ok=false quando o horário é vazio/inválido
// (geração agendada desativada).
func duracaoAteProximoHorario(horario string, agora time.Time) (time.Duration, bool) {
	partes := strings.SplitN(strings.TrimSpace(horario), ":", 2)
	if len(partes) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(partes[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(partes[1]))
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	prox := time.Date(agora.Year(), agora.Month(), agora.Day(), h, m, 0, 0, agora.Location())
	if !prox.After(agora) {
		prox = prox.AddDate(0, 0, 1)
	}
	return prox.Sub(agora), true
}

// agendarGeracao enfileira a geração do contábil (RF-126) 1×/dia no HORÁRIO configurado
// (HH:MM, independente de quando a API subiu), para o escopo (produto+domínio), na ÚLTIMA
// data com posição importada. Pula se desativado (horário vazio/inválido ou escopo vazio),
// se a data já foi executada para o escopo (evita empilhar versões) ou se já há geração igual na fila.
func agendarGeracao(ctx context.Context, js *service.JobService, cfg *service.ConfiguracaoService, posRepo repository.PosicaoCarteiraRepository, execRepo repository.ExecucaoRepository) {
	horarioCfg := func() string {
		v, _ := cfg.Obter(ctx, service.ConfigGeracaoHorario)
		return strings.TrimSpace(v)
	}
	for {
		dur, ok := duracaoAteProximoHorario(horarioCfg(), time.Now())
		if !ok {
			// Desativado/ inválido: re-checa a cada hora (o usuário pode configurar depois).
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Hour):
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(dur):
		}
		if _, ok := duracaoAteProximoHorario(horarioCfg(), time.Now()); !ok {
			continue // desativado durante a espera
		}
		produto, _ := cfg.Obter(ctx, service.ConfigGeracaoProduto)
		dominio, _ := cfg.Obter(ctx, service.ConfigGeracaoDominio)
		produto, dominio = strings.TrimSpace(produto), strings.TrimSpace(dominio)
		if produto == "" || dominio == "" {
			continue // escopo não configurado
		}
		data, ok, err := posRepo.UltimaDataPosicao(ctx)
		if err != nil || !ok {
			continue // sem posição importada
		}
		dataStr := data.Format("2006-01-02")
		// Não regerar: se o escopo já foi executado nessa data, pula (evita nova versão a cada ciclo).
		if datas, err := execRepo.DatasExecutadas(ctx, produto, dominio); err == nil {
			for _, d := range datas {
				if d.Format("2006-01-02") == dataStr {
					dataStr = "" // marca como já executada
					break
				}
			}
		}
		if dataStr == "" || geracaoPendente(ctx, js, data.Format("2006-01-02"), produto, dominio) {
			continue
		}
		payload, _ := json.Marshal(map[string]string{"data": dataStr, "produto": produto, "dominio": dominio})
		desc := fmt.Sprintf("Geração agendada %s (%s/%s)", dataStr, produto, dominio)
		if _, err := js.Enfileirar(ctx, "geracao_movimento", desc, string(payload), 3); err != nil {
			log.Printf("[geracao-agendada] erro ao enfileirar: %v", err)
		}
	}
}

// geracaoPendente informa se já existe um job de geração pendente/executando para o mesmo
// (data, produto, domínio) — evita enfileirar duplicado.
func geracaoPendente(ctx context.Context, js *service.JobService, data, produto, dominio string) bool {
	for _, st := range []string{model.JobPendente, model.JobExecutando} {
		pag, err := js.Listar(ctx, st, 1, 200)
		if err != nil || pag == nil {
			continue
		}
		for _, j := range pag.Jobs {
			if j.Tipo != "geracao_movimento" {
				continue
			}
			var p struct{ Data, Produto, Dominio string }
			if json.Unmarshal([]byte(j.Payload), &p) == nil && p.Data == data && p.Produto == produto && p.Dominio == dominio {
				return true
			}
		}
	}
	return false
}

// agendarExpurgo enfileira periodicamente o job de expurgo (RF-123), respeitando o
// intervalo configurado. Só enfileira quando a retenção está habilitada (N>0) e não há
// outro expurgo pendente/executando — evitando acúmulo. O expurgo em si roda no worker.
func agendarExpurgo(ctx context.Context, js *service.JobService, exp *service.ExpurgoService) {
	for {
		horas := exp.IntervaloHoras(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(horas) * time.Hour):
		}
		if exp.DiasRetencao(ctx) <= 0 {
			continue // retenção desativada
		}
		if expurgoPendente(ctx, js) {
			continue // já há um expurgo na fila
		}
		n := exp.DiasRetencao(ctx)
		if _, err := js.Enfileirar(ctx, "expurgo_movimento", fmt.Sprintf("Expurgo: reter últimos %d dia(s)", n), "", 1); err != nil {
			log.Printf("[expurgo] erro ao enfileirar: %v", err)
		}
	}
}

// expurgoPendente informa se já existe um job de expurgo pendente ou executando.
func expurgoPendente(ctx context.Context, js *service.JobService) bool {
	for _, st := range []string{model.JobPendente, model.JobExecutando} {
		pag, err := js.Listar(ctx, st, 1, 100)
		if err != nil || pag == nil {
			continue
		}
		for _, j := range pag.Jobs {
			if j.Tipo == "expurgo_movimento" {
				return true
			}
		}
	}
	return false
}
