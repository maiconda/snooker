# Service Boundaries

Este repositorio segue um modelo de monorepo com microservicos em pastas de raiz.

## Layout

```text
snooker/
  auth/
    cmd/auth/
    internal/
    Dockerfile
  docker-compose.yml
  docs/
  specs/
```

## Regras

1. Cada servico nasce em uma pasta de raiz propria.
2. Cada servico tem seu proprio entrypoint em `cmd/<service>`.
3. Cada servico mantem codigo privado em `<service>/internal`.
4. Cada servico tem Dockerfile proprio.
5. Cada servico possui seu banco ou schema operacional proprio.
6. Um servico nao importa `internal` de outro servico.
7. Integracoes entre servicos devem acontecer por API publica ou eventos.

## Servico atual

| Servico | Pasta | Container | Banco | Responsabilidade |
| --- | --- | --- | --- | --- |
| Auth | `auth/` | `snooker-auth` | `snooker-auth-postgres` | Identidade, credenciais, JWT e refresh tokens |
| Frontend | `frontend/` | `snooker-frontend` | N/A | Interface web React e integracao com APIs publicas |

## Proximos modulos esperados

Futuros modulos como profile, lobby, matchmaking, game, realtime e storage devem entrar como pastas irmas de `auth/`, cada uma com build e ownership de dados independentes.
