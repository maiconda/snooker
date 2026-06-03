# Relatório de Auditoria de Segurança e Validação de Arquitetura no Kubernetes

Este documento apresenta uma revisão crítica, ameaças mapeadas e validação técnica da arquitetura de Autenticação e Onboarding para a plataforma Snooker Multiplayer. O foco está em identificar pontos únicos de falha (SPOF), condições de corrida em concorrência, brechas de segurança e particularidades de implantação em um cluster Kubernetes (K8s).

---

## 1. Concorrência e Condições de Corrida (Race Conditions)

### 📌 Ponto de Falha 1.1: A Corrida de Requisições do Refresh Token (RTR Lockout)
*   **Problema:** Em aplicações modernas (Single Page Applications ou Mobile), quando o Access Token expira, múltiplos componentes da interface do usuário podem tentar disparar requisições HTTP paralelas de forma simultânea. Isso gera chamadas concorrentes ao endpoint `POST /api/v1/auth/refresh`.
    *   A primeira requisição chega ao Servidor A, valida o Refresh Token ativo ($RT_1$), grava o novo par ($RT_2$) e revoga o $RT_1$.
    *   Milisegundos depois, a segunda requisição concorrente (enviada antes do cliente receber o novo cookie) chega ao Servidor B contendo o mesmo $RT_1$.
    *   O Servidor B consulta o banco de dados, constata que $RT_1$ já está revogado, assume que houve um ataque de roubo de sessão (Detecção de Reuso) e bloqueia toda a família de tokens, forçando o logout imediato e legítimo do usuário.
*   **Mitigação Recomendada:** Implementar um **Período de Tolerância (Grace Period)** no banco de dados.
    *   Adicionar o campo `revoked_at TIMESTAMP WITH TIME ZONE` na tabela `refresh_tokens`.
    *   Quando um token for rotacionado, marcamos `revoked = true` e salvamos o timestamp em `revoked_at`.
    *   Se o mesmo token for enviado novamente dentro de uma janela de tempo de tolerância muito curta (ex: **15 a 30 segundos**), o servidor não dispara o alarme de reuso. Em vez disso, ele simplesmente retorna o mesmo novo par de tokens ($RT_2$) gerado no primeiro refresh.

---

## 2. Particularidades e Pontos de Falha no Kubernetes (K8s)

### 📌 Ponto de Falha 2.1: Assinatura de JWT Descentralizada (Split-Brain)
*   **Problema:** Se os pods do microsserviço da Core API gerarem a chave privada de assinatura do JWT dinamicamente em memória RAM durante a inicialização, o Pod A não conseguirá validar os tokens emitidos pelo Pod B. Isso resultará em falhas aleatórias de autenticação (`401 Unauthorized`) à medida que o Ingress Controller balancear as requisições HTTP entre as réplicas.
*   **Mitigação Recomendada:** Centralização de segredos.
    *   As chaves de assinatura do JWT (chave secreta HMAC ou par de chaves RSA) devem ser configuradas externamente e injetadas de forma idêntica em todos os pods através de um recurso **Kubernetes Secret** montado como variável de ambiente ou volume de arquivo.

### 📌 Ponto de Falha 2.2: Saturação de Conexões no PostgreSQL sob Autoscaling (HPA)
*   **Problema:** O Kubernetes escala horizontalmente (HPA) os pods da Core API Go com base no uso de CPU/Requisições. Em picos de tráfego, o cluster pode subir de 2 para 20 réplicas rapidamente. Se cada pod Go estiver configurado com um Connection Pool (`pgxpool`) com máximo de 50 conexões, os pods tentarão abrir até $20 \times 50 = 1000$ conexões simultâneas ao PostgreSQL. Isso excederá o limite máximo de conexões configurado no PostgreSQL (`max_connections`, tipicamente 100 a 200 por padrão), derrubando a persistência e gerando indisponibilidade de login.
*   **Mitigação Recomendada:**
    *   Ajustar a fórmula de conexões: $\text{Conexões Máximas do Pool do Go} \times \text{Máximo de Pods Permitidos pelo HPA} < \text{max\_connections do PostgreSQL}$.
    *   Recomenda-se configurar pools de tamanho pequeno nos pods Go (ex: `MaxConns = 10` ou `15`), pois o Go processa milhares de queries de forma assíncrona extremamente rápida com poucas conexões persistentes.
    *   Para escalas massivas, introduzir um proxy de pool de conexão intermediário (ex: **PgBouncer**) como contêiner sidecar ou deployment isolado para multiplexar as conexões físicas do PostgreSQL de forma segura.

### 📌 Ponto de Falha 2.3: Perda de Cookies devido a Domínios Cruzados e Regras de Ingress
*   **Problema:** O uso do atributo de cookie `SameSite=Strict` é excelente para segurança, mas se o frontend e a API estiverem rodando em domínios principais diferentes (ex: Frontend em `www.snooker.com` e Core API em `api.snooker-backend.com`), o navegador do cliente **bloqueará** o envio do cookie `refresh_token` nas requisições HTTP de origem cruzada. Além disso, se o Ingress Controller do K8s não estiver configurado para permitir `Credentials` no CORS, o envio de cookies falhará no handshake.
*   **Mitigação Recomendada:**
    *   **Subdomínios Unificados:** Frontend e API devem compartilhar o mesmo domínio principal (ex: `play.snooker.com` para o app e `play.snooker.com/api` ou `api.play.snooker.com` para os microsserviços).
    *   Se usar subdomínios diferentes (ex: `api.snooker.com`), configurar o cookie para usar o escopo do domínio pai: `Domain=snooker.com` e mudar o SameSite para `SameSite=Lax`.
    *   Garantir a configuração correta do CORS no Ingress para expor e aceitar cabeçalhos de autenticação:
        ```yaml
        nginx.ingress.kubernetes.io/cors-allow-credentials: "true"
        nginx.ingress.kubernetes.io/cors-allow-headers: "DNT,X-CustomHeader,Keep-Alive,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Authorization"
        ```

---

## 3. Gestão e Limpeza de Armazenamento de Objetos (Storage Leak)

### 📌 Ponto de Falha 3.1: Vazamento de Imagens Órfãs (Storage Bloat)
*   **Problema:** Um usuário mal-intencionado ou um bot pode bombardear a rota `/profile/upload-url` solicitando milhares de URLs pré-assinadas e realizando o upload dos arquivos para o MinIO/S3, mas **nunca** concluindo a chamada final de perfil `/profile/complete`. Isso gerará gigabytes de imagens órfãs armazenadas que consomem espaço em disco e nunca são associadas a nenhum usuário real.
*   **Mitigação Recomendada:** Estrutura de Buckets com Prefixo Temporário e Ciclo de Vida (S3 Lifecycle Rules).
    *   O backend Go gera a Presigned URL com o prefixo de armazenamento temporário: `profiles/temp/<uuid>.png`.
    *   Quando a chamada `/profile/complete` é executada com sucesso, o backend realiza uma operação interna rápida na SDK do S3 de **cópia/movimentação** do objeto da pasta temporária para a pasta ativa definitiva: de `profiles/temp/<uuid>.png` para `profiles/active/<user_id>.png`.
    *   Configura-se uma **Regra de Ciclo de Vida (Lifecycle Rule)** diretamente no MinIO/S3 para **deletar automaticamente** qualquer objeto dentro do prefixo `profiles/temp/` que tenha mais de 24 horas de criação.

---

## 4. Dependências Externas e Alta Disponibilidade

### 📌 Ponto de Falha 4.1: Indisponibilidade do Provedor OAuth (Google JWKS Down)
*   **Problema:** Para validar o `id_token` do Google localmente, a Core API precisa das chaves públicas expostas em endpoints do Google. Se o serviço Go tentar buscar essas chaves na API da Google síncronamente a cada requisição de login e o Google falhar ou houver uma oscilação na saída de internet do nosso cluster K8s, a autenticação de todos os usuários falhará.
*   **Mitigação Recomendada:**
    *   Implementar um cache persistente ou em memória RAM no Go para os certificados JWKS do Google (gerenciado de forma assíncrona por uma goroutine em background).
    *   Configurar o cache com expiração confortável (ex: 24 horas) e tolerância de uso de chaves expiradas em caso de queda de rede com os servidores do Google.
    *   Atualizar o cache assincronamente apenas quando o token recebido contiver uma assinatura (`kid`) desconhecida pelo backend (indicando rotação de chaves legítima pelo Google).

---

## 5. Resumo da Tabela de Ameaças (Threat Modeling)

| Vetor de Ameaça | Impacto no Sistema | Probabilidade | Severidade | Solução Arquitetural Aplicada |
| :--- | :--- | :--- | :--- | :--- |
| **Ataque XSS (Cross-Site Scripting)** | Roubo do token de acesso e sequestro permanente de sessão. | Média | Crítica | **Refresh Token em Cookie HttpOnly** (invisível ao JS) + Access Token mantido em memória volátil. |
| **Ataque CSRF (Cross-Site Request Forgery)** | Ações disparadas em nome do usuário através de cookies salvos. | Alta | Alta | Atributo do Cookie definido como **SameSite=Strict** ou **Lax** com domínio unificado + cabeçalhos CORS explícitos. |
| **Falso Positivo de Reuso de Token** | Jogadores legítimos deslogados constantemente em conexões lentas. | Alta | Média | **Grace Period (Janela de Tolerância de 15s)** para Refresh Tokens recém-revogados. |
| **Conexões do Banco Exauridas** | Queda total de todos os microsserviços de autenticação e jogo. | Alta | Crítica | Pool de conexões em Go dimensionado conforme o HPA (`MaxConns`) + sidecar **PgBouncer** para alta escala. |
| **Vazamento de Disco no MinIO** | Esgotamento de armazenamento por uploads falsos/órfãos. | Média | Alta | **S3 Lifecycle Rules** deletando a pasta `profiles/temp/` a cada 24 horas. |
