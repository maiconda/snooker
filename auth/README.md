# Auth Service

Servico isolado de autenticacao do Snooker.

## Ownership

- Codigo: `auth/`
- Processo: container `snooker-auth`
- Banco: container `snooker-auth-postgres`
- Tabelas: `usuarios`, `refresh_tokens`

## API

- `GET /health/live`
- `GET /health/ready`
- `POST /api/v1/auth/signup`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/google`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`

## Boundary Rules

- Outros servicos nao devem importar `auth/internal/*`.
- Outros servicos nao devem ler ou escrever o banco do auth diretamente.
- Integracoes futuras devem acontecer via API publica ou eventos.
