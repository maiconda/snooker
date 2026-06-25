export type Ball = {
  id: number;
  x: number;
  y: number;
  vx: number;
  vy: number;
  spinX: number;
  spinY: number;
  radius: number;
  isWhite: boolean;
  sunk: boolean;
  sinking?: boolean;
  sinkProgress?: number;
  sinkStartX?: number;
  sinkStartY?: number;
  sinkX?: number;
  sinkY?: number;
  color: string;
  owner?: "creator" | "opponent" | "neutral";
  points?: number;
};

export type Pocket = {
  x: number;
  y: number;
};

export const TABLE_RADIUS = 0.95; // Raio da mesa de sinuca
export const TABLE_WIDTH = TABLE_RADIUS * 2; // Largura da mesa
export const TABLE_HEIGHT = TABLE_RADIUS * 2; // Altura da mesa
export const BALL_RADIUS = 0.035; // Raio das bolas
export const POCKET_RADIUS = 0.055; // Raio das caçapas
export const RESTITUTION = 0.96; // Coeficiente de restituição (elasticidade) da colisão entre bolas
export const CUE_BALL_START = { x: -TABLE_RADIUS * 0.64, y: 0 }; // Posição inicial da bola branca

const GRAVITY = 9.81; // Aceleração da gravidade
const SLIDING_FRICTION = 0.18; // Atrito de deslizamento (quando a bola está escorregando)
const ROLLING_FRICTION = 0.035; // Atrito de rolamento puro
const CUSHION_RESTITUTION_MIN = 0.72; // Elasticidade mínima da tabela
const CUSHION_RESTITUTION_MAX = 0.84; // Elasticidade máxima da tabela
const CUSHION_TANGENT_DAMPING_MIN = 0.76; // Amortecimento tangencial mínimo da tabela
const CUSHION_TANGENT_DAMPING_MAX = 0.9; // Amortecimento tangencial máximo da tabela
const STOP_SPEED = 0.003; // Limite de velocidade linear para considerar a bola parada
const STOP_SPIN = 0.08; // Limite de rotação para parar o giro da bola
const SLIP_TO_ROLLING_THRESHOLD = 0.018; // Limiar de velocidade para transição de deslizamento para rolamento
const SINK_ANIMATION_SECONDS = 0.42; // Tempo de queda da bola na caçapa
const CANONICAL_STATE_SCALE = 1_000_000; // Escala para arredondamento de estado
const RANDOM_POCKET_COUNT = 3; // Quantidade de caçapas geradas aleatoriamente
const RANDOM_POCKET_ATTEMPTS = 900; // Tentativas máximas de posicionamento de caçapa
const RANDOM_POCKET_PLACEMENT_RADIUS = TABLE_RADIUS - POCKET_RADIUS * 1.7; // Limite de raio para gerar caçapas
const RANDOM_POCKET_MIN_DISTANCE = POCKET_RADIUS * 3.4; // Distância mínima entre caçapas
const RANDOM_POCKET_BALL_CLEARANCE = POCKET_RADIUS + BALL_RADIUS * 1.9; // Distância mínima entre caçapa e bolas

export const POCKETS: Pocket[] = [
  { x: TABLE_RADIUS * 0.55, y: 0 },
  { x: -TABLE_RADIUS * 0.28, y: TABLE_RADIUS * 0.48 },
  { x: -TABLE_RADIUS * 0.28, y: -TABLE_RADIUS * 0.48 }
];

// Calcula a distância ao quadrado entre dois pontos
function distanceSq(ax: number, ay: number, bx: number, by: number): number {
  const dx = ax - bx;
  const dy = ay - by;
  return dx * dx + dy * dy;
}

// Curva de transição cúbica suave para animações
function easeOutCubic(value: number): number {
  return 1 - Math.pow(1 - value, 3);
}

// Inicia o processo de queda da bola em uma caçapa
function beginSinking(ball: Ball, pocket: Pocket): void {
  stopBall(ball);
  ball.sinking = true;
  ball.sinkProgress = 0;
  ball.sinkStartX = ball.x;
  ball.sinkStartY = ball.y;
  ball.sinkX = pocket.x;
  ball.sinkY = pocket.y;
}

// Atualiza a posição da bola deslizando em direção ao centro da caçapa
function updateSinkingBall(ball: Ball, dt: number): void {
  const progress = Math.min(1, (ball.sinkProgress ?? 0) + dt / SINK_ANIMATION_SECONDS);
  const easedProgress = easeOutCubic(progress);
  const startX = ball.sinkStartX ?? ball.x;
  const startY = ball.sinkStartY ?? ball.y;
  const targetX = ball.sinkX ?? ball.x;
  const targetY = ball.sinkY ?? ball.y;

  ball.sinkProgress = progress;
  ball.x = startX + (targetX - startX) * easedProgress;
  ball.y = startY + (targetY - startY) * easedProgress;

  if (progress >= 1) {
    ball.sinking = false;
    ball.sunk = true;
    ball.x = targetX;
    ball.y = targetY;
  }
}

// Valida se a caçapa candidata respeita as distâncias mínimas de outras caçapas e bolas
function isPocketPlacementValid(pocket: Pocket, pockets: Pocket[], balls: Ball[]): boolean {
  const minPocketDistanceSq = RANDOM_POCKET_MIN_DISTANCE * RANDOM_POCKET_MIN_DISTANCE;
  const ballClearanceSq = RANDOM_POCKET_BALL_CLEARANCE * RANDOM_POCKET_BALL_CLEARANCE;

  for (const placedPocket of pockets) {
    if (distanceSq(pocket.x, pocket.y, placedPocket.x, placedPocket.y) < minPocketDistanceSq) {
      return false;
    }
  }

  for (const ball of balls) {
    if (ball.sunk) continue;
    if (distanceSq(pocket.x, pocket.y, ball.x, ball.y) < ballClearanceSq) {
      return false;
    }
  }

  return true;
}

export function createRandomPockets(balls: Ball[] = []): Pocket[] {
  const pockets: Pocket[] = [];

  for (
    let attempt = 0;
    pockets.length < RANDOM_POCKET_COUNT && attempt < RANDOM_POCKET_ATTEMPTS;
    attempt++
  ) {
    const angle = Math.random() * Math.PI * 2;
    const radius = Math.sqrt(Math.random()) * RANDOM_POCKET_PLACEMENT_RADIUS;
    const pocket = {
      x: Math.cos(angle) * radius,
      y: Math.sin(angle) * radius
    };

    if (isPocketPlacementValid(pocket, pockets, balls)) {
      pockets.push(pocket);
    }
  }

  while (pockets.length < RANDOM_POCKET_COUNT) {
    const angle = (pockets.length / RANDOM_POCKET_COUNT) * Math.PI * 2 + Math.random() * 0.45;
    const radius = RANDOM_POCKET_PLACEMENT_RADIUS * (0.55 + Math.random() * 0.32);
    pockets.push({
      x: Math.cos(angle) * radius,
      y: Math.sin(angle) * radius
    });
  }

  return pockets;
}

// Inicializa o conjunto de bolas nas posições padrão
export function initBalls(): Ball[] {
  const list: Ball[] = [];

  list.push({
    id: 0,
    x: CUE_BALL_START.x,
    y: CUE_BALL_START.y,
    vx: 0,
    vy: 0,
    spinX: 0,
    spinY: 0,
    radius: BALL_RADIUS,
    isWhite: true,
    sunk: false,
    color: "#ffffff"
  });

  const addTargetBall = (id: number, x: number, y: number) => {
    const owner = getBallOwner(id);
    list.push({
      id,
      x,
      y,
      vx: 0,
      vy: 0,
      spinX: 0,
      spinY: 0,
      radius: BALL_RADIUS,
      isWhite: false,
      sunk: false,
      color: getBallColor(id),
      owner,
      points: owner === "neutral" ? 30 : 10
    });
  };

  // Bola preta (id 8) posicionada no centro
  addTargetBall(8, 0, 0);

  // As outras 14 bolas posicionadas em círculo ao redor do centro
  const outerIds = [1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 12, 13, 14, 15];
  const CIRCLE_RADIUS = 0.16; // Raio para evitar sobreposição das bolas

  outerIds.forEach((id, index) => {
    const angle = (index / outerIds.length) * Math.PI * 2;
    addTargetBall(
      id,
      Math.cos(angle) * CIRCLE_RADIUS,
      Math.sin(angle) * CIRCLE_RADIUS
    );
  });

  return list;
}

// Determina qual jogador é dono da bola
export function getBallOwner(id: number): Ball["owner"] {
  if (id >= 1 && id <= 7) return "creator";
  if (id >= 9 && id <= 15) return "opponent";
  if (id === 8) return "neutral";
  return undefined;
}

// Retorna o código de cor hexadecimal correspondente ao ID da bola
function getBallColor(id: number): string {
  if (id >= 1 && id <= 7) return "#f4b942";
  if (id >= 9 && id <= 15) return "#4aa3ff";
  if (id === 8) return "#111111";
  return "#ffffff";
}

// Zera as velocidades lineares e angulares da bola
function stopBall(ball: Ball): void {
  ball.vx = 0;
  ball.vy = 0;
  ball.spinX = 0;
  ball.spinY = 0;
}

// Normaliza números decimais para evitar flutuações e ruídos de ponto flutuante
function canonicalNumber(value: number | undefined): number | undefined {
  if (value === undefined) return undefined;
  const rounded = Math.round(value * CANONICAL_STATE_SCALE) / CANONICAL_STATE_SCALE;
  return Object.is(rounded, -0) ? 0 : rounded;
}

// Normaliza todas as coordenadas e velocidades físicas da bola
function canonicalizeBall(ball: Ball): void {
  ball.x = canonicalNumber(ball.x) ?? ball.x;
  ball.y = canonicalNumber(ball.y) ?? ball.y;
  ball.vx = canonicalNumber(ball.vx) ?? ball.vx;
  ball.vy = canonicalNumber(ball.vy) ?? ball.vy;
  ball.spinX = canonicalNumber(ball.spinX) ?? ball.spinX;
  ball.spinY = canonicalNumber(ball.spinY) ?? ball.spinY;

  ball.sinkProgress = canonicalNumber(ball.sinkProgress);
  ball.sinkStartX = canonicalNumber(ball.sinkStartX);
  ball.sinkStartY = canonicalNumber(ball.sinkStartY);
  ball.sinkX = canonicalNumber(ball.sinkX);
  ball.sinkY = canonicalNumber(ball.sinkY);
}

// Limita um valor de velocidade a zero se ele for menor que a variação mínima
function clampMagnitude(value: number, maxDelta: number): number {
  if (Math.abs(value) <= maxDelta) return 0;
  return value - Math.sign(value) * maxDelta;
}

// Calcula o atrito da bola com o pano da mesa (deslizamento e rolamento)
function applyFeltFriction(ball: Ball, dt: number): void {
  const speed = Math.sqrt(ball.vx * ball.vx + ball.vy * ball.vy);
  
  const slipX = ball.vx + ball.spinY * ball.radius;
  const slipY = ball.vy - ball.spinX * ball.radius;
  const slipSpeed = Math.sqrt(slipX * slipX + slipY * slipY);

  if (speed < STOP_SPEED && slipSpeed < SLIP_TO_ROLLING_THRESHOLD) {
    stopBall(ball);
    return;
  }

  // Se a bola estiver deslizando, reduz a velocidade e gera rotação (efeito)
  if (slipSpeed > SLIP_TO_ROLLING_THRESHOLD) {
    const deceleration = SLIDING_FRICTION * GRAVITY;
    const speedDelta = deceleration * dt;
    const fx = -(slipX / slipSpeed) * speedDelta;
    const fy = -(slipY / slipSpeed) * speedDelta;

    ball.vx += fx;
    ball.vy += fy;

    const angularScale = 2.5 / ball.radius;
    ball.spinX += -fy * angularScale;
    ball.spinY += fx * angularScale;
    return;
  }

  // Se estiver rolando puro, desacelera suavemente
  if (speed > 0) {
    const speedDelta = ROLLING_FRICTION * GRAVITY * dt;
    const nextSpeed = Math.max(0, speed - speedDelta);
    const scale = nextSpeed / speed;

    ball.vx *= scale;
    ball.vy *= scale;

    if (nextSpeed === 0) {
      stopBall(ball);
      return;
    }
  }

  // Sincroniza a rotação com o movimento linear no rolamento puro
  ball.spinX = ball.vy / ball.radius;
  ball.spinY = -ball.vx / ball.radius;

  if (
    Math.sqrt(ball.vx * ball.vx + ball.vy * ball.vy) < STOP_SPEED &&
    Math.sqrt(ball.spinX * ball.spinX + ball.spinY * ball.spinY) < STOP_SPIN
  ) {
    stopBall(ball);
  }
}

// Resolve o impacto e rebote da bola contra a tabela (tabelas amortecidas)
function resolveCushionCollision(ball: Ball, normalX: number, normalY: number): void {
  const normalVelocity = ball.vx * normalX + ball.vy * normalY;
  if (normalVelocity >= 0) return;

  const tangentX = -normalY;
  const tangentY = normalX;
  const tangentVelocity = ball.vx * tangentX + ball.vy * tangentY;

  const speed = Math.sqrt(ball.vx * ball.vx + ball.vy * ball.vy);
  
  const directness = speed > 0 ? Math.abs(normalVelocity) / speed : 1;
  const normalRestitution =
    CUSHION_RESTITUTION_MIN +
    (CUSHION_RESTITUTION_MAX - CUSHION_RESTITUTION_MIN) * directness;
  const tangentDamping =
    CUSHION_TANGENT_DAMPING_MIN +
    (CUSHION_TANGENT_DAMPING_MAX - CUSHION_TANGENT_DAMPING_MIN) * directness;

  const bouncedNormalVelocity = -normalVelocity * normalRestitution;
  const dampedTangentVelocity = tangentVelocity * tangentDamping;

  ball.vx = normalX * bouncedNormalVelocity + tangentX * dampedTangentVelocity;
  ball.vy = normalY * bouncedNormalVelocity + tangentY * dampedTangentVelocity;

  // Transfere o atrito lateral do impacto para a rotação da bola
  const tangentLoss = tangentVelocity - dampedTangentVelocity;
  ball.spinX += (tangentY * tangentLoss * 1.8) / ball.radius;
  ball.spinY += (-tangentX * tangentLoss * 1.8) / ball.radius;
  ball.spinX *= 0.92;
  ball.spinY *= 0.92;
}

// Avança a simulação física do jogo (movimento, caçapas e colisões)
export function stepSimulation(balls: Ball[], dt: number, pockets: Pocket[] = POCKETS): void {
  // 1. Atualiza posições e caçapas
  for (const ball of balls) {
    if (ball.sunk) continue;

    if (ball.sinking) {
      updateSinkingBall(ball, dt);
      continue;
    }

    ball.x += ball.vx * dt;
    ball.y += ball.vy * dt;

    applyFeltFriction(ball, dt);

    for (const pocket of pockets) {
      const dx = ball.x - pocket.x;
      const dy = ball.y - pocket.y;
      const distSq = dx * dx + dy * dy;
      if (distSq < POCKET_RADIUS * POCKET_RADIUS) {
        beginSinking(ball, pocket);
        break;
      }
    }
  }

  // 2. Colisão com a tabela
  for (const ball of balls) {
    if (ball.sunk || ball.sinking) continue;

    const distance = Math.sqrt(ball.x * ball.x + ball.y * ball.y);
    const maxDistance = TABLE_RADIUS - ball.radius;

    if (distance > maxDistance) {
      const outwardX = distance > 0 ? ball.x / distance : 1;
      const outwardY = distance > 0 ? ball.y / distance : 0;

      ball.x = outwardX * maxDistance;
      ball.y = outwardY * maxDistance;
      resolveCushionCollision(ball, -outwardX, -outwardY);
    }
  }

  // 3. Colisões entre as bolas
  for (let i = 0; i < balls.length; i++) {
    const ballA = balls[i];
    if (ballA.sunk || ballA.sinking) continue;

    for (let j = i + 1; j < balls.length; j++) {
      const ballB = balls[j];
      if (ballB.sunk || ballB.sinking) continue;

      const dx = ballB.x - ballA.x;
      const dy = ballB.y - ballA.y;
      const dist = Math.sqrt(dx * dx + dy * dy);
      const minDist = ballA.radius + ballB.radius;

      if (dist < minDist) {
        
        // Corrige a sobreposição física empurrando as bolas
        const overlap = minDist - dist;
        const nx = dist > 0 ? dx / dist : 1;
        const ny = dist > 0 ? dy / dist : 0;

        ballA.x -= nx * overlap * 0.5;
        ballA.y -= ny * overlap * 0.5;
        ballB.x += nx * overlap * 0.5;
        ballB.y += ny * overlap * 0.5;

        // Calcula o impulso elástico/inelástico (conservação de momento)
        const rvx = ballB.vx - ballA.vx;
        const rvy = ballB.vy - ballA.vy;
        const velocityAlongNormal = rvx * nx + rvy * ny;

        if (velocityAlongNormal < 0) {
          const impulse = (-(1 + RESTITUTION) * velocityAlongNormal) / 2;
          const impulseX = impulse * nx;
          const impulseY = impulse * ny;

          ballA.vx -= impulseX;
          ballA.vy -= impulseY;
          ballB.vx += impulseX;
          ballB.vy += impulseY;

          // Transfere efeito (spin) entre as bolas no impacto
          const tangentX = -ny;
          const tangentY = nx;
          const tangentVelocity = rvx * tangentX + rvy * tangentY;
          const tangentImpulse = clampMagnitude(tangentVelocity * 0.04, impulse * 0.12);

          ballA.vx += tangentImpulse * tangentX;
          ballA.vy += tangentImpulse * tangentY;
          ballB.vx -= tangentImpulse * tangentX;
          ballB.vy -= tangentImpulse * tangentY;

          ballA.spinX += (tangentY * tangentImpulse) / ballA.radius;
          ballA.spinY += (-tangentX * tangentImpulse) / ballA.radius;
          ballB.spinX -= (tangentY * tangentImpulse) / ballB.radius;
          ballB.spinY -= (-tangentX * tangentImpulse) / ballB.radius;
        }
      }
    }
  }

  for (const ball of balls) {
    canonicalizeBall(ball);
  }
}

// Verifica se todas as bolas estão imóveis e fora de processo de queda
export function isStatic(balls: Ball[]): boolean {
  for (const ball of balls) {
    if (ball.sinking) return false;

    if (
      !ball.sunk &&
      (ball.vx !== 0 || ball.vy !== 0 || ball.spinX !== 0 || ball.spinY !== 0)
    ) {
      return false;
    }
  }
  return true;
}
