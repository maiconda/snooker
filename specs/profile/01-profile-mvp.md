# Profile MVP

## Objetivo

Implementar o perfil simples do jogador com foto, bio, nickname e XP, mantendo `auth` e `profile` como servicos separados.

## Experiencia

- Usuario autenticado com `status = onboarding_pending` deve ser redirecionado obrigatoriamente para `/perfil`.
- Usuario ativo deve conseguir acessar `/perfil` a qualquer momento pela Home.
- A foto deve ser ajustada para proporcao 1:1 no frontend antes do upload.
- O visual deve seguir tons de preto, branco e cinza, alinhado ao jogo.

## Backend

### Profile

- `GET /api/v1/profiles/me`
- `POST /api/v1/profiles/me/photo-upload-url`
- `POST /api/v1/profiles/me/complete`
- `PATCH /api/v1/profiles/me`
- `GET /api/v1/profiles/{user_id}`

### Auth

- `POST /api/v1/internal/users/{user_id}/activate`
- Protegido por `X-Internal-API-Key`.
- Retorna novo access token com `status = active`.

## Storage

- MinIO/S3 bucket: `snooker-profiles`.
- Upload direto do browser via presigned PUT.
- Backend confirma existencia, tamanho e tipo do objeto antes de salvar `photo_url`.
- SVG nao e aceito.

## Validacao

- `go test ./auth/... ./profile/...`
- `npm run typecheck`
- `npm run build`
- `docker compose config`
