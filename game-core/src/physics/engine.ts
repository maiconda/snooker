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
};

export type Pocket = {
  x: number;
  y: number;
};

export const TABLE_RADIUS = 0.95;
export const TABLE_WIDTH = TABLE_RADIUS * 2;
export const TABLE_HEIGHT = TABLE_RADIUS * 2;
export const BALL_RADIUS = 0.035;
export const POCKET_RADIUS = 0.055;
export const RESTITUTION = 0.96; // phenolic resin ball-to-ball elasticity
export const CUE_BALL_START = { x: -TABLE_RADIUS * 0.64, y: 0 };

const GRAVITY = 9.81;
const SLIDING_FRICTION = 0.18;
const ROLLING_FRICTION = 0.035;
const CUSHION_RESTITUTION_MIN = 0.72;
const CUSHION_RESTITUTION_MAX = 0.84;
const CUSHION_TANGENT_DAMPING_MIN = 0.76;
const CUSHION_TANGENT_DAMPING_MAX = 0.9;
const STOP_SPEED = 0.003;
const STOP_SPIN = 0.08;
const SLIP_TO_ROLLING_THRESHOLD = 0.018;
const SINK_ANIMATION_SECONDS = 0.42;
const CANONICAL_STATE_SCALE = 1_000_000;
const RANDOM_POCKET_COUNT = 3;
const RANDOM_POCKET_ATTEMPTS = 900;
const RANDOM_POCKET_PLACEMENT_RADIUS = TABLE_RADIUS - POCKET_RADIUS * 1.7;
const RANDOM_POCKET_MIN_DISTANCE = POCKET_RADIUS * 3.4;
const RANDOM_POCKET_BALL_CLEARANCE = POCKET_RADIUS + BALL_RADIUS * 1.9;

export const POCKETS: Pocket[] = [
  { x: TABLE_RADIUS * 0.55, y: 0 },
  { x: -TABLE_RADIUS * 0.28, y: TABLE_RADIUS * 0.48 },
  { x: -TABLE_RADIUS * 0.28, y: -TABLE_RADIUS * 0.48 }
];

function distanceSq(ax: number, ay: number, bx: number, by: number): number {
  const dx = ax - bx;
  const dy = ay - by;
  return dx * dx + dy * dy;
}

function easeOutCubic(value: number): number {
  return 1 - Math.pow(1 - value, 3);
}

function beginSinking(ball: Ball, pocket: Pocket): void {
  stopBall(ball);
  ball.sinking = true;
  ball.sinkProgress = 0;
  ball.sinkStartX = ball.x;
  ball.sinkStartY = ball.y;
  ball.sinkX = pocket.x;
  ball.sinkY = pocket.y;
}

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

  const colors = [
    "#f2f2f0", "#dededb", "#c9c9c5", "#b6b6b1", "#a4a49f",
    "#91918c", "#7f7f7a", "#111111", "#e9e9e6", "#d5d5d1",
    "#c1c1bc", "#adada8", "#9a9a95", "#868681", "#72726d"
  ];

  const addTargetBall = (id: number, x: number, y: number) => {
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
      color: colors[(id - 1) % colors.length]
    });
  };

  // Black ball (id 8, color #111111) exactly at the center
  addTargetBall(8, 0, 0);

  // Remaining 14 target balls arranged in a circle around the center
  const outerIds = [1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 12, 13, 14, 15];
  const CIRCLE_RADIUS = 0.16; // Minimum radius to prevent overlap: 0.1573

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

function stopBall(ball: Ball): void {
  ball.vx = 0;
  ball.vy = 0;
  ball.spinX = 0;
  ball.spinY = 0;
}

function canonicalNumber(value: number | undefined): number | undefined {
  if (value === undefined) return undefined;
  const rounded = Math.round(value * CANONICAL_STATE_SCALE) / CANONICAL_STATE_SCALE;
  return Object.is(rounded, -0) ? 0 : rounded;
}

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

function clampMagnitude(value: number, maxDelta: number): number {
  if (Math.abs(value) <= maxDelta) return 0;
  return value - Math.sign(value) * maxDelta;
}

function applyFeltFriction(ball: Ball, dt: number): void {
  const speed = Math.sqrt(ball.vx * ball.vx + ball.vy * ball.vy);
  const slipX = ball.vx + ball.spinY * ball.radius;
  const slipY = ball.vy - ball.spinX * ball.radius;
  const slipSpeed = Math.sqrt(slipX * slipX + slipY * slipY);

  if (speed < STOP_SPEED && slipSpeed < SLIP_TO_ROLLING_THRESHOLD) {
    stopBall(ball);
    return;
  }

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

  // Keep pure rolling locked to the translational velocity once sliding ends.
  ball.spinX = ball.vy / ball.radius;
  ball.spinY = -ball.vx / ball.radius;

  if (
    Math.sqrt(ball.vx * ball.vx + ball.vy * ball.vy) < STOP_SPEED &&
    Math.sqrt(ball.spinX * ball.spinX + ball.spinY * ball.spinY) < STOP_SPIN
  ) {
    stopBall(ball);
  }
}

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

  const tangentLoss = tangentVelocity - dampedTangentVelocity;
  ball.spinX += (tangentY * tangentLoss * 1.8) / ball.radius;
  ball.spinY += (-tangentX * tangentLoss * 1.8) / ball.radius;
  ball.spinX *= 0.92;
  ball.spinY *= 0.92;
}

export function stepSimulation(balls: Ball[], dt: number, pockets: Pocket[] = POCKETS): void {
  // 1. Update positions and check pockets
  for (const ball of balls) {
    if (ball.sunk) continue;

    if (ball.sinking) {
      updateSinkingBall(ball, dt);
      continue;
    }

    ball.x += ball.vx * dt;
    ball.y += ball.vy * dt;

    applyFeltFriction(ball, dt);

    // Check if ball falls into a pocket
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

  // 2. Check wall collisions
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

  // 3. Check ball-to-ball collisions
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
        // Resolve overlap
        const overlap = minDist - dist;
        const nx = dist > 0 ? dx / dist : 1;
        const ny = dist > 0 ? dy / dist : 0;

        ballA.x -= nx * overlap * 0.5;
        ballA.y -= ny * overlap * 0.5;
        ballB.x += nx * overlap * 0.5;
        ballB.y += ny * overlap * 0.5;

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
