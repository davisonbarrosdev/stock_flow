package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("Defina a variável DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		cancel()
		log.Fatalf("Erro ao configurar o banco: %v", err)
	}

	err = db.Ping(ctx)
	cancel()

	if err != nil {
		db.Close()
		log.Fatalf("Erro ao conectar ao PostgreSQL: %v", err)
	}

	defer db.Close()

	// Garantir que a tabela de usuários exista/admin inicial seja configurado
	ensureAdminUser(db)

	mux := http.NewServeMux()

	// Auth & Users
	mux.HandleFunc("POST /login", loginUser(db))
	mux.HandleFunc("GET /me", requireAuth(getMe(db)))
	mux.HandleFunc("GET /users", requireAdmin(listUsers(db)))
	mux.HandleFunc("POST /users", requireAdmin(createUser(db)))
	mux.HandleFunc("DELETE /users/{id}", requireAdmin(deleteUser(db)))

	// Products (Leitura = qualquer usuário logado; Alteração = Admin; Estoque = Auth)
	mux.HandleFunc("GET /products", requireAuth(listProducts(db)))
	mux.HandleFunc("GET /products/{id}", requireAuth(getProduct(db)))
	mux.HandleFunc("POST /products", requireAdmin(createProduct(db)))
	mux.HandleFunc("PUT /products/{id}", requireAdmin(updateProduct(db)))
	mux.HandleFunc("DELETE /products/{id}", requireAdmin(deleteProduct(db)))
	mux.HandleFunc("PATCH /products/{id}/stock", requireAuth(adjustStock(db)))
	mux.HandleFunc("GET /dashboard", requireAuth(dashboardStats(db)))

	// Health Check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		resposta := map[string]string{
			"status":  "ok",
			"service": "stockflow-api",
		}

		if err := json.NewEncoder(w).Encode(resposta); err != nil {
			log.Printf("Erro ao enviar resposta: %v", err)
		}
	})

	// Servir arquivos estáticos do diretório web/
	mux.Handle("/", http.FileServer(http.Dir("web")))

	server := &http.Server{
		Addr:              "127.0.0.1:8081",
		Handler:           enableCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Iniciando StockFlow em http://localhost:8081")
	log.Fatal(server.ListenAndServe())
}
