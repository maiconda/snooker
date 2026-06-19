# Service Boundaries

Este repositorio segue um modelo de monorepo com microservicos em pastas de raiz.

## Layout

```text
snooker/
  auth/
    cmd/auth/
    internal/
    Dockerfile
  profile/
    cmd/profile/
    internal/
    Dockerfile
  frontend/
  game-core/
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
| Profile | `profile/` | `snooker-profile` | `snooker-profile-postgres` | Foto, bio, nickname e XP do jogador |
| Frontend | `frontend/` | `snooker-frontend` | N/A | Interface web React e integracao com APIs publicas |
| Game Core | `game-core/` | `snooker-game-core` | N/A | Motor visual/fisica deterministica client-side em desenvolvimento |

## Infraestrutura local atual

| Infra | Container | Responsabilidade |
| --- | --- | --- |
| MinIO | `snooker-minio` | Armazenamento de fotos de perfil via API S3 |
| MinIO Init | `snooker-minio-init` | Criacao/configuracao do bucket `snooker-profiles` |

## Itens deliberadamente fora do runtime atual

- Redis nao e usado por nenhum servico atual e foi removido do `docker-compose.yml`.
- NATS permanece como decisao arquitetural futura para `realtime`, `lobby` e eventos de jogo, mas nao deve entrar no runtime ate existir consumidor real.

## Proximos modulos esperados

Futuros modulos como lobby, matchmaking, game, realtime e storage devem entrar como pastas irmas de `auth/`, cada uma com build e ownership de dados independentes.

## Integracoes atuais

- `profile` valida o JWT emitido pelo `auth` usando as claims publicas `sub`, `email` e `status`.
- `profile` nao acessa o banco do `auth`; ao concluir onboarding, chama `POST /api/v1/internal/users/{user_id}/activate` no `auth` usando `X-Internal-API-Key`.
- `profile` usa MinIO/S3 para fotos por URL pre-assinada. O browser envia a imagem direto ao bucket e o profile confirma o objeto antes de gravar a referencia.
