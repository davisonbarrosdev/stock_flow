# StockFlow

API de estoque em Go com PostgreSQL.

## Executar

Configure `DATABASE_URL` para seu PostgreSQL local e execute:

```sh
export DATABASE_URL='postgres://dev@localhost:5432/stockflow?sslmode=disable'
go run ./cmd/api
```

A API escuta em `http://localhost:8081`. Antes da primeira execução, aplique a migração em um banco sem a tabela `products`:

```sh
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/001_create_products.sql
```

## Rotas

- `GET /health`: verifica se a API está respondendo.
- `POST /products`: cadastra um produto; retorna 201, 400 para dados inválidos, 409 para SKU duplicado ou 500 para falha no banco.

```sh
curl -i http://localhost:8081/products \
  -H 'Content-Type: application/json' \
  -d '{"sku":"PROD-001","name":"Caderno","description":"Caderno de 100 folhas","price_cents":1990,"stock_quantity":10,"minimum_stock":2}'
```

SKU e nome são obrigatórios, com limites de 50 e 150 caracteres. Preço é informado em centavos; quantidades são inteiras. Preço e quantidades não podem ser negativos. Campos numéricos omitidos recebem zero.

## Testes

```sh
go test ./...
go vet ./...
TEST_DATABASE_URL="$DATABASE_URL" go test ./... -v
```

O teste com PostgreSQL usa uma tabela temporária na própria conexão, sem alterar os produtos cadastrados.
