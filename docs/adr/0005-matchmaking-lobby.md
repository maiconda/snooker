# ADR 0005: Modelo Híbrido de Lobby (Salas Pré-Criadas e Salas Particulares por Código)

- **Status:** Aceito
- **Data:** 2026-05-29
- **Autor:** Antigravity & Jogador (Co-Designers)

### 1. Contexto e Declaração do Problema

A plataforma necessita conectar os jogadores para que as partidas de sinuca iniciem de forma simples e intuitiva. Os jogadores devem ter a flexibilidade de encontrar oponentes rapidamente na comunidade ou, alternativamente, jogar de forma restrita e privada com seus amigos.

### 2. Opções Consideradas

- **Opção A (Fila de Matchmaking Competitiva Pura):** Os jogadores entram em uma fila e o servidor os emparelha de forma totalmente cega e automatizada com base em nível de XP equivalente.
- **Opção B (Modelo Híbrido de Lobby e Código Particular):**
  - **Lobby Público (Salas Pré-Criadas):** Exibe uma lista de mesas de sinuca públicas ativas pré-configuradas onde qualquer jogador pode entrar instantaneamente como competidor ou espectador.
  - **Salas Particulares (Código de Acesso):** Permite a um jogador criar uma mesa exclusiva e protegida, gerando um código único de 6 caracteres para compartilhar com um amigo para acesso direto.

### 3. Trade-offs das Opções

#### Opção A (Fila de Matchmaking Competitiva Pura)
- **Prós:** Ideal para jogos altamente competitivos com grande base de usuários ativos.
- **Contras:** Impede partidas amistosas diretas entre conhecidos, e pode gerar tempos de espera longos em períodos de tráfego baixo.

#### Opção B (Modelo Híbrido de Lobby e Código Particular)
- **Prós:** Excelente engajamento inicial. Dá autonomia total para os usuários jogarem com amigos imediatamente através de códigos simples. A lista de salas pré-criadas garante que sempre haverá uma mesa ativa onde os usuários podem entrar sem depender de algoritmos complexos de pareamento cego.
- **Contras:** Exige que a Core API gerencie com segurança o ciclo de vida e a visibilidade das salas (públicas vs privadas) e previna conflitos de códigos de acesso expirados.

### 4. Decisão Escolhida

> **Opção Escolhida:** Opção B (Modelo Híbrido de Lobby e Código Particular)
>
> **Justificativa:** Esta abordagem atende de forma brilhante à experiência inicial de uso da plataforma Snooker Multiplayer. Ela permite um onboarding social ágil (jogar com amigos compartilhando um código simples) e garante dinâmica comunitária ativa por meio do painel de salas públicas pré-configuradas.

### 5. Consequências e Impactos

- **Impacto Positivo:** Onboarding social extremamente dinâmico e facilidade de testes em ambientes de desenvolvimento.
- **Impacto Negativo/Riscos:** Risco de colisão de códigos gerados de forma aleatória ou acúmulo de salas "fantasmas" (abandonadas).
- **Ações de Mitigação:** Implementar geração de códigos pseudo-aleatórios seguros de 6 caracteres na Core API, com expiração automática de salas ociosas caso o segundo jogador não se conecte dentro de 5 minutos.
