package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"strings"

	"srcoff/internal/db"
	"srcoff/internal/evaluator"
	"srcoff/internal/handler"
	"srcoff/internal/repository"
	filerepo "srcoff/internal/repository/file"
	"srcoff/internal/service"
)

func main() {
	// 1. Selecionar backend de armazenamento via STORAGE_BACKEND (sqlserver | file)
	backend := os.Getenv("STORAGE_BACKEND")
	if backend == "" {
		backend = "file"
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
	)

	var rawSQLDB *sql.DB

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

	default: // sqlserver
		rawSQLDB = db.Connect()
		defer rawSQLDB.Close()
		log.Printf("Backend: SQL Server")
		posicaoRepo = repository.NewPosicaoCarteiraRepo(rawSQLDB)
		regraRepo = repository.NewRegraContabilRepo(rawSQLDB)
		movimentoRepo = repository.NewMovimentoContabilRepo(rawSQLDB)
		parametrizacaoRepo = repository.NewParametrizacaoRepo(rawSQLDB)
		inconsistenciaRepo = repository.NewInconsistenciaRepo(rawSQLDB)
		padraoArquivoRepo = repository.NewPadraoArquivoRepo(rawSQLDB)
		configuracaoRepo = repository.NewConfiguracaoRepo(rawSQLDB)
		execucaoRepo = repository.NewExecucaoRepo(rawSQLDB)
		notificacaoRepo = repository.NewNotificacaoRepo(rawSQLDB)
	}

	// 2. Instanciar avaliador de expressões
	eval := evaluator.New()

	// 3. Instanciar serviços
	movimentoSvc := service.NewMovimentoContabilService(posicaoRepo, regraRepo, movimentoRepo, eval).
		ComInconsistenciaRepo(inconsistenciaRepo).
		ComExecucaoRepo(execucaoRepo).
		ComPadraoRepo(padraoArquivoRepo)
	inconsistenciaSvc := service.NewInconsistenciaService(inconsistenciaRepo)
	execucaoSvc := service.NewExecucaoService(execucaoRepo, padraoArquivoRepo).
		ComInconsistenciaRepo(inconsistenciaRepo)
	notificacaoSvc := service.NewNotificacaoService(notificacaoRepo)
	regraSvc := service.NewRegraContabilService(regraRepo)
	conciliacaoSvc := service.NewConciliacaoService(posicaoRepo, movimentoRepo)
	posicaoSvc := service.NewPosicaoCarteiraService(posicaoRepo)
	parametrizacaoSvc := service.NewParametrizacaoService(parametrizacaoRepo)
	padraoArquivoSvc := service.NewPadraoArquivoService(padraoArquivoRepo)
	configuracaoSvc := service.NewConfiguracaoService(configuracaoRepo)

	// 4. Instanciar handlers
	movimentoHandler := handler.NewMovimentoContabilHandler(movimentoSvc)
	regraHandler := handler.NewRegraContabilHandler(regraSvc)
	conciliacaoHandler := handler.NewConciliacaoHandler(conciliacaoSvc)
	conciliacaoIAHandler := handler.NewConciliacaoIAHandler(movimentoSvc, posicaoSvc)
	posicaoHandler := handler.NewPosicaoCarteiraHandler(posicaoSvc, padraoArquivoSvc)
	parametrizacaoHandler := handler.NewParametrizacaoHandler(parametrizacaoSvc)
	padraoArquivoHandler := handler.NewPadraoArquivoHandler(padraoArquivoSvc)
	configuracaoHandler := handler.NewConfiguracaoHandler(configuracaoSvc)
	inconsistenciaHandler := handler.NewInconsistenciaHandler(inconsistenciaSvc)
	execucaoHandler := handler.NewExecucaoHandler(execucaoSvc)
	notificacaoHandler := handler.NewNotificacaoHandler(notificacaoSvc)
	exportHandler := handler.NewExportHandler(movimentoSvc)
	nlQueryHandler := handler.NewNLQueryHandler(rawSQLDB)

	// Watcher de pasta monitorada (varredura periódica em segundo plano).
	// O intervalo (em minutos) é parametrizável; sem parametrização, não varre.
	// A importação automática dispara o contábil automaticamente e gera notificações.
	pastaWatcher := handler.NewPastaWatcher(posicaoSvc, padraoArquivoSvc, configuracaoSvc, movimentoSvc, notificacaoSvc)
	pastaWatcher.Start(context.Background())

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
		if strings.HasSuffix(path, "/condicoes") {
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
		if r.Method == http.MethodDelete {
			regraHandler.ExcluirCondicao(w, r)
		} else {
			regraHandler.EditarCondicao(w, r)
		}
	})
	http.HandleFunc("/api/v1/conciliacao", conciliacaoHandler.Conciliar)
	http.HandleFunc("/api/v1/conciliacao-ia", conciliacaoIAHandler.Analisar)
	http.HandleFunc("/api/v1/conciliacao-ia/ajuste", conciliacaoIAHandler.AplicarAjuste)
	validarHandler := handler.NewValidarExpressaoHandler()
	http.HandleFunc("/api/v1/validar-expressao", validarHandler.Validar)
	http.HandleFunc("/api/v1/nlquery", nlQueryHandler.Query)
	http.HandleFunc("/api/v1/movimento-contabil/export", exportHandler.ExportMovimentoCSV)
	http.HandleFunc("/api/v1/movimento-contabil/export-txt", exportHandler.ExportMovimentoTXT)
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
	http.HandleFunc("/api/v1/configuracoes", configuracaoHandler.Configuracoes)
	http.HandleFunc("/api/v1/inconsistencias", inconsistenciaHandler.Listar)
	http.HandleFunc("/api/v1/inconsistencias/export", inconsistenciaHandler.Export)
	http.HandleFunc("/api/v1/movimento-contabil/status", execucaoHandler.Status)
	http.HandleFunc("/api/v1/movimento-contabil/calendario", execucaoHandler.Calendario)
	http.HandleFunc("/api/v1/notificacoes", notificacaoHandler.Notificacoes)

	// 6. Ler porta
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("API SRCOff iniciada na porta %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("erro ao iniciar servidor: %v", err)
	}
}
