package main

import (
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
)

func main() {
	tmpl := template.Must(template.ParseGlob("cmd/frontend/templates/*.html"))

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	frontendPort := os.Getenv("FRONTEND_PORT")
	if frontendPort == "" {
		frontendPort = "9090"
	}

	client := &http.Client{}

	// Proxy reverso: /api/* → API REST (mantém o path completo)
	http.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		target := apiURL + r.URL.RequestURI()
		proxyRequest(client, w, r, target)
	})

	// Export CSV — passa query string para a API
	http.HandleFunc("/consulta/export", func(w http.ResponseWriter, r *http.Request) {
		target := apiURL + "/api/v1/movimento-contabil/export?" + r.URL.RawQuery
		proxyDownload(client, w, r, target)
	})

	// Export TXT — passa query string para a API
	http.HandleFunc("/consulta/export-txt", func(w http.ResponseWriter, r *http.Request) {
		dataParam := r.URL.Query().Get("data")
		target := apiURL + "/api/v1/movimento-contabil/export-txt?data=" + url.QueryEscape(dataParam)
		proxyDownload(client, w, r, target)
	})

	// SPA — serve app.html para todas as rotas de navegação
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "app.html", nil); err != nil {
			log.Printf("erro ao renderizar app.html: %v", err)
			http.Error(w, "Erro interno", http.StatusInternalServerError)
		}
	})

	log.Printf("Frontend SRCOff iniciado na porta %s", frontendPort)
	if err := http.ListenAndServe(":"+frontendPort, nil); err != nil {
		log.Fatalf("erro ao iniciar servidor frontend: %v", err)
	}
}

// proxyRequest faz proxy de qualquer método HTTP para a API, repassando body e headers.
func proxyRequest(client *http.Client, w http.ResponseWriter, r *http.Request, target string) {
	req, err := http.NewRequest(r.Method, target, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header = r.Header.Clone()

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// proxyDownload faz proxy de download (GET) repassando Content-Disposition.
func proxyDownload(client *http.Client, w http.ResponseWriter, r *http.Request, target string) {
	resp, err := client.Get(target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Content-Disposition", resp.Header.Get("Content-Disposition"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
