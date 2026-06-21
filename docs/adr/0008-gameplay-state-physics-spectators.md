# ADR 0008: Gameplay state, deterministic physics, and spectator playback

- **Status:** Aceito
- **Data:** 2026-06-20
- **Autor:** Codex & Jogador (Co-Designers)

### 1. Contexto e Declaracao do Problema

O gameplay multiplayer precisa manter dois jogadores e espectadores vendo a mesma partida sem transmitir coordenadas de todas as bolas em tempo real. A plataforma tambem precisa preservar baixa latencia para quem joga, experiencia fluida para quem assiste, baixo custo operacional e uma trilha de auditoria contra divergencias ou trapacas.

A decisao anterior de fisica deterministica no cliente com auditoria ao final da jogada resolve parte do problema, mas nao define autoridade canonica quando os hashes divergem, nem trata entrada tardia de espectadores, reconexao, ordem de eventos, replay ou correcao visual.

### 2. Opcoes Consideradas

1. **Fisica autoritativa server-side em todas as jogadas:** o servidor calcula toda a simulacao e envia posicoes aos clientes.
2. **Fisica client-side com consenso simples ao final:** os dois jogadores simulam localmente e o servidor compara apenas os hashes finais.
3. **Servidor autoritativo sobre eventos e snapshots, com fisica client-side deterministica, protocolo de commit e verificador sob demanda:** o servidor ordena eventos, valida turno e sequencia, guarda snapshots canonicos, aceita consenso dos jogadores e arbitra divergencias por replay deterministico.

### 3. Avaliacao de Trade-offs

#### Opcao 1: Fisica autoritativa server-side em todas as jogadas

- **Pros:** verdade canonica simples; forte protecao contra cliente malicioso; espectadores podem receber estado oficial.
- **Contras:** alto custo de CPU e rede; maior sensibilidade a latencia; contradiz o objetivo de evitar streaming 60fps; pior experiencia de tacada para jogadores.

#### Opcao 2: Fisica client-side com consenso simples ao final

- **Pros:** baixo custo; animacao local fluida; pouco trafego; implementacao inicial simples.
- **Contras:** detecta divergencia tarde; nao define quem esta certo; nao resolve espectadores entrando no meio da tacada; depende de determinismo perfeito antes de existir mecanismo de recuperacao; dois clientes maliciosos poderiam combinar resultados invalidos se nunca houver verificacao independente.

#### Opcao 3: Servidor autoritativo sobre eventos e snapshots, com commit e verificador sob demanda

- **Pros:** mantem baixo custo e baixa latencia; cria estado canonico de partida; suporta espectadores com replay deterministico; permite auditoria forte; escala melhor que fisica server-side continua; reduz impacto de divergencias reais entre browsers.
- **Contras:** exige contrato de eventos mais rigoroso; exige engine deterministica versionada; adiciona complexidade de replay, fast-forward e disputa.

### 4. Decisao Escolhida

> **Opcao Escolhida:** Servidor autoritativo sobre eventos e snapshots, com fisica client-side deterministica, protocolo de commit e verificador sob demanda.
>
> **Justificativa:** O servidor nao transmitira frames nem calculara fisica continuamente. Ele sera a autoridade sobre partida, turno, `shot_seq`, seed, versao da fisica, eventos aceitos e snapshots confirmados. Jogadores e espectadores reproduzirao localmente a tacada a partir dos mesmos parametros. Ao final, os jogadores enviarao hashes e estado final canonico. O servidor confirmara a jogada em caso de consenso; em divergencia, timeout ou suspeita, acionara replay deterministico server-side para arbitragem.

### 5. Consequencias e Impactos

- **Impacto Positivo:** baixa latencia para jogadores; experiencia fluida para espectadores; menor custo operacional; trilha de auditoria clara; suporte a reconexao e entrada tardia.
- **Impacto Negativo/Riscos:** determinismo precisa ser tratado como requisito de produto; o protocolo de eventos fica mais estrito; o verificador server-side precisa usar a mesma versao da engine.
- **Acoes de Mitigacao:** remover aleatoriedade nao seedada; usar timestep fixo; versionar engine; quantizar estado canonico; incluir `physics_version`, `seed`, `shot_seq` e hash inicial/final em todos os commits; manter replay server-side sob demanda e amostragem antifraude.

### 6. Regras Arquiteturais

1. O servidor e autoridade sobre criacao da partida, jogadores conectados, espectadores conectados, ordem de eventos, turno atual, `shot_seq`, seed, `physics_version`, snapshot canonico confirmado, estado `active_shot` e encerramento.
2. Os clientes sao responsaveis por renderizar a mesa, executar simulacao local deterministica, enviar resultado final quando forem jogadores e reproduzir a partida em modo playback quando forem espectadores.
3. Espectadores nao participam do consenso.
4. Espectadores assistem com buffer curto de playback, inicialmente entre 300ms e 800ms, para suavizar jitter e permitir fast-forward local.
5. Toda tacada nasce de um evento validado pelo servidor. Nenhum cliente altera diretamente o snapshot canonico.
6. Divergencias resultam em `shot_disputed` e arbitragem por replay deterministico. Se o verificador nao estiver disponivel, a partida pausa em estado auditavel.
7. O hash canonico nunca depende de relogio local, ordem instavel de objeto, `Date.now`, `Math.random`, FPS de render ou delta variavel.
8. A mira do jogador do turno pode ser transmitida como telemetria efemera via `cue_state` para oponente e espectadores, incluindo posicao, angulo e forca (`power`) do taco. Esse evento melhora a experiencia de acompanhamento e permite posicionar o taco 3D remoto, mas nunca altera fisica, turno, pontuacao ou snapshot canonico.
9. O chat da sala continua ativo durante a partida usando o mesmo canal WebSocket da sala.
10. Ao fim da partida, a sala permanece viva em estado `finished` ate que os jogadores escolham revanche, volta ao lobby, saida ou encerramento.
11. O MVP integrado publica `shot_started` e `game_state_sync` para compartilhar tacada e snapshot pos-jogada, enquanto a validacao deterministica server-side permanece como evolucao posterior.
12. XP de partida e atribuido pelo servico de perfil a partir de `match_finished`: participantes recebem XP base e vencedor recebe bonus.

### 7. Eventos Minimos

- `match_initialized`
- `spectator_sync`
- `cue_state`
- `shot_started`
- `game_state_sync`
- `shot_result_submitted`
- `shot_committed`
- `shot_disputed`
- `turn_changed`
- `match_finished`
- `rematch_requested`
- `room_reset`
- `room_closed`

Detalhes operacionais do protocolo ficam descritos em `docs/gameplay-state-and-physics.md`.
