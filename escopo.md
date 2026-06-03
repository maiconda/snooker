# Documento de Escopo: Plataforma de Sinuca Multiplayer Escalonável

Este documento detalha o escopo, as funcionalidades e a arquitetura de engenharia para o desenvolvimento de uma plataforma de sinuca online multiplayer. O projeto é desenhado com foco estrito em alta disponibilidade e escalabilidade horizontal, utilizando **arquitetura de microsserviços** e orquestração em **Kubernetes** para suportar picos de tráfego, cálculos complexos de física e múltiplas conexões simultâneas.

---

## 1. Funcionalidades Principais do Sistema

A plataforma será dividida em módulos funcionais que interagem entre si de forma assíncrona por meio de microsserviços isolados.

* **Gestão de Contas e Progressão:** Criação de perfis de usuário com autenticação segura. Cada partida vencida concederá pontos de experiência (XP), permitindo o avanço de nível e a criação de um ranking global.
* **Gestão de Salas e Matchmaking:** Os jogadores poderão criar salas privadas (protegidas por senha ou link de convite direto) ou entrar em salas públicas abertas. O sistema alocará os jogadores dinamicamente.
* **Modo Espectador e Chat em Tempo Real:** Salas públicas permitirão a entrada de múltiplos espectadores passivos. Um sistema de mensageria (chat) será integrado à sala, permitindo a comunicação entre jogadores e espectadores com baixa latência.
* **Motor de Física Server-side:** Para evitar trapaças e garantir a sincronia, todo o cálculo de colisão das bolas será feito em servidores isolados e transmitido como posições de renderização para os clientes.

---

## 2. Arquitetura de Microsserviços

O projeto adotará uma arquitetura distribuída e orquestrada via Kubernetes para lidar com a separação de responsabilidades (I/O intensivo vs. CPU intensivo):

* **Frontend (Interface do Usuário):** Aplicações cliente (web ou mobile) reativas para renderização fluida, consumindo a API e mantendo conexões persistentes em tempo real.
* **Microsserviço de Core API (Backend Transacional):** Responsável pela persistência de dados críticos, como criação de contas, gestão de amizades, histórico de partidas e atualização de XP no banco de dados.
* **Microsserviço Gateway de Conexões em Tempo Real:** Um serviço leve focado exclusivamente em manter milhares de conexões abertas simultaneamente, distribuindo mensagens de chat e atualizações de estado do jogo.
* **Microsserviços Workers de Física (Event-Driven):** Contêineres dedicados exclusivamente ao processamento da lógica vetorial. Consomem eventos de uma fila de mensagens de forma concorrente para calcular as tacadas.

---

## 3. Estratégia de Escalabilidade no Kubernetes

A topologia do cluster Kubernetes será configurada para reagir a diferentes tipos de estresse, otimizando os recursos computacionais disponíveis de forma autônoma.

| Microsserviço | Métrica de Gatilho (Autoscaling) | Comportamento de Escalabilidade |
| :--- | :--- | :--- |
| **Core API (Contas/XP)** | Uso de CPU e Requisições HTTP | Escala via Horizontal Pod Autoscaler (HPA) tradicional. Gerencia os picos de login e consultas de XP em horários de pico. |
| **Gateway de Tempo Real** | Consumo de Memória (RAM) e Conexões Simultâneas | Se uma sala pública atrai milhares de espectadores, o cluster replica os pods horizontalmente para distribuir o *broadcast* sem derrubar a rede. |
| **Workers de Física** | Volume da Fila de Mensagens (Event-Driven) | Permanece em repouso. Quando muitos jogadores dão tacadas simultaneamente, o K8s cria réplicas instantâneas para calcular os vetores em paralelo e volta a zero (*Scale-to-Zero*) ao fim dos movimentos. |

---

## 4. Estrutura de Dados e Persistência

Para garantir a performance, os microsserviços farão uso de armazenamento híbrido, separando dados em repouso de dados efêmeros de alto acesso, sem acoplamento a uma tecnologia específica:

* **Banco de Dados Relacional:** Para dados estruturados e persistentes. Garantirá a integridade transacional de contas, amizades e histórico de partidas.
* **Banco de Dados em Memória (Cache):** Crucial para o funcionamento em tempo real. Armazenará o estado efêmero das salas e as coordenadas em movimento das bolas (estado volátil repassado ao frontend).
* **Message Broker (Mensageria):** Responsável por intermediar a comunicação assíncrona pesada, recebendo os eventos das tacadas e entregando aos Workers de física de forma ordenada.
