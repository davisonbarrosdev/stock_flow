package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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

func productTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
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
	t.Cleanup(db.Close)
	migration, err := os.ReadFile("../../migrations/001_create_products.sql")
	if err != nil {
		t.Fatal(err)
	}
	// A tabela temporária existe apenas nesta conexão; os produtos reais não são alterados.
	sql := strings.Replace(string(migration), "CREATE TABLE products", "CREATE TEMP TABLE products", 1)
	if _, err := db.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCreateProductDatabase(t *testing.T) {
	db := productTestDB(t)
	ctx := context.Background()
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

func TestListProductsDatabase(t *testing.T) {
	db := productTestDB(t)
	handler := listProducts(db)
	get := func() *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler(response, httptest.NewRequest("GET", "/products", nil))
		return response
	}
	empty := get()
	if empty.Code != 200 || strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("lista vazia: %d %s", empty.Code, empty.Body.String())
	}
	if _, err := db.Exec(context.Background(), `INSERT INTO pg_temp.products (sku,name,description,price_cents,stock_quantity,minimum_stock) VALUES ('B','Caderno','100 folhas',1990,10,2), ('A','Lápis','',250,20,5)`); err != nil {
		t.Fatal(err)
	}
	response := get()
	if response.Code != 200 || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("resposta: %d %s", response.Code, response.Body.String())
	}
	var items []product
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("quantidade: %d", len(items))
	}
	first := items[0]
	if first.SKU != "B" || first.ID >= items[1].ID || first.Name != "Caderno" || first.Description != "100 folhas" || first.PriceCents != 1990 || first.StockQuantity != 10 || first.MinimumStock != 2 || first.CreatedAt.IsZero() {
		t.Fatalf("produto ou ordem inesperada: %+v", items)
	}
	db.Close()
	failure := get()
	if failure.Code != 500 || !strings.Contains(failure.Body.String(), "Não foi possível listar os produtos") {
		t.Fatalf("falha: %d %s", failure.Code, failure.Body.String())
	}
}

func TestGetProductInvalidID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /products/{id}", getProduct(nil))
	for _, id := range []string{"abc", "0", "-1", "1.5", "9223372036854775808"} {
		t.Run(id, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest("GET", "/products/"+id, nil))
			if response.Code != 400 {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestGetProductDatabase(t *testing.T) {
	db := productTestDB(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /products/{id}", getProduct(db))
	request := func(id string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("GET", "/products/"+id, nil))
		if response.Header().Get("Content-Type") != "application/json" {
			t.Fatal("resposta não é JSON")
		}
		return response
	}
	missing := request("1")
	if missing.Code != 404 || !strings.Contains(missing.Body.String(), "Produto não encontrado") {
		t.Fatalf("ausente: %d %s", missing.Code, missing.Body.String())
	}
	var id int64
	err := db.QueryRow(context.Background(), `INSERT INTO pg_temp.products (sku,name,description,price_cents,stock_quantity,minimum_stock) VALUES ('TEST','Caderno','100 folhas',1990,10,2) RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	response := request(strconv.FormatInt(id, 10))
	if response.Code != 200 {
		t.Fatalf("consulta: %d %s", response.Code, response.Body.String())
	}
	var item product
	if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	if item.ID != id || item.SKU != "TEST" || item.Name != "Caderno" || item.Description != "100 folhas" || item.PriceCents != 1990 || item.StockQuantity != 10 || item.MinimumStock != 2 || item.CreatedAt.IsZero() {
		t.Fatalf("produto inesperado: %+v", item)
	}
	db.Close()
	failure := request(strconv.FormatInt(id, 10))
	if failure.Code != 500 || !strings.Contains(failure.Body.String(), "Não foi possível consultar o produto") {
		t.Fatalf("falha: %d %s", failure.Code, failure.Body.String())
	}
}
