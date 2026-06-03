# ADR 0001: Arquitetura de Microsserviços baseada em Go (Golang) e NATS.io

- **Status:** Aceito
- **Data:** 2026-05-29
- **Autor:** Antigravity & Jogador (Co-Designers)

### 1. Contexto e Declaração do Problema

A plataforma de sinuca multiplayer requer um backend altamente eficiente para processar:
1. **Lobby, Matchmaking e Cadastro (Core API):** Cadastro, logins, criação de salas particulares e persistência de XP dos jogadores em banco de dados relacional.
2. **Distribuição em Tempo Real (Gateway WebSocket):** Manter milhares de conexões persistentes WebSockets abertas com latência mínima e rotear eventos instantaneamente.

Para evitar gargalos operacionais e alto custo de servidores em nuvem, precisamos de um ecossistema leve que lide com alta concorrência de rede e que seja facilmente escalável no Kubernetes.

### 2. Opções Consideradas

- **Opção A (TypeScript com Node.js + Redis):** Desenvolvimento rápido e ecossistema dinâmico de rede.
- **Opção B (Go / Golang com NATS.io + PostgreSQL):** Utilização de linguagem compilada de altíssima velocidade no backend (Go) com o broker de mensagens em nuvem ultra rápido NATS.io para distribuição stateless das salas.

### 3. Trade-offs das Opções

#### Opção A (TypeScript com Node.js)
- **Prós:** Facilidade de compartilhamento de tipos e lógica com o frontend web, ecossistema gigante.
- **Contras:** Maior consumo de memória RAM sob milhares de conexões WebSocket abertas concorrentes, processamento single-thread que exige múltiplas réplicas pesadas sob estresse.

#### Opção B (Go / Golang com NATS.io)
- **Prós:** Velocidade computacional bruta comparável a C/C++, concorrência nativa levíssima (goroutines que consomem apenas ~2KB por conexão), consumo de memória extremamente baixo e cold-start quase instantâneo em Kubernetes. O NATS.io oferece Pub/Sub de latência sub-milissegundo, sendo mais rápido e leve que o Redis para este padrão.
- **Contras:** Curva de aprendizado ligeiramente maior que TypeScript.

### 4. Decisão Escolhida

> **Opção Escolhida:** Opção B (Go / Golang com NATS.io + PostgreSQL)
>
> **Justificativa:** Para manter os custos operacionais de servidores baixos no Kubernetes e garantir tempo de resposta de altíssima performance para os jogadores, a stack em Go unificada na Core API e no Gateway WebSocket é perfeita. A integração nativa do Go com o NATS.io (ambos escritos em Go) garante conexões otimizadas e latência mínima na transmissão de dados das tacadas.

### 5. Consequências e Impactos

- **Impacto Positivo:** Consumo de recursos computacionais drasticamente reduzido no Kubernetes. Excelente isolamento de concorrência por goroutines.
- **Impacto Negativo/Riscos:** Exige maior rigor no gerenciamento de concorrência e tipos no Go.
- **Ações de Mitigação:** Utilização de frameworks leves como Gin ou Fiber para as rotas HTTP e a biblioteca oficial do NATS.io para Go.
