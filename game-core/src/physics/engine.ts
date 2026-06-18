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
  color: string;
};

export const TABLE_WIDTH = 2.0;
export const TABLE_HEIGHT = 1.0;
export const BALL_RADIUS = 0.035;
export const POCKET_RADIUS = 0.055;
export const RESTITUTION = 0.96; // phenolic resin ball-to-ball elasticity

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

export const POCKETS = [
  { x: -1.0, y: 0.5 },
  { x: 0.0, y: 0.5 },
  { x: 1.0, y: 0.5 },
  { x: -1.0, y: -0.5 },
  { x: 0.0, y: -0.5 },
  { x: 1.0, y: -0.5 }
];

export function initBalls(): Ball[] {
  const list: Ball[] = [];
  
  // White Cue Ball
  list.push({
    id: 0,
    x: -0.5,
    y: 0,
    vx: 0,
    vy: 0,
    spinX: 0,
    spinY: 0,
    radius: BALL_RADIUS,
    isWhite: true,
    sunk: false,
    color: "#ffffff"
  });

  // Target Balls in a Triangle (Rack)
  // Let's place the rack centered around X = 0.5
  const startX = 0.5;
  const colors = [
    "#facc15", "#3b82f6", "#ef4444", "#8b5cf6", "#f97316",
    "#22c55e", "#b91c1c", "#111111", "#eab308", "#2563eb",
    "#dc2626", "#7c3aed", "#ea580c", "#16a34a", "#991b1b"
  ];
  
  let ballId = 1;
  const rowCount = 5;
  const spacing = BALL_RADIUS * 2.02; // tiny gap to prevent overlapping at start

  for (let row = 0; row < rowCount; row++) {
    const x = startX + row * spacing * 0.866; // cos(30 deg) = 0.866
    const startY = - (row * spacing) / 2;
    for (let col = 0; col <= row; col++) {
      const y = startY + col * spacing;
      list.push({
        id: ballId,
        x,
        y,
        vx: 0,
        vy: 0,
        spinX: 0,
        spinY: 0,
        radius: BALL_RADIUS,
        isWhite: false,
        sunk: false,
        color: colors[(ballId - 1) % colors.length]
      });
      ballId++;
    }
  }

  return list;
}

function stopBall(ball: Ball): void {
  ball.vx = 0;
  ball.vy = 0;
  ball.spinX = 0;
  ball.spinY = 0;
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

export function stepSimulation(balls: Ball[], dt: number): void {
  // 1. Update positions and check pockets
  for (const ball of balls) {
    if (ball.sunk) continue;

    ball.x += ball.vx * dt;
    ball.y += ball.vy * dt;

    applyFeltFriction(ball, dt);

    // Check if ball falls into a pocket
    for (const pocket of POCKETS) {
      const dx = ball.x - pocket.x;
      const dy = ball.y - pocket.y;
      const distSq = dx * dx + dy * dy;
      if (distSq < POCKET_RADIUS * POCKET_RADIUS) {
        ball.sunk = true;
        stopBall(ball);
        break;
      }
    }
  }

  // 2. Check wall collisions
  for (const ball of balls) {
    if (ball.sunk) continue;

    const minX = -TABLE_WIDTH / 2 + ball.radius;
    const maxX = TABLE_WIDTH / 2 - ball.radius;
    const minY = -TABLE_HEIGHT / 2 + ball.radius;
    const maxY = TABLE_HEIGHT / 2 - ball.radius;

    if (ball.x < minX) {
      ball.x = minX;
      resolveCushionCollision(ball, 1, 0);
    } else if (ball.x > maxX) {
      ball.x = maxX;
      resolveCushionCollision(ball, -1, 0);
    }

    if (ball.y < minY) {
      ball.y = minY;
      resolveCushionCollision(ball, 0, 1);
    } else if (ball.y > maxY) {
      ball.y = maxY;
      resolveCushionCollision(ball, 0, -1);
    }
  }

  // 3. Check ball-to-ball collisions
  for (let i = 0; i < balls.length; i++) {
    const ballA = balls[i];
    if (ballA.sunk) continue;

    for (let j = i + 1; j < balls.length; j++) {
      const ballB = balls[j];
      if (ballB.sunk) continue;

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
}

export function isStatic(balls: Ball[]): boolean {
  for (const ball of balls) {
    if (
      !ball.sunk &&
      (ball.vx !== 0 || ball.vy !== 0 || ball.spinX !== 0 || ball.spinY !== 0)
    ) {
      return false;
    }
  }
  return true;
}
