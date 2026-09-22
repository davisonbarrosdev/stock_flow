package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type user struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"` // 'admin' ou 'employee'
	CreatedAt time.Time `json:"created_at"`
}

type userInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
	User  user   `json:"user"`
}

// Memory session storage token -> user
var (
	sessions = make(map[string]user)
	sessionMu sync.RWMutex
)

func generateToken() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(bytes)
}

func ensureAdminUser(db *pgxpool.Pool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Erro ao gerar hash da senha admin: %v", err)
		return
	}

	var exists bool
	err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(email) = 'admin@stockflow.com')`).Scan(&exists)
	if err != nil {
		log.Printf("Aviso: Tabela 'users' ainda não existe ou erro ao verificar: %v", err)
		return
	}

	if !exists {
		_, err = db.Exec(ctx, `
			INSERT INTO users (name, email, password_hash, role)
			VALUES ($1, $2, $3, $4)
		`, "Administrador", "admin@stockflow.com", string(hash), "admin")
		if err != nil {
			log.Printf("Erro ao criar admin padrão: %v", err)
		} else {
			log.Println("==> Usuário Admin criado com sucesso: admin@stockflow.com / admin123")
		}
	} else {
		var currentHash string
		_ = db.QueryRow(ctx, `SELECT password_hash FROM users WHERE LOWER(email) = 'admin@stockflow.com'`).Scan(&currentHash)
		if bcrypt.CompareHashAndPassword([]byte(currentHash), []byte("admin123")) != nil {
			_, _ = db.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE LOWER(email) = 'admin@stockflow.com'`, string(hash))
			log.Println("==> Senha do Admin redefinida para 'admin123'")
		}
	}
}

func getUserFromRequest(r *http.Request) (user, bool) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return user{}, false
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return user{}, false
	}

	sessionMu.RLock()
	u, found := sessions[token]
	sessionMu.RUnlock()

	return u, found
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := getUserFromRequest(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autorizado. Faça login primeiro."})
			return
		}
		// Injeta no contexto se necessário
		ctx := context.WithValue(r.Context(), "user", u)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u, _ := getUserFromRequest(r)
		if u.Role != "admin" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Acesso restrito a Administradores"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loginUser(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var input *loginInput
		if err := decoder.Decode(&input); err != nil || input == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie email e senha válidos"})
			return
		}

		input.Email = strings.ToLower(strings.TrimSpace(input.Email))
		if input.Email == "" || input.Password == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Email e senha são obrigatórios"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var u user
		var passHash string
		err := db.QueryRow(ctx, `SELECT id, name, email, password_hash, role, created_at FROM users WHERE LOWER(email) = $1`, input.Email).
			Scan(&u.ID, &u.Name, &u.Email, &passHash, &u.Role, &u.CreatedAt)

		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Email ou senha incorretos"})
			return
		}
		if err != nil {
			log.Printf("Erro ao consultar usuário: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Erro interno ao processar login"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(passHash), []byte(input.Password)); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Email ou senha incorretos"})
			return
		}

		token := generateToken()
		sessionMu.Lock()
		sessions[token] = u
		sessionMu.Unlock()

		writeJSON(w, http.StatusOK, loginResponse{Token: token, User: u})
	}
}

func createUser(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var input *userInput
		if err := decoder.Decode(&input); err != nil || input == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie um objeto JSON válido"})
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie apenas um objeto JSON"})
			return
		}

		input.Name = strings.TrimSpace(input.Name)
		input.Email = strings.ToLower(strings.TrimSpace(input.Email))
		input.Role = strings.ToLower(strings.TrimSpace(input.Role))

		if input.Role == "" {
			input.Role = "employee"
		}

		if input.Name == "" || utf8.RuneCountInString(input.Name) > 150 ||
			input.Email == "" || utf8.RuneCountInString(input.Email) > 150 ||
			utf8.RuneCountInString(input.Password) < 6 ||
			(input.Role != "admin" && input.Role != "employee") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Nome e email são obrigatórios (máx 150), a senha deve ter no mínimo 6 caracteres e o cargo deve ser 'admin' ou 'employee'"})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Erro ao criptografar senha: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível cadastrar o usuário"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var result user
		result.Name = input.Name
		result.Email = input.Email
		result.Role = input.Role

		err = db.QueryRow(ctx,
			`INSERT INTO users (name, email, password_hash, role) VALUES ($1, $2, $3, $4) RETURNING id, created_at`,
			input.Name, input.Email, string(hash), input.Role,
		).Scan(&result.ID, &result.CreatedAt)

		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "Já existe um usuário cadastrado com este email"})
				return
			}
			log.Printf("Erro ao cadastrar usuário: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível cadastrar o usuário"})
			return
		}

		writeJSON(w, http.StatusCreated, result)
	}
}

func listUsers(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		rows, err := db.Query(ctx, `SELECT id, name, email, role, created_at FROM users ORDER BY id ASC`)
		if err != nil {
			log.Printf("Erro ao listar usuários: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível listar os usuários"})
			return
		}
		defer rows.Close()

		usersList := make([]user, 0)
		for rows.Next() {
			var u user
			if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.CreatedAt); err != nil {
				log.Printf("Erro ao ler usuário: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível listar os usuários"})
				return
			}
			usersList = append(usersList, u)
		}
		if err := rows.Err(); err != nil {
			log.Printf("Erro ao percorrer usuários: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível listar os usuários"})
			return
		}

		writeJSON(w, http.StatusOK, usersList)
	}
}

func deleteUser(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "O ID deve ser um número inteiro positivo"})
			return
		}

		currentUser, _ := getUserFromRequest(r)
		if currentUser.ID == id {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Você não pode excluir sua própria conta de administrador"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		tag, err := db.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
		if err != nil {
			log.Printf("Erro ao deletar usuário: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível deletar o usuário"})
			return
		}
		if tag.RowsAffected() == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Usuário não encontrado"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"message": "Usuário removido com sucesso"})
	}
}

func getMe(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := getUserFromRequest(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autenticado"})
			return
		}
		writeJSON(w, http.StatusOK, u)
	}
}
