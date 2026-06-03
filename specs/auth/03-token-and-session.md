# Especificação Técnica: Controle de Sessão, JWT e Rotação de Tokens (RTR)

Esta especificação define o comportamento lógico detalhado da emissão de tokens JWT, armazenamento e validação de Refresh Tokens, o mecanismo de Rotação de Refresh Tokens (RTR) com detecção de fraude e a tolerância a concorrências (Grace Period).

---

## 1. Emissão de JWT (Access Tokens)

O microsserviço Core API deve utilizar o algoritmo **HS256** (HMAC com SHA-256) ou **RS256** (chaves assimétricas RSA) para assinar as claims. As seguintes claims são obrigatórias em qualquer token emitido:

```json
{
  "sub": "user_uuid_here",
  "email": "user@example.com",
  "status": "onboarding_pending | active",
  "iat": 1780185600,
  "exp": 1780186500
}
```

*   **Tempo de vida (Access Token):** Estrito a **15 minutos**.
*   **Chave Secreta compartilhada:** Carregada exclusivamente através da variável de ambiente `JWT_SECRET` gerenciada por K8s Secrets.

---

## 2. Emissão de Cookies de Refresh Token

O Refresh Token gerado é um hash opaco de alta entropia. O servidor deve entregá-lo ao cliente através do cabeçalho de resposta HTTP `Set-Cookie` com os seguintes parâmetros rígidos de segurança:

```http
Set-Cookie: refresh_token=<OPAQUE_STRING_HERE>; Path=/api/v1/auth; HttpOnly; Secure; SameSite=Strict; Max-Age=604800
```

*   **HttpOnly:** Deve ser definido como true. Impede leitura por scripts (XSS).
*   **Secure:** Deve ser definido como true. Só trafega sob HTTPS (exceto em localhost em modo desenvolvimento).
*   **SameSite:** Definido como `Strict` (ou `Lax` dependendo dos testes de Ingress CORS).
*   **Path:** Limitado ao caminho de autenticação (`/api/v1/auth`) para evitar o envio do cookie em requisições de outras rotas da API, economizando banda.

---

## 3. Algoritmo de Rotação e Detecção de Reuso com Grace Period

Para mitigar problemas de latência e concorrência no cliente (múltiplas requisições de refresh simultâneas), a rota `/refresh` deve seguir exatamente este algoritmo de validação e rotação:

```text
[POST /api/v1/auth/refresh]
       │
       ▼
Recebe 'refresh_token' do Cookie
       │
       ▼
Calcula hash SHA-256 e busca na tabela 'refresh_tokens'
       │
       ├─► [NÃO ENCONTRADO / EXPIRADO]
       │     └─► Retorna 401 Unauthorized
       │
       └─► [ENCONTRADO]
             │
             ├─► É um Token ATIVO (revoked = false E expiração > atual)?
             │     │
             │     ├─► [SIM - FLUXO NORMAL]
             │     │     1. Gera novo RT (RT_NEW) com mesma family_id
             │     │     2. Insere RT_NEW no banco
             │     │     3. Atualiza RT anterior: revoked = true, revoked_at = NOW()
             │     │     4. Retorna RT_NEW no Cookie e novo Access Token no JSON (200 OK)
             │     │
             │     └─► [NÃO]
             │           │
             │           └─► Foi revogado dentro do Grace Period (revoked_at >= NOW - 15 segundos)?
             │                 │
             │                 ├─► [SIM - CONCORRÊNCIA DO CLIENTE]
             │                 │     1. Identifica que a rotação já ocorreu recentemente.
             │                 │     2. Busca o token ativo da mesma family_id.
             │                 │     3. Retorna esse token ativo no Cookie e o Access Token correspondente.
             │                 │
             │                 └─► [NÃO - TENTATIVA DE FRAUDE / REUSO]
             │                       1. Alerta de Segurança disparado.
             │                       2. Marca TODOS os tokens da mesma 'family_id' como revoked = true.
             │                       3. Retorna 401 Unauthorized (obriga login completo).
```

---

## 4. Critérios de Aceitação dos Tokens e RTR

Para que esta camada lógica de controle de sessão seja considerada implementada:
1.  **Testes de Expiração:** Tentar usar um Access Token cuja claim `exp` está no passado deve ser rejeitado imediatamente pelo middleware de segurança.
2.  **Testes de Rotação Segura (RTR):** O teste automatizado deve realizar um refresh de sessão válido, guardar o token antigo e tentar usá-lo após 20 segundos. A resposta deve ser `401 Unauthorized` e todos os tokens da mesma família devem estar inativados no banco.
3.  **Testes de Concorrência (Grace Period):** Simular 3 chamadas concorrentes ao endpoint de `/refresh` usando o mesmo token. O servidor deve responder com sucesso em todas elas, retornando o par atualizado sem disparar o alarme de segurança.
4.  **Criptografia do Hash:** Os tokens não devem ser armazenados em texto limpo na tabela `refresh_tokens`. O banco de dados deve salvar apenas o hash `SHA-256` gerado pelo Go (`sha256.Sum256`).
