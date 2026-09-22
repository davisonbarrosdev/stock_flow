-- Script completo para inicialização do Banco de Dados StockFlow
-- Este script cria a estrutura necessária para Produtos e Usuários (Admin e Funcionários)

BEGIN;

-- 1. Tabela de Produtos
CREATE TABLE IF NOT EXISTS products (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sku VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(150) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    price_cents BIGINT NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
    stock_quantity INTEGER NOT NULL DEFAULT 0 CHECK (stock_quantity >= 0),
    minimum_stock INTEGER NOT NULL DEFAULT 0 CHECK (minimum_stock >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (length(trim(sku)) > 0),
    CHECK (length(trim(name)) > 0)
);

-- 2. Tabela de Usuários (Funcionários e Admins)
CREATE TABLE IF NOT EXISTS users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name VARCHAR(150) NOT NULL,
    email VARCHAR(150) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'employee' CHECK (role IN ('admin', 'employee')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (length(trim(name)) > 0),
    CHECK (length(trim(email)) > 0)
);

-- 3. Inserir Administrador Padrão (se não existir nenhum usuário)
-- Email: admin@stockflow.com | Senha: admin123 (hash bcrypt)
INSERT INTO users (name, email, password_hash, role)
SELECT 
    'Administrador Geral', 
    'admin@stockflow.com', 
    '$2a$10$3zR1Y0hQZ1bE64d4G1w4e.W7uP9/0pM9aO3zR1Y0hQZ1bE64d4G1w', -- Hash bcrypt de 'admin123'
    'admin'
WHERE NOT EXISTS (SELECT 1 FROM users);

COMMIT;
