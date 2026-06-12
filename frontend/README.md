# Frontend

Modulo web isolado do Snooker, construido com React, Vite e Tailwind CSS.

## Rotas iniciais

- `/` pagina inicial vazia.
- `/login` formulario de login local e login com Google.
- `/cadastro` formulario de cadastro local.

## Auth integration

O frontend conversa com o Auth Service apenas pela API publica:

- `POST /api/v1/auth/signup`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/google`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`

O access token fica em memoria. O bootstrap da sessao usa o refresh cookie HttpOnly emitido pelo Auth Service.

## Env

Em desenvolvimento local, o Vite le o `.env` da raiz e usa:

- `GOOGLE_CLIENT_ID`
- `AUTH_API_ORIGIN`, opcional, default `http://localhost:8081`

## Comandos

```bash
npm install
npm run dev
npm run build
```
