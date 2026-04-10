package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"srcoff/internal/db"
	"srcoff/internal/evaluator"
	"srcoff/internal/handler"
	"srcoff/internal/repository"
	"srcoff/internal/service"
)

func main() {
	// 1. Inicializar banco de dados
	sqlDB := db.Connect()
	defer sqlDB.Close()

	// 2. Instanciar repositórios
	posicaoRepo := repository.NewPosicaoCarteiraRepo(sqlDB)
	regraRepo := repository.NewRegraContabilRepo(sqlDB)
	movimentoRepo := repository.NewMovimentoContabilRepo(sqlDB)

	// 3. Instanciar avaliador de expressões
	eval := evaluator.New()

	// 4. Instanciar serviços
	movimentoSvc := service.NewMovimentoContabilService(posicaoRepo, regraRepo, movimentoRepo, eval)
	regraSvc := service.NewRegraContabilService(regraRepo)

	// 5. Instanciar handlers
	movimentoHandler := handler.NewMovimentoContabilHandler(movimentoSvc)
	regraHandler := handler.NewRegraContabilHandler(regraSvc)

	// 6. Registrar rotas
	http.HandleFunc("/api/v1/movimento-contabil", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			movimentoHandler.ConsultarMovimento(w, r)
		} else {
			movimentoHandler.GerarMovimento(w, r)
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

	http.HandleFunc("/api/v1/condicoes/", regraHandler.EditarCondicao)

	// 7. Ler porta da variável de ambiente
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	// 8. Log de inicialização
	log.Printf("API SRCOff iniciada na porta %s", port)

	// 9. Iniciar servidor
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("erro ao iniciar servidor: %v", err)
	}
}
