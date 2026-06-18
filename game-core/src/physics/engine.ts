export type Ball = {
  id: number;
  x: number;
  y: number;
  vx: number;
  vy: number;
  radius: number;
  isWhite: boolean;
  sunk: boolean;
  color: string;
};

export const TABLE_WIDTH = 2.0;
export const TABLE_HEIGHT = 1.0;
export const BALL_RADIUS = 0.035;
export const POCKET_RADIUS = 0.055;
export const FRICTION = 0.985; // applied per frame (~60fps)
export const RESTITUTION = 0.85; // bounce elasticity

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

export function stepSimulation(balls: Ball[], dt: number): void {
  // 1. Update positions and check pockets
  for (const ball of balls) {
    if (ball.sunk) continue;

    ball.x += ball.vx * dt;
    ball.y += ball.vy * dt;

    // Apply friction/deceleration
    ball.vx *= FRICTION;
    ball.vy *= FRICTION;

    // Stop completely if very slow
    if (Math.abs(ball.vx) < 0.002) ball.vx = 0;
    if (Math.abs(ball.vy) < 0.002) ball.vy = 0;

    // Check if ball falls into a pocket
    for (const pocket of POCKETS) {
      const dx = ball.x - pocket.x;
      const dy = ball.y - pocket.y;
      const distSq = dx * dx + dy * dy;
      if (distSq < POCKET_RADIUS * POCKET_RADIUS) {
        ball.sunk = true;
        ball.vx = 0;
        ball.vy = 0;
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
      ball.vx = -ball.vx * RESTITUTION;
    } else if (ball.x > maxX) {
      ball.x = maxX;
      ball.vx = -ball.vx * RESTITUTION;
    }

    if (ball.y < minY) {
      ball.y = minY;
      ball.vy = -ball.vy * RESTITUTION;
    } else if (ball.y > maxY) {
      ball.y = maxY;
      ball.vy = -ball.vy * RESTITUTION;
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
        const nx = dx / dist;
        const ny = dy / dist;

        ballA.x -= nx * overlap * 0.5;
        ballA.y -= ny * overlap * 0.5;
        ballB.x += nx * overlap * 0.5;
        ballB.y += ny * overlap * 0.5;

        // Elastic collision math
        const kx = ballA.vx - ballB.vx;
        const ky = ballA.vy - ballB.vy;
        const p = nx * kx + ny * ky;

        if (p > 0) { // they are moving towards each other
          ballA.vx -= p * nx;
          ballA.vy -= p * ny;
          ballB.vx += p * nx;
          ballB.vy += p * ny;
        }
      }
    }
  }
}

export function isStatic(balls: Ball[]): boolean {
  for (const ball of balls) {
    if (!ball.sunk && (ball.vx !== 0 || ball.vy !== 0)) {
      return false;
    }
  }
  return true;
}
