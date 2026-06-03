# ADR 0003: Persistência Transacional com PostgreSQL e Gerenciamento de Salas em Memória

- **Status:** Aceito
- **Data:** 2026-05-29
- **Autor:** Antigravity & Jogador (Co-Designers)

### 1. Contexto e Declaração do Problema

O jogo requer armazenamento confiável de longo prazo de dados estruturados críticos (dados cadastrais, amizades, histórico de vitórias, pontuação e rankings globais).
Ao mesmo tempo, as salas ativas de jogo e seus estados temporários de lobby exigem manipulação extremamente rápida em memória durante a duração da partida, não necessitando de persistência em disco rígido a cada alteração.

### 2. Opções Consideradas

- **Opção A (Bancos NoSQL Flexíveis):** Utilização de bancos de dados NoSQL (como MongoDB) para todos os dados, buscando flexibilidade de esquema.
- **Opção B (PostgreSQL Relacional + Controle de Estado no Backend):**
  - **Persistência de Longo Prazo:** Banco de dados relacional **PostgreSQL** para dados transacionais críticos que exigem garantias ACID completas.
  - **Estado Temporário:** Gerenciamento do estado volátil das salas e matchmaking diretamente na memória do cluster Go, coordenado via Pub/Sub pelo broker **NATS.io**, sem o custo de escritas de banco durante a partida.

### 3. Trade-offs das Opções

#### Opção A (Bancos NoSQL Flexíveis)
- **Prós:** Facilidade de gravação de logs flexíveis de histórico de partidas sem estruturas rígidas.
- **Contras:** Falta de suporte maduro a transações seguras de XP de jogadores e complexidade maior para geração de rankings eficientes e consistentes.

#### Opção B (PostgreSQL Relacional + Estado na Memória do Backend)
- **Prós:** Integridade transacional e financeira do XP garantida pelas regras relacionais do PostgreSQL. Desempenho máximo para atualizações de lobby, já que o estado efêmero é mantido estritamente na RAM dos serviços em Go e propagado instantaneamente via NATS.io, eliminando a carga de gravação de banco durante as jogadas.
- **Contras:** Exige código bem estruturado em Go para limpar memórias e gerenciar estados voláteis em caso de falha de conexão de jogadores.

### 4. Decisão Escolhida

> **Opção Escolhida:** Opção B (PostgreSQL Relacional + Estado na Memória do Backend)
>
> **Justificativa:** A integridade de dados e a segurança do progresso de XP dos jogadores são requisitos fundamentais que o PostgreSQL atende perfeitamente. Unir a robustez transacional do PostgreSQL à altíssima velocidade de gerenciamento de sessões em memória do Go associada ao barramento distribuído NATS.io garante resiliência e tempos de resposta instantâneos.

### 5. Consequências e Impactos

- **Impacto Positivo:** Consultas complexas de rankings globais rápidas através de índices relacionais. Banco de dados relacional protegido de sobrecarga por requisições de alta frequência.
- **Impacto Negativo/Riscos:** Exige sincronização precisa dos microsserviços em Go para registrar os resultados finais das partidas de forma assíncrona ao PostgreSQL.
- **Ações de Mitigação:** Criação de rotinas assíncronas em Go para escrita em lote (batch-write) dos históricos de partidas no PostgreSQL apenas quando houver consenso do término da partida pelos clientes.
