# Especificação Técnica: Contratos e Endpoints da API REST (Auth)

Este documento especifica os caminhos de roteamento, payloads de entrada e saída, códigos de status HTTP e padrões de resposta de erro para a camada de Autenticação e Onboarding na Core API.

---

## 1. Padronização de Respostas de Erro

Para que o cliente frontend possa processar erros de validação e segurança de forma robusta, todas as respostas com status HTTP diferente de `2xx` devem seguir esta estrutura JSON:

```json
{
  "error": {
    "code": "BAD_REQUEST",
    "message": "Mensagem detalhada para o programador/usuário",
    "details": [
      {
        "field": "display_name",
        "issue": "O campo deve conter no mínimo 3 caracteres."
      }
    ]
  }
}
```

### Tabela de Códigos de Erros de Negócio:
*   `INVALID_CREDENTIALS`: Login inválido (email ou senha incorretos).
*   `EMAIL_ALREADY_EXISTS`: Tentativa de criar conta com email já cadastrado.
*   `ONBOARDING_PENDING`: Acesso a rotas restritas antes da conclusão do preenchimento de perfil.
*   `VALIDATION_FAILED`: Um ou mais campos de validação falharam no payload.
*   `TOKEN_EXPIRED`: O Access Token fornecido expirou.
*   `UNAUTHORIZED`: Cabeçalho `Authorization` ausente ou malformatado.

---

## 2. Especificação Detalhada dos Endpoints

### 2.1. Criar Conta Local (Signup)
*   **Path:** `POST /api/v1/auth/signup`
*   **Request Headers:** `Content-Type: application/json`
*   **Request JSON Payload:**
    ```json
    {
      "email": "username@domain.com",
      "password": "SecurePassword123!"
    }
    ```
*   **Regras de Entrada:**
    *   `email`: string, formato RFC 5322 obrigatório.
    *   `password`: string, min 8, max 72, mínimo 1 maiúscula, 1 minúscula, 1 número e 1 caractere especial.
*   **Response (201 Created):**
    ```json
    {
      "message": "Conta criada com sucesso",
      "token": "eyJhbGciOi...",
      "status": "onboarding_pending"
    }
    ```
    *   *Nota:* Deve também emitir o cookie `refresh_token` no cabeçalho HTTP `Set-Cookie`.

### 2.2. Login Local
*   **Path:** `POST /api/v1/auth/login`
*   **Request JSON Payload:**
    ```json
    {
      "email": "username@domain.com",
      "password": "SecurePassword123!"
    }
    ```
*   **Response (200 OK):**
    ```json
    {
      "token": "eyJhbGciOi...",
      "status": "active"
    }
    ```

### 2.3. Google OAuth Sign-In
*   **Path:** `POST /api/v1/auth/google`
*   **Request JSON Payload:**
    ```json
    {
      "id_token": "eyJhbGciOiJSUzI1Ni..."
    }
    ```
*   **Fluxo interno:** O servidor Go decodifica o JWT localmente, valida a assinatura com as JWKS públicas obtidas dos servidores da Google, lê as claims (`email`, `sub`), cria a conta se não existir (status: `onboarding_pending`) ou autentica se já existir.

### 2.4. Solicitar Link de Upload (Presigned URL)
*   **Path:** `GET /api/v1/profile/upload-url`
*   **Request Headers:** `Authorization: Bearer <Access Token>` (Pode ter status `onboarding_pending` ou `active`)
*   **Response (200 OK):**
    ```json
    {
      "upload_url": "https://storage.snooker.local/profile-pics/temp/c39d8923-a5ff-4bc1-9c88.png?Signature=...",
      "object_key": "c39d8923-a5ff-4bc1-9c88.png"
    }
    ```

### 2.5. Concluir Perfil (Onboarding Final)
*   **Path:** `POST /api/v1/profile/complete`
*   **Request Headers:** `Authorization: Bearer <Access Token>` (Estrito com status `onboarding_pending`)
*   **Request JSON Payload:**
    ```json
    {
      "display_name": "PlayerOne",
      "bio": "Mestre da sinuca.",
      "photo_key": "c39d8923-a5ff-4bc1-9c88.png"
    }
    ```
*   **Response (200 OK):**
    ```json
    {
      "message": "Perfil configurado com sucesso. Conta ativa.",
      "token": "eyJhbGciOi..." -- Novo JWT ativo
    }
    ```

### 2.6. Logout (Revogar Sessão)
*   **Path:** `POST /api/v1/auth/logout`
*   **Request Headers:** `Authorization: Bearer <Access Token>`
*   **Response (200 OK):**
    ```json
    {
      "message": "Sessão encerrada com sucesso"
    }
    ```
    *   *Headers:* Deve enviar `Set-Cookie` com Max-Age=0 para expirar o cookie de refresh.

---

## 3. Critérios de Aceitação dos Endpoints

A implementação das rotas deve possuir testes de integração de API (HTTP Mocks/Requests) que validem:
1.  **Proteção de Rota:** Tentar acessar `/upload-url` ou `/complete` sem token ou com token inválido deve retornar `401 Unauthorized`.
2.  **Transição de Escopo:** Tentar acessar rotas de jogo normais (ex: `/lobbies`) com o token temporário (`onboarding_pending`) deve retornar `403 Forbidden`.
3.  **CORS e Preflight:** Chamadas com método `OPTIONS` devem retornar os cabeçalhos de controle de acesso permitindo credenciais (`Access-Control-Allow-Credentials: true`).
