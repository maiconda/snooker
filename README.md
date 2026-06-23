# Snooker Multiplayer Platform

Este repositório contém uma plataforma multiplayer de sinuca baseada em microsserviços Go (Backend) e React com Three.js/Tailwind (Frontend), orquestrada em contêineres Docker.

---

## 🚀 Como Executar o Projeto

Fornecemos configurações distintas de Docker Compose para os ambientes de **Desenvolvimento** e **Produção**.

### Requisitos Prévios

1. Crie um arquivo `.env` na raiz do projeto (use o `.env.example` como base):
   ```bash
   cp .env.example .env
   ```
2. Certifique-se de preencher as variáveis obrigatórias no seu `.env`:
   - `GOOGLE_CLIENT_ID` (obtido no Google Cloud Console para o fluxo de OAuth)
   - `JWT_SECRET` (chave de segurança do token JWT)

---

## 🛠️ 1. Ambiente de Desenvolvimento (Dev)

No ambiente de desenvolvimento, o fluxo é otimizado para HMR (Hot Module Replacement), permitindo que você altere o código do frontend e veja as mudanças imediatamente, além de expor as portas de serviços e bancos de dados para conexões de depuração do host.

### Executar em Desenvolvimento:
```bash
docker compose -f docker-compose.dev.yml up --build
```

### Características do Dev:
- **Porta do Frontend**: http://localhost:3000 (com live-reloading ativado).
- **Porta do Game Core (Protótipo)**: http://localhost:3001.
- **Portas expostas para depuração**:
  - API de Autenticação: `8081`
  - API de Perfis: `8082`
  - API de Partidas (Lobby): `8083`
  - Banco Auth (Postgres): `5432`
  - Banco Perfis/Jogo (Postgres): `5433`
  - Banco Partidas (Postgres): `5434`
  - Object Storage Console (MinIO): `9001` (login/senha: `minioadmin` / `minioadmin`)
  - Object Storage API (MinIO): `9005`
  - Fila de Mensagens (NATS): `4222`

---

## 📦 2. Ambiente de Produção (Prod)

No ambiente de produção, todo o código do frontend e do backend é compilado dentro de imagens Docker otimizadas (multistage builds). 
Nginx serve como o único ponto de entrada para o usuário, roteando de forma segura as chamadas de API internas e WebSockets sem expor portas extras ao host.

### Executar em Produção:
```bash
docker compose -f docker-compose.prod.yml up -d --build
```

### Características do Prod:
- **Porta Unificada (Frontend + APIs)**: http://localhost:3000
- **Segurança máxima**: Portas de bancos de dados, NATS, MinIO e microsserviços individuais Go são isoladas na rede Docker interna do compose, reduzindo a superfície de ataque no servidor Host.
- **Performance**: Código empacotado de forma estática servido pelo Nginx de alta performance.
- **Sem montagem de volumes locais de código**: As imagens contêm tudo o que é necessário para a execução estável.

---

## 🧹 Limpeza de Volumes (Reset)

Se precisar limpar as bases de dados e iniciar o ambiente do zero, execute:

- **Para Dev**:
  ```bash
  docker compose -f docker-compose.dev.yml down -v
  ```
- **Para Prod**:
  ```bash
  docker compose -f docker-compose.prod.yml down -v
  ```
