package main

import (
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
		posicaoRepo   repository.PosicaoCarteiraRepository
		regraRepo     repository.RegraContabilRepository
		movimentoRepo repository.MovimentoContabilRepository
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

	default: // sqlserver
		rawSQLDB = db.Connect()
		defer rawSQLDB.Close()
		log.Printf("Backend: SQL Server")
		posicaoRepo = repository.NewPosicaoCarteiraRepo(rawSQLDB)
		regraRepo = repository.NewRegraContabilRepo(rawSQLDB)
		movimentoRepo = repository.NewMovimentoContabilRepo(rawSQLDB)
	}

	// 2. Instanciar avaliador de expressões
	eval := evaluator.New()

	// 3. Instanciar serviços
	movimentoSvc := service.NewMovimentoContabilService(posicaoRepo, regraRepo, movimentoRepo, eval)
	regraSvc := service.NewRegraContabilService(regraRepo)
	conciliacaoSvc := service.NewConciliacaoService(posicaoRepo, movimentoRepo)
	posicaoSvc := service.NewPosicaoCarteiraService(posicaoRepo)

	// 4. Instanciar handlers
	movimentoHandler := handler.NewMovimentoContabilHandler(movimentoSvc)
	regraHandler := handler.NewRegraContabilHandler(regraSvc)
	conciliacaoHandler := handler.NewConciliacaoHandler(conciliacaoSvc)
	posicaoHandler := handler.NewPosicaoCarteiraHandler(posicaoSvc)
	exportHandler := handler.NewExportHandler(movimentoSvc)
	nlQueryHandler := handler.NewNLQueryHandler(rawSQLDB)

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

	http.HandleFunc("/api/v1/regras/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/condicoes") {
			if r.Method == http.MethodGet {
				regraHandler.ListarCondicoes(w, r)
			} else {
				regraHandler.CriarCondicao(w, r)
			}
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
	http.HandleFunc("/api/v1/nlquery", nlQueryHandler.Query)
	http.HandleFunc("/api/v1/movimento-contabil/export", exportHandler.ExportMovimentoCSV)
	http.HandleFunc("/api/v1/movimento-contabil/export-txt", exportHandler.ExportMovimentoTXT)
	http.HandleFunc("/api/v1/posicao", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			posicaoHandler.Listar(w, r)
		case http.MethodPost:
			posicaoHandler.Inserir(w, r)
		case http.MethodDelete:
			posicaoHandler.Deletar(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

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
