# ADR 0002: Motor de Física Determinista Client-side e Validação por Consenso

- **Status:** Aceito
- **Data:** 2026-05-29
- **Autor:** Antigravity & Jogador (Co-Designers)

### 1. Contexto e Declaração do Problema

O motor de física calcula o movimento e colisões das bolas de sinuca.
- **Transmissão Contínua:** Transmitir coordenadas XY de todas as bolas a 60fps do servidor para os clientes consome gigabytes de tráfego de rede e gera jitter visual sob conexões instáveis.
- **Custos do Servidor:** Executar cálculos físicos vetoriais tridimensionais ou bidimensionais contínuos para milhares de salas ativas simultaneamente exige um cluster pesado e caro de CPU.

### 2. Opções Consideradas

- **Opção A (Física no Servidor):** Executar toda a física dos vetores em containers dedicados e transmitir as coordenadas geradas para os clientes renderizarem.
- **Opção B (Física Determinista no Cliente + Consenso):** Cada cliente executa a simulação física de forma local e idêntica com base em parâmetros iniciais da tacada. Ao final do movimento, ambos os clientes reportam o resultado à Core API. O servidor valida por consenso (compara se os dados reportados são idênticos) e processa a pontuação.

### 3. Trade-offs das Opções

#### Opção A (Física no Servidor)
- **Prós:** Segurança absoluta, pois o cliente é apenas um receptor passivo de coordenadas e não pode alterar dados locais.
- **Contras:** Custo absurdo de infraestrutura de rede e CPU. Experiência de jogo sensível a picos de latência de rede.

#### Opção B (Física Determinista no Cliente + Consenso)
- **Prós:** Custo computacional zero de física no servidor. Experiência de jogo extremamente fluida a 60fps localmente, livre de lag de rede durante a animação do movimento. Consumo de banda de rede insignificante (apenas parâmetros de início e consenso de fim).
- **Contras:** Exige que a física seja 100% determinística (mesma tacada inicial deve gerar precisamente as mesmas paradas em todas as máquinas de destino).
- **Segurança contra Trapaças:** Resolvida pelo mecanismo de **Consenso**. Se um dos jogadores trapacear localmente modificando as paradas, o reporte final divergirá do relatório do oponente honesto, resultando na suspensão automática da partida para auditoria de fraude.

### 4. Decisão Escolhida

> **Opção Escolhida:** Opção B (Física Determinista no Cliente + Consenso)
>
> **Justificativa:** Esta decisão otimiza de forma brilhante a infraestrutura de servidores do Snooker Multiplayer. Ela permite rodar milhares de partidas simultâneas em servidores leves, sem gargalos. O mecanismo de comparação de consensos protege os dados do jogo contra cheats de forma inteligente e descentralizada.

### 5. Consequências e Impactos

- **Impacto Positivo:** Latência nula na simulação das tacadas para os jogadores e custo operacional baixíssimo para a plataforma.
- **Impacto Negativo/Riscos:** Se houver variação sutil nas bibliotecas matemáticas de float/vetores entre plataformas dos jogadores (ex: Web vs Mobile), pode haver divergências de física genuínas.
- **Ações de Mitigação:** Utilizar aritmética de ponto fixo (Fixed-Point Math) ou bibliotecas de física deterministas estritas para garantir cálculos exatamente idênticos em qualquer sistema operacional ou arquitetura de processador.
