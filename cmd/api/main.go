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

	mux := http.NewServeMux()
	mux.HandleFunc("POST /products", createProduct(db))

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

	server := &http.Server{
		Addr:              "127.0.0.1:8081",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Iniciando StockFlow em http://localhost:8081")
	log.Fatal(server.ListenAndServe())
}
