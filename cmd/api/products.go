package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type productInput struct {
	SKU           string `json:"sku"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	PriceCents    int64  `json:"price_cents"`
	StockQuantity int32  `json:"stock_quantity"`
	MinimumStock  int32  `json:"minimum_stock"`
}

type product struct {
	ID int64 `json:"id"`
	productInput
	CreatedAt time.Time `json:"created_at"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("Erro ao enviar JSON: %v", err)
	}
}

func createProduct(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var input *productInput
		if err := decoder.Decode(&input); err != nil || input == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie um objeto JSON válido com os campos do produto"})
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie apenas um objeto JSON"})
			return
		}
		input.SKU = strings.TrimSpace(input.SKU)
		input.Name = strings.TrimSpace(input.Name)
		if input.SKU == "" || utf8.RuneCountInString(input.SKU) > 50 || input.Name == "" || utf8.RuneCountInString(input.Name) > 150 || input.PriceCents < 0 || input.StockQuantity < 0 || input.MinimumStock < 0 || strings.ContainsRune(input.SKU+input.Name+input.Description, 0) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "SKU (até 50 caracteres) e nome (até 150) são obrigatórios; preço e quantidades devem ser não negativos; textos não podem conter caractere nulo"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result := product{productInput: *input}
		err := db.QueryRow(ctx, `INSERT INTO products (sku, name, description, price_cents, stock_quantity, minimum_stock) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, created_at`, input.SKU, input.Name, input.Description, input.PriceCents, input.StockQuantity, input.MinimumStock).Scan(&result.ID, &result.CreatedAt)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "Já existe um produto com este SKU"})
				return
			}
			log.Printf("Erro ao cadastrar produto: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível cadastrar o produto"})
			return
		}
		writeJSON(w, http.StatusCreated, result)
	}
}

func listProducts(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := db.Query(ctx, `SELECT id, sku, name, description, price_cents, stock_quantity, minimum_stock, created_at FROM products ORDER BY id ASC`)
		if err != nil {
			log.Printf("Erro ao listar produtos: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível listar os produtos"})
			return
		}
		defer rows.Close()
		products := make([]product, 0)
		for rows.Next() {
			var item product
			if err := rows.Scan(&item.ID, &item.SKU, &item.Name, &item.Description, &item.PriceCents, &item.StockQuantity, &item.MinimumStock, &item.CreatedAt); err != nil {
				log.Printf("Erro ao ler produto: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível listar os produtos"})
				return
			}
			products = append(products, item)
		}
		if err := rows.Err(); err != nil {
			log.Printf("Erro ao percorrer produtos: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível listar os produtos"})
			return
		}
		writeJSON(w, http.StatusOK, products)
	}
}

func getProduct(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "O ID deve ser um número inteiro positivo"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var item product
		err = db.QueryRow(ctx, `SELECT id, sku, name, description, price_cents, stock_quantity, minimum_stock, created_at FROM products WHERE id = $1`, id).Scan(&item.ID, &item.SKU, &item.Name, &item.Description, &item.PriceCents, &item.StockQuantity, &item.MinimumStock, &item.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Produto não encontrado"})
			return
		}
		if err != nil {
			log.Printf("Erro ao consultar produto: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível consultar o produto"})
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}
