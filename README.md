# StockFlow — Controle de Estoque Inteligente & Funcionários

API RESTful em Go com PostgreSQL e Frontend integrado em HTML/CSS/JS dark mode.

---

## 🚀 Como Subir o Banco de Dados

### Opção A: Via Docker Compose (Mais Fácil)

Para subir o PostgreSQL com todas as tabelas e o administrador inicial já configurados:

```sh
docker-compose up -d
```

String de conexão padrão:
```sh
export DATABASE_URL='postgres://dev:devpassword@localhost:5432/stockflow?sslmode=disable'
```

---

### Opção B: PostgreSQL Local Existente

Se já tiver um servidor PostgreSQL rodando localmente, execute o script de inicialização do banco:

```sh
export DATABASE_URL='postgres://usuario:senha@localhost:5432/stockflow?sslmode=disable'
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f scripts/setup_database.sql
```

Ou execute as migrações em ordem:
```sh
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/001_create_products.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/002_create_users.sql
```

---

## 🏃 Executar a Aplicação Go

```sh
go run ./cmd/api
```

Acesse no navegador: **`http://localhost:8081`**

### 🔑 Credenciais Padrão do Administrador
- **E-mail**: `admin@stockflow.com`
- **Senha**: `admin123`

---

## 👥 Permissões e Acessos

| Perfil | Ações Permitidas |
|--------|-------------------|
| **Funcionário (`employee`)** | Consultar produtos, buscar itens e realizar entrada/saída rápida de estoque. |
| **Administrador (`admin`)** | Tudo do funcionário + Criar, Editar e Deletar Produtos, Cadastrar e Excluir Funcionários. |

---

## 🧪 Testes

```sh
go vet ./...
go build ./...
TEST_DATABASE_URL="$DATABASE_URL" go test ./... -v
```
