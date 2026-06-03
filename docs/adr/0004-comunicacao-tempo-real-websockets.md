# ADR 0004: Conexões WebSockets em Go e NATS.io como Distribuidor Stateless

- **Status:** Aceito
- **Data:** 2026-05-29
- **Autor:** Antigravity & Jogador (Co-Designers)

### 1. Contexto e Declaração do Problema

A transmissão instantânea de eventos de início de tacada, confirmações de tacada, chat e consenso de fim de movimento entre jogadores e espectadores precisa ocorrer de forma bidirecional e com latência sub-100ms.
Ao rodar múltiplos pods de Gateway de conexões sob Kubernetes para suportar picos de tráfego, os jogadores conectados em pods físicos distintos precisam trocar mensagens sem criar dependência ou afinidade de servidor físico (stateless design).

### 2. Opções Consideradas

- **Opção A (WebSockets com Redis Pub/Sub):** Uso do Redis in-memory Pub/Sub para roteamento inter-servidores.
- **Opção B (WebSockets em Go com NATS.io Pub/Sub):** Implementar o servidor de conexões persistentes em Go (usando `gorilla/websocket` ou `nhooyr/websocket`) e utilizar o broker **NATS.io** de alta performance como o barramento de mensagens e Pub/Sub distribuído para sincronização de pods.

### 3. Trade-offs das Opções

#### Opção A (WebSockets com Redis Pub/Sub)
- **Prós:** Solução muito comum no mercado com vasta documentação.
- **Contras:** Redis consome mais CPU e rede para gerenciar filas ativas que o NATS.io, além de exigir configuração complexa de Sentinel ou Cluster para alta disponibilidade de Pub/Sub.

#### Opção B (WebSockets em Go com NATS.io)
- **Prós:** O Go gerencia conexões WebSocket concorrentes com goroutines de forma extremamente leve, permitindo centenas de milhares de conexões ativas por pod com pouco consumo de RAM. O NATS.io possui desempenho de Pub/Sub extraordinariamente rápido, latência inferior à do Redis e suporte simplificado de clustering nativo no Kubernetes para tolerância a falhas.
- **Contras:** Requer a inclusão e gerência da biblioteca cliente NATS Go no código.

### 4. Decisão Escolhida

> **Opção Escolhida:** Opção B (WebSockets em Go com NATS.io Pub/Sub)
>
> **Justificativa:** O emparelhamento de Go (serviço WebSockets stateless de alta vazão e baixo consumo) com NATS.io (barramento Pub/Sub de altíssima performance escrito em Go) é a arquitetura ideal. Ela garante escalabilidade horizontal ilimitada no Kubernetes: se um pod de conexão cair, o cliente simplesmente reconecta a outro pod e continua a partida sem perda de dados, pois o NATS.io gerencia a distribuição das mensagens de forma centralizada.

### 5. Consequências e Impactos

- **Impacto Positivo:** Arquitetura 100% resiliente a falhas e stateless. Facilidade de balanceamento de carga de conexões persistentes no Ingress Controller do Kubernetes.
- **Impacto Negativo/Riscos:** Volume alto de pacotes de dados fluindo na rede interna entre os microsserviços em Go e o cluster NATS.
- **Ações de Mitigação:** Compactar mensagens usando formatos leves binários ou JSON minimizado e utilizar a biblioteca nativa altamente otimizada do NATS para Go.
