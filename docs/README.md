# Documentação de Arquitetura da Plataforma de Sinuca Multiplayer

Este diretório contém a documentação técnica e o registro de decisões de engenharia adotadas no desenvolvimento da plataforma de sinuca multiplayer escalável.

## 📌 Escopo do Projeto
Para entender o escopo geral, requisitos de negócio e a topologia básica planejada, leia o arquivo principal de escopo:
- [📄 escopo.md (Raiz)](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/escopo.md)

---

## 🔑 Módulos Detalhados
Para especificações arquiteturais completas e contratos de API de módulos específicos:
- [📄 Arquitetura Completa de Autenticação e Onboarding](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/auth-architecture.md)
- [📄 Política de Access & Refresh Tokens](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/auth-tokens-policy.md)
- [📄 Auditoria de Segurança e Validação K8s](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/security-and-k8s-audit.md)

---

## 🛠️ Especificações de Desenvolvimento (Code Specs)
Arquivos de guia detalhados para a equipe de engenharia implementar o módulo de Autenticação com foco estrito em testes:
- [📂 Pasta de Especificações de Autenticação](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/specs/auth/)
  - [📄 01. Banco de Dados e Modelos Go](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/specs/auth/01-database-and-models.md)
  - [📄 02. Contratos e Endpoints da API REST](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/specs/auth/02-api-endpoints.md)
  - [📄 03. Controle de Sessão, JWT e Rotação (RTR)](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/specs/auth/03-token-and-session.md)
  - [📄 04. Integração de Object Storage (MinIO/S3)](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/specs/auth/04-storage-integration.md)
  - [📄 05. Plano de Testes e Critérios de Conclusão (DoD)](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/specs/auth/05-testing-and-validation.md)

---

## 🏛️ Architecture Decision Records (ADRs)

Adotamos a prática de registrar todas as decisões técnicas significativas utilizando o modelo padrão de **ADRs**. Isto serve como registro histórico de trade-offs de engenharia e facilita a integração de novos engenheiros e agentes.

Abaixo está o índice das decisões arquiteturais tomadas:

1. **[ADR 0001: Arquitetura de Microsserviços baseada em Go (Golang) e NATS.io](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/adr/0001-arquitetura-microsservicos.md)**
   - *Resumo:* Decomposição do sistema em microsserviços autônomos de alta concorrência em Go (Core API e Gateway WebSocket) utilizando o broker cloud-native NATS.io para mensageria inter-servidores de baixíssima latência.
2. **[ADR 0002: Motor de Física Determinista Client-side e Validação por Consenso](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/adr/0002-motor-fisica-event-driven.md)**
   - *Resumo:* Execução local determinística da simulação de física das tacadas a 60fps pelos clientes, com validação de resultado de final de jogada enviada ao servidor por mecanismo de consenso duplo, eliminando tráfego desnecessário e custos computacionais de física no cluster.
3. **[ADR 0003: Persistência Transacional com PostgreSQL e Gerenciamento de Salas em Memória](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/adr/0003-armazenamento-hibrido.md)**
   - *Resumo:* Utilização do banco de dados relacional robusto PostgreSQL para persistência transacional de contas, histórico e progresso XP de longo prazo, mantendo o estado volátil das salas diretamente na memória do cluster de Go coordenado por NATS.io.
4. **[ADR 0004: Conexões WebSockets em Go e NATS.io como Distribuidor Stateless](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/adr/0004-comunicacao-tempo-real-websockets.md)**
   - *Resumo:* Utilização de WebSockets persistentes em Go (com goroutines levíssimas) integrados ao Pub/Sub do NATS.io para permitir um Gateway stateless de alta performance e resiliência linear.
5. **[ADR 0005: Modelo Híbrido de Lobby (Salas Pré-Criadas e Salas Particulares por Código)](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/adr/0005-matchmaking-lobby.md)**
   - *Resumo:* Implementação de salas públicas pré-configuradas em painel geral para entrada direta, combinadas com criação dinâmica de salas privadas acessadas de forma exclusiva por códigos pseudo-aleatórios de convite de 6 caracteres.
6. **[ADR 0006: Fluxo de Autenticação e Onboarding de Usuário (OAuth, Presigned URLs e JWT)](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/adr/0006-fluxo-autenticacao-onboarding.md)**
   - *Resumo:* Detalhamento do fluxo seguro e de alta performance para criação de contas (Google OAuth via JWKS local ou Email/Senha), upload de imagem de perfil direto para MinIO/S3 via Presigned URLs e gerenciamento de sessão stateless com JWT.
7. **[ADR 0007: Isolamento do Auth Service na raiz](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/adr/0007-auth-service-root-isolation.md)**
   - *Resumo:* Formaliza o auth como servico de raiz independente, com entrypoint, Dockerfile, container e ownership de dados proprios.
8. **[ADR 0008: Gameplay state, deterministic physics, and spectator playback](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/adr/0008-gameplay-state-physics-spectators.md)**
   - *Resumo:* Define o servidor como autoridade de eventos e snapshots, mantendo fisica deterministica client-side, commit por consenso, replay server-side sob demanda e playback fluido para espectadores.

## Gameplay

- [Gameplay state and physics architecture](file:///c:/Users/maico/OneDrive/Área%20de%20Trabalho/snooker/docs/gameplay-state-and-physics.md)
