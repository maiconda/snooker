# Profile Service

Servico isolado de perfil publico do Snooker.

## Ownership

- Codigo: `profile/`
- Processo: container `snooker-profile`
- Banco: container `snooker-profile-postgres`, database `snooker_game`
- Tabelas: `profiles`, `photo_upload_sessions`
- Storage: bucket MinIO `snooker-profiles`

## API

- `GET /health/live`
- `GET /health/ready`
- `GET /api/v1/profiles/me`
- `POST /api/v1/profiles/me/complete`
- `PATCH /api/v1/profiles/me`
- `POST /api/v1/profiles/me/photo-upload-url`
- `GET /api/v1/profiles/:user_id`

## Boundary Rules

- O servico valida JWT pelo contrato publico das claims (`sub`, `email`, `status`).
- O servico nao importa `auth/internal/*`.
- O servico nao escreve no banco do auth.
- A conclusao do onboarding chama a API interna do auth para ativar a conta.
