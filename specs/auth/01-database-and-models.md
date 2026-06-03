# Especificação Técnica: Banco de Dados e Modelagem de Dados (Auth/Profile)

Esta especificação define os modelos de dados e a camada de persistência para o módulo de Autenticação e Onboarding. Esta modelagem deve ser seguida à risca para garantir a consistência e segurança das credenciais e perfis de jogadores.

---

## 1. DDL SQL (PostgreSQL)

O banco de dados PostgreSQL deve ser inicializado com os seguintes comandos DDL. As tabelas devem estar associadas a migrations gerenciadas (ex: `golang-migrate` ou ferramentas ORM como `ent` ou `GORM`).

```sql
-- Habilitar UUID
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Enums Lógicos de Domínio
CREATE TYPE user_status AS ENUM ('onboarding_pending', 'active', 'blocked');
CREATE TYPE auth_provider AS ENUM ('local', 'google');

-- 2. Tabela de Credenciais de Usuários
CREATE TABLE usuarios (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NULL, -- Nulo se provider for 'google'
    provider auth_provider NOT NULL DEFAULT 'local',
    status user_status NOT NULL DEFAULT 'onboarding_pending',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 3. Tabela de Perfis Públicos
CREATE TABLE perfis (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID UNIQUE NOT NULL,
    display_name VARCHAR(50) NOT NULL,
    bio VARCHAR(200) NOT NULL DEFAULT '',
    photo_url VARCHAR(512) NOT NULL DEFAULT '',
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES usuarios(id) ON DELETE CASCADE
);

-- 4. Tabela de Gerenciamento de Sessão (Refresh Tokens)
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    token_hash VARCHAR(255) UNIQUE NOT NULL, -- Hash SHA-256 do token opaco gerado
    family_id UUID NOT NULL,                 -- Identifica a família/dispositivo de login
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at TIMESTAMP WITH TIME ZONE NULL, -- Para controle de Grace Period
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_refresh_user FOREIGN KEY (user_id) REFERENCES usuarios(id) ON DELETE CASCADE
);

-- 5. Índices de Otimização e Segurança
CREATE INDEX idx_usuarios_email ON usuarios(email);
CREATE INDEX idx_usuarios_status ON usuarios(status);
CREATE INDEX idx_refresh_tokens_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens(family_id);
```

---

## 2. Modelagem Go (Structs)

As representações de dados em Go devem utilizar tipos estritos e tags para mapeamento de banco e serialização JSON. 

### 2.1. Struct `Usuario`
```go
package models

import (
	"time"
	"github.com/google/uuid"
)

type UserStatus string
type AuthProvider string

const (
	StatusOnboardingPending UserStatus = "onboarding_pending"
	StatusActive             UserStatus = "active"
	StatusBlocked            UserStatus = "blocked"

	ProviderLocal  AuthProvider = "local"
	ProviderGoogle AuthProvider = "google"
)

type Usuario struct {
	ID           uuid.UUID    `json:"id" db:"id"`
	Email        string       `json:"email" db:"email"`
	PasswordHash *string      `json:"-" db:"password_hash"` // Ponteiro para aceitar nulo
	Provider     AuthProvider `json:"provider" db:"provider"`
	Status       UserStatus   `json:"status" db:"status"`
	CreatedAt    time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at" db:"updated_at"`
}
```

### 2.2. Struct `Perfil`
```go
package models

import (
	"time"
	"github.com/google/uuid"
)

type Perfil struct {
	ID          uuid.UUID `json:"id" db:"id"`
	UserID      uuid.UUID `json:"user_id" db:"user_id"`
	DisplayName string    `json:"display_name" db:"display_name"`
	Bio         string    `json:"bio" db:"bio"`
	PhotoURL    string    `json:"photo_url" db:"photo_url"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}
```

### 2.3. Struct `RefreshToken`
```go
package models

import (
	"time"
	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID  `db:"id"`
	UserID    uuid.UUID  `db:"user_id"`
	TokenHash string     `db:"token_hash"`
	FamilyID  uuid.UUID  `db:"family_id"`
	ExpiresAt time.Time  `db:"expires_at"`
	Revoked   bool       `db:"revoked"`
	RevokedAt *time.Time `db:"revoked_at"`
	CreatedAt time.Time  `db:"created_at"`
}
```

---

## 3. Critérios de Aceitação e Testes de Modelo

Para que esta especificação de modelagem seja considerada implementada, a suíte de testes de integração do banco de dados deve cobrir e validar:

1.  **Restrição de Unicidade:** Tentar inserir dois usuários com o mesmo e-mail deve falhar com erro de chave duplicada (`pgcode 23505`).
2.  **Integridade de Chave Estrangeira:** Inserir um registro em `perfis` com um `user_id` inexistente na tabela `usuarios` deve ser rejeitado.
3.  **Cascateamento:** A deleção de um registro na tabela `usuarios` deve apagar automaticamente (cascade) os registros de `perfis` e `refresh_tokens` associados.
4.  **Mutação de Status:** Testes unitários devem validar as transições de estados permitidas conforme a máquina de estados.
