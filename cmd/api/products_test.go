package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateProductInvalidInput(t *testing.T) {
	for _, body := range []string{`null`, `{`, `{}`, `{"sku":"A","name":"B","price_cents":-1}`, `{"sku":"A","name":"B","stock_quantity":2147483648}`, `{"sku":"A","name":"B","extra":true}`, `{"sku":"A","name":"B"} {}`, `{"sku":"A","name":"B","description":"\u0000"}`} {
		t.Run(body, func(t *testing.T) {
			response := httptest.NewRecorder()
			createProduct(nil)(response, httptest.NewRequest("POST", "/products", strings.NewReader(body)))
			if response.Code != 400 {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCreateProductDatabase(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("Defina TEST_DATABASE_URL para testar com PostgreSQL")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migration, err := os.ReadFile("../../migrations/001_create_products.sql")
	if err != nil {
		t.Fatal(err)
	}
	// A tabela temporária existe apenas nesta conexão; os produtos reais não são alterados.
	sql := strings.Replace(string(migration), "CREATE TABLE products", "CREATE TEMP TABLE products", 1)
	if _, err := db.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	body := `{"sku":" TEST-001 ","name":" Produto teste ","price_cents":1990,"stock_quantity":5,"minimum_stock":2}`
	for i, expected := range []int{201, 409} {
		response := httptest.NewRecorder()
		createProduct(db)(response, httptest.NewRequest("POST", "/products", strings.NewReader(body)))
		if response.Code != expected {
			t.Fatalf("status %d: %s", response.Code, response.Body.String())
		}
		if i == 0 {
			var result product
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.ID == 0 || result.SKU != "TEST-001" || result.Name != "Produto teste" || result.PriceCents != 1990 || result.CreatedAt.IsZero() {
				t.Fatalf("produto inesperado: %+v", result)
			}
		}
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM pg_temp.products").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("produtos: %d", count)
	}
}
