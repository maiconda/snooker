import { Canvas, useFrame, useThree } from "@react-three/fiber";
import { OrbitControls } from "@react-three/drei";
import { useCallback, useEffect, useRef, useState } from "react";
import type { ComponentRef, Dispatch, MutableRefObject, SetStateAction } from "react";
import { Vector3 } from "three";
import {
  BALL_RADIUS,
  CUE_BALL_START,
  TABLE_RADIUS,
  initBalls,
  isStatic,
  stepSimulation
} from "./physics/engine";
import type { Ball, Pocket } from "./physics/engine";
import { Table3D } from "./components/Table3D";
import type { RenderedPocket } from "./components/Table3D";
import { Ball3D } from "./components/Ball3D";
import { exportAuditState } from "./utils/stateExporter";
import { Button } from "../components/Button";

export type Scoreboard = {
  creator: number;
  opponent: number;
};

export type MatchStatus = "aiming" | "striking" | "moving" | "finished";

export type ShotStartedEvent = {
  shot_seq: number;
  shooter_user_id: string;
  angle: number;
  power: number;
  server_started_at_ms?: number;
};

export type MatchSnapshot = {
  balls: Ball[];
  pockets: Pocket[];
  scores: Scoreboard;
  turn_user_id: string;
  turn_seq: number;
  turn_started_at_ms?: number;
  turn_deadline_at_ms?: number;
  shot_seq: number;
  status: MatchStatus;
  winner_user_id?: string;
  audit_hash?: string;
  updated_at_ms: number;
  active_shot?: ShotStartedEvent;
};

export type CueTelemetry = {
  shot_seq: number;
  turn_user_id: string;
  x: number;
  y: number;
  angle: number;
  power: number;
  is_aiming: boolean;
};

export type ShotResultSubmittedEvent = {
  shot_seq: number;
  shooter_user_id: string;
  balls: Ball[];
  pockets: Pocket[];
  cue_ball_sunk: boolean;
  audit_hash?: string;
};

export type MatchHudState = {
  angle: number;
  power: number;
  status: MatchStatus;
  canShoot: boolean;
};

type SnookerMatchProps = {
  creatorId: string;
  opponentId?: string;
  currentUserId: string;
  remoteCue?: CueTelemetry | null;
  incomingShot?: ShotStartedEvent | null;
  incomingSnapshot?: MatchSnapshot | null;
  disabled?: boolean;
  resetKey?: string;
  onCueState: (cue: CueTelemetry) => void;
  onLocalShotStarted: (shot: ShotStartedEvent) => void;
  onShotResult: (result: ShotResultSubmittedEvent) => void;
  onHudChange?: (state: MatchHudState) => void;
};

type SimulationLoopProps = {
  ballsRef: MutableRefObject<Ball[]>;
  pocketsRef: MutableRefObject<Pocket[]>;
  isAiming: boolean;
  setIsAiming: (aiming: boolean) => void;
  onSimulationStopped: () => void;
};

type CueBallPosition = { x: number; y: number };
type CameraMode = "default" | "custom";
type OrbitControlsHandle = ComponentRef<typeof OrbitControls>;

type AimInputState = {
  left: boolean;
  right: boolean;
  fine: boolean;
};

type PendingShot = ShotStartedEvent;
type SnapshotVersion = {
  shotSeq: number;
  turnSeq: number;
  updatedAtMs: number;
  status?: MatchStatus;
};

const FORCE_MULTIPLIER = 0.04;
const AIM_RESPONSE = 6.4;
const AIM_FINE_RESPONSE = 3.8;
const AIM_RELEASE_RESPONSE = 6.2;
const AIM_MAX_SPEED = 1.28;
const AIM_FINE_MULTIPLIER = 0.07;
const AIM_HUD_UPDATE_INTERVAL = 0.033;
const POWER_STEP = 5;
const POWER_FINE_STEP = 1;
const POCKET_TRANSITION_MS = 560;
const PHYSICS_FIXED_STEP_SECONDS = 1 / 240;
const PHYSICS_MAX_FRAME_DELTA_SECONDS = 0.1;
const PHYSICS_MAX_TICKS_PER_FRAME = 24;
const CUE_RESPAWN_CLEARANCE = BALL_RADIUS * 2.45;
const CUE_SEND_INTERVAL_MS = 33;
const AUDIT_POSITION_SCALE = 10_000;

function clampPower(value: number): number {
  return Math.min(100, Math.max(0, value));
}

function normalizeAngle(value: number): number {
  const fullTurn = Math.PI * 2;
  let normalized = value % fullTurn;
  if (normalized <= -Math.PI) normalized += fullTurn;
  if (normalized > Math.PI) normalized -= fullTurn;
  return normalized;
}

function snapshotVersion(snapshot: MatchSnapshot): SnapshotVersion {
  return {
    shotSeq: snapshot.shot_seq,
    turnSeq: snapshot.turn_seq,
    updatedAtMs: snapshot.updated_at_ms,
    status: snapshot.status
  };
}

function isNewerSnapshotVersion(next: SnapshotVersion, current: SnapshotVersion): boolean {
  if (next.shotSeq !== current.shotSeq) {
    return next.shotSeq > current.shotSeq;
  }
  if (next.turnSeq !== current.turnSeq) {
    return next.turnSeq > current.turnSeq;
  }
  if (next.updatedAtMs !== current.updatedAtMs) {
    return next.updatedAtMs > current.updatedAtMs;
  }
  return snapshotStatusRank(next.status) > snapshotStatusRank(current.status);
}

function snapshotStatusRank(status?: MatchStatus): number {
  switch (status) {
    case "aiming":
      return 1;
    case "striking":
      return 2;
    case "moving":
      return 3;
    case "finished":
      return 4;
    default:
      return 0;
  }
}

function auditPosition(value: number): number {
  const rounded = Math.round(value * AUDIT_POSITION_SCALE) / AUDIT_POSITION_SCALE;
  return Object.is(rounded, -0) ? 0 : rounded;
}

function hasSameAuditedBallState(current: Ball[], next: Ball[]): boolean {
  if (current.length !== next.length) return false;

  const currentById = new Map(current.map((ball) => [ball.id, ball]));
  for (const nextBall of next) {
    const currentBall = currentById.get(nextBall.id);
    if (!currentBall) return false;
    if (currentBall.sunk !== nextBall.sunk) return false;
    if (auditPosition(currentBall.x) !== auditPosition(nextBall.x)) return false;
    if (auditPosition(currentBall.y) !== auditPosition(nextBall.y)) return false;
  }

  return true;
}

function alignLocalBallsToSnapshot(current: Ball[], snapshotBalls: Ball[]): Ball[] {
  const currentById = new Map(current.map((ball) => [ball.id, ball]));

  return snapshotBalls.map((snapshotBall) => {
    const localBall = currentById.get(snapshotBall.id);
    if (!localBall) return { ...snapshotBall };

    localBall.vx = snapshotBall.vx;
    localBall.vy = snapshotBall.vy;
    localBall.spinX = snapshotBall.spinX;
    localBall.spinY = snapshotBall.spinY;
    localBall.radius = snapshotBall.radius;
    localBall.isWhite = snapshotBall.isWhite;
    localBall.sunk = snapshotBall.sunk;
    localBall.sinking = snapshotBall.sinking;
    localBall.sinkProgress = snapshotBall.sinkProgress;
    localBall.sinkStartX = snapshotBall.sinkStartX;
    localBall.sinkStartY = snapshotBall.sinkStartY;
    localBall.sinkX = snapshotBall.sinkX;
    localBall.sinkY = snapshotBall.sinkY;
    localBall.color = snapshotBall.color;
    localBall.owner = snapshotBall.owner;
    localBall.points = snapshotBall.points;
    return localBall;
  });
}

function isCueRespawnPositionFree(
  position: CueBallPosition,
  balls: Ball[],
  cueBallId: number
): boolean {
  const playableRadius = TABLE_RADIUS - BALL_RADIUS;
  const distanceFromCenter = Math.sqrt(position.x * position.x + position.y * position.y);
  if (distanceFromCenter > playableRadius) return false;

  const clearanceSq = CUE_RESPAWN_CLEARANCE * CUE_RESPAWN_CLEARANCE;
  return balls.every((ball) => {
    if (ball.id === cueBallId || ball.sunk || ball.sinking) return true;

    const dx = position.x - ball.x;
    const dy = position.y - ball.y;
    return dx * dx + dy * dy >= clearanceSq;
  });
}

function findCueRespawnPosition(balls: Ball[], cueBallId: number): CueBallPosition {
  if (isCueRespawnPositionFree(CUE_BALL_START, balls, cueBallId)) {
    return CUE_BALL_START;
  }

  for (const radius of [0.09, 0.16, 0.24, 0.34]) {
    for (let index = 0; index < 16; index++) {
      const angle = (index / 16) * Math.PI * 2;
      const candidate = {
        x: CUE_BALL_START.x + Math.cos(angle) * radius,
        y: CUE_BALL_START.y + Math.sin(angle) * radius
      };

      if (isCueRespawnPositionFree(candidate, balls, cueBallId)) {
        return candidate;
      }
    }
  }

  return { x: -TABLE_RADIUS * 0.38, y: 0 };
}

function SimulationLoop({
  ballsRef,
  pocketsRef,
  isAiming,
  setIsAiming,
  onSimulationStopped
}: SimulationLoopProps) {
  const accumulatorRef = useRef(0);
  const wasAimingRef = useRef(true);

  useFrame((_, delta) => {
    if (isAiming) {
      accumulatorRef.current = 0;
      wasAimingRef.current = true;
      return;
    }

    if (wasAimingRef.current) {
      accumulatorRef.current = 0;
      wasAimingRef.current = false;
    }

    accumulatorRef.current += Math.min(delta, PHYSICS_MAX_FRAME_DELTA_SECONDS);

    let ticksThisFrame = 0;
    while (
      accumulatorRef.current >= PHYSICS_FIXED_STEP_SECONDS &&
      ticksThisFrame < PHYSICS_MAX_TICKS_PER_FRAME
    ) {
      stepSimulation(ballsRef.current, PHYSICS_FIXED_STEP_SECONDS, pocketsRef.current);
      accumulatorRef.current -= PHYSICS_FIXED_STEP_SECONDS;
      ticksThisFrame++;

      if (isStatic(ballsRef.current)) {
        accumulatorRef.current = 0;
        wasAimingRef.current = true;
        setIsAiming(true);
        onSimulationStopped();
        break;
      }
    }
  });

  return null;
}

type SmoothAimControllerProps = {
  isAiming: boolean;
  isCueAnimating: boolean;
  canControl: boolean;
  aimAngleRef: MutableRefObject<number>;
  inputRef: MutableRefObject<AimInputState>;
  velocityRef: MutableRefObject<number>;
  setAimAngle: Dispatch<SetStateAction<number>>;
};

function SmoothAimController({
  isAiming,
  isCueAnimating,
  canControl,
  aimAngleRef,
  inputRef,
  velocityRef,
  setAimAngle
}: SmoothAimControllerProps) {
  const hudUpdateElapsedRef = useRef(0);
  const lastHudAngleRef = useRef(0);

  useFrame((_, delta) => {
    const cappedDelta = Math.min(delta, 0.033);
    hudUpdateElapsedRef.current += cappedDelta;

    if (!isAiming || isCueAnimating || !canControl) {
      velocityRef.current *= Math.exp(-AIM_RELEASE_RESPONSE * cappedDelta);
      if (Math.abs(velocityRef.current) < 0.0001) {
        velocityRef.current = 0;
      }
      return;
    }

    const input = inputRef.current;
    const direction = Number(input.right) - Number(input.left);
    const precision = input.fine ? AIM_FINE_MULTIPLIER : 1;
    const targetVelocity = direction * AIM_MAX_SPEED * precision;
    const response =
      direction === 0 ? AIM_RELEASE_RESPONSE : input.fine ? AIM_FINE_RESPONSE : AIM_RESPONSE;
    const blend = 1 - Math.exp(-response * cappedDelta);

    velocityRef.current += (targetVelocity - velocityRef.current) * blend;

    if (direction === 0 && Math.abs(velocityRef.current) < 0.0001) {
      velocityRef.current = 0;
    }

    if (Math.abs(velocityRef.current) > 0.0001) {
      aimAngleRef.current += velocityRef.current * cappedDelta;
    }

    if (
      hudUpdateElapsedRef.current >= AIM_HUD_UPDATE_INTERVAL &&
      Math.abs(aimAngleRef.current - lastHudAngleRef.current) > 0.002
    ) {
      lastHudAngleRef.current = aimAngleRef.current;
      hudUpdateElapsedRef.current = 0;
      setAimAngle(normalizeAngle(aimAngleRef.current));
    }
  });

  return null;
}

type CameraRigProps = {
  ballsRef: MutableRefObject<Ball[]>;
  aimAngleRef: MutableRefObject<number>;
  cameraMode: CameraMode;
  defaultZoomed: boolean;
  controlsRef: MutableRefObject<OrbitControlsHandle | null>;
};

function CameraRig({
  ballsRef,
  aimAngleRef,
  cameraMode,
  defaultZoomed,
  controlsRef
}: CameraRigProps) {
  const { camera } = useThree();
  const desiredPosition = useRef(new Vector3());
  const desiredTarget = useRef(new Vector3());

  useFrame((_, delta) => {
    if (cameraMode === "custom") return;

    const cappedDelta = Math.min(delta, 0.05);
    const smoothFactor = 1 - Math.exp(-cappedDelta * 7);

    const liveCueBall = ballsRef.current.find((b) => b.isWhite && !b.sunk);

    if (liveCueBall) {
      const aimAngle = aimAngleRef.current;
      const aimX = Math.cos(aimAngle);
      const aimZ = Math.sin(aimAngle);
      const cameraDistance = defaultZoomed ? 0.46 : 0.78;
      const cameraHeight = defaultZoomed ? 0.32 : 0.46;
      const targetDistance = defaultZoomed ? 0.28 : 0.22;

      desiredPosition.current.set(
        liveCueBall.x - aimX * cameraDistance,
        cameraHeight,
        liveCueBall.y - aimZ * cameraDistance
      );
      desiredTarget.current.set(
        liveCueBall.x + aimX * targetDistance,
        BALL_RADIUS * 0.65,
        liveCueBall.y + aimZ * targetDistance
      );
    } else {
      desiredPosition.current.set(0, 1.5, 0.9);
      desiredTarget.current.set(0, 0, 0);
    }

    camera.position.lerp(desiredPosition.current, smoothFactor);

    if (controlsRef.current) {
      controlsRef.current.target.lerp(desiredTarget.current, smoothFactor);
      controlsRef.current.update();
    } else {
      camera.lookAt(desiredTarget.current);
    }
  });

  return null;
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return ["INPUT", "TEXTAREA", "SELECT", "BUTTON"].includes(target.tagName);
}

export function SnookerMatch({
  creatorId,
  currentUserId,
  remoteCue,
  incomingShot,
  incomingSnapshot,
  disabled = false,
  resetKey,
  onCueState,
  onLocalShotStarted,
  onShotResult,
  onHudChange
}: SnookerMatchProps) {
  const ballsRef = useRef<Ball[]>(initBalls());
  const pocketsRef = useRef<Pocket[]>([]);
  const pocketIdRef = useRef(0);
  const pocketTransitionTimerRef = useRef<number | null>(null);
  const controlsRef = useRef<OrbitControlsHandle | null>(null);
  const aimAngleRef = useRef(0);
  const aimInputRef = useRef<AimInputState>({ left: false, right: false, fine: false });
  const aimVelocityRef = useRef(0);
  const pendingShotRef = useRef<PendingShot | null>(null);
  const lastRemoteShotSeqRef = useRef(0);
  const lastIncomingSnapshotVersionRef = useRef<SnapshotVersion>({
    shotSeq: -1,
    turnSeq: 0,
    updatedAtMs: 0
  });
  const lastCueSentAtRef = useRef(0);
  const shotSeqRef = useRef(0);
  const turnUserIdRef = useRef(creatorId);
  const statusRef = useRef<MatchStatus>("aiming");

  const [balls, setBalls] = useState<Ball[]>([]);
  const [pockets, setPockets] = useState<RenderedPocket[]>([]);
  const [isAiming, setIsAiming] = useState(true);
  const [isCueAnimating, setIsCueAnimating] = useState(false);
  const [cameraMode, setCameraMode] = useState<CameraMode>("default");
  const [defaultZoomed, setDefaultZoomed] = useState(false);
  const [aimAngle, setAimAngle] = useState(0);
  const [power, setPower] = useState(50);
  const [shotId, setShotId] = useState(0);
  const [turnUserId, setTurnUserId] = useState(creatorId);
  const [winnerUserId, setWinnerUserId] = useState<string | undefined>();
  const [status, setStatus] = useState<MatchStatus>("aiming");

  const createRenderedPockets = useCallback(
    (nextPockets: Pocket[], state: RenderedPocket["state"]): RenderedPocket[] =>
      nextPockets.map((pocket) => ({
        ...pocket,
        id: pocketIdRef.current++,
        state
      })),
    []
  );

  const setMatchStatus = useCallback((nextStatus: MatchStatus) => {
    statusRef.current = nextStatus;
    setStatus(nextStatus);
  }, []);

  const setMatchTurn = useCallback((nextTurnUserId: string) => {
    turnUserIdRef.current = nextTurnUserId;
    setTurnUserId(nextTurnUserId);
  }, []);

  const transitionToPockets = useCallback(
    (nextPockets: Pocket[] | null | undefined, includeExitAnimation = true) => {
      const safePockets = nextPockets ?? [];
      const isSamePockets =
        pocketsRef.current.length === safePockets.length &&
        safePockets.every((p, idx) => {
          const curr = pocketsRef.current[idx];
          return curr && Math.abs(p.x - curr.x) < 0.0001 && Math.abs(p.y - curr.y) < 0.0001;
        });

      if (isSamePockets) {
        return;
      }

      pocketsRef.current = clonePockets(safePockets);

      if (pocketTransitionTimerRef.current !== null) {
        window.clearTimeout(pocketTransitionTimerRef.current);
      }

      const enteringPockets = createRenderedPockets(safePockets, "entering");
      setPockets((currentPockets) => [
        ...(includeExitAnimation
          ? currentPockets
              .filter((pocket) => pocket.state !== "exiting")
              .map((pocket) => ({ ...pocket, state: "exiting" as const }))
          : []),
        ...enteringPockets
      ]);

      pocketTransitionTimerRef.current = window.setTimeout(() => {
        setPockets((currentPockets) =>
          currentPockets
            .filter((pocket) => pocket.state !== "exiting")
            .map((pocket) => ({ ...pocket, state: "active" as const }))
        );
        pocketTransitionTimerRef.current = null;
      }, POCKET_TRANSITION_MS);
    },
    [createRenderedPockets]
  );

  const resetMatch = useCallback(() => {
    const initialBalls = initBalls();

    ballsRef.current = initialBalls;
    shotSeqRef.current = 0;
    pendingShotRef.current = null;
    aimAngleRef.current = 0;
    aimVelocityRef.current = 0;
    aimInputRef.current.left = false;
    aimInputRef.current.right = false;
    aimInputRef.current.fine = false;

    // Reset sequence tracking refs to allow correct packet synchronization on reset/rematch
    lastRemoteShotSeqRef.current = 0;
    lastIncomingSnapshotVersionRef.current = {
      shotSeq: -1,
      turnSeq: 0,
      updatedAtMs: 0
    };
    lastCueSentAtRef.current = 0;

    transitionToPockets([], false);
    setBalls([...ballsRef.current]);
    setIsAiming(true);
    setIsCueAnimating(false);
    setCameraMode("default");
    setAimAngle(0);
    setPower(50);
    setShotId(0);
    setWinnerUserId(undefined);
    setMatchTurn(creatorId);
    setMatchStatus("aiming");
  }, [creatorId, setMatchStatus, setMatchTurn, transitionToPockets]);

  useEffect(() => {
    resetMatch();
    return () => {
      if (pocketTransitionTimerRef.current !== null) {
        window.clearTimeout(pocketTransitionTimerRef.current);
      }
    };
  }, [resetKey, resetMatch]);

  const activeCueBall = balls.find((b) => b.isWhite && !b.sunk);
  const activeCueBallPosition = activeCueBall
    ? { x: activeCueBall.x, y: activeCueBall.y }
    : null;
  const activeCueBallId = activeCueBall?.id;
  const isLocalTurn = currentUserId === turnUserId && !winnerUserId;
  const canControlCue = Boolean(isLocalTurn && isAiming && status === "aiming" && !isCueAnimating && !disabled);
  const shouldRenderRemoteCue = Boolean(
    remoteCue &&
    !isLocalTurn &&
    remoteCue.turn_user_id !== currentUserId &&
    remoteCue.is_aiming &&
    !winnerUserId
  );
  const effectivePower = canControlCue ? power : shouldRenderRemoteCue && remoteCue ? remoteCue.power : power;
  const canShoot = Boolean(canControlCue && activeCueBall && power > 0);

  useEffect(() => {
    onHudChange?.({
      angle: aimAngle,
      power,
      status,
      canShoot
    });
  }, [aimAngle, canShoot, onHudChange, power, status]);

  useEffect(() => {
    if (!remoteCue || !shouldRenderRemoteCue) return;
    const remoteAngle = normalizeAngle(remoteCue.angle);
    aimAngleRef.current = remoteAngle;
    setAimAngle(remoteAngle);
    setPower(clampPower(remoteCue.power));
  }, [remoteCue, shouldRenderRemoteCue]);

  useEffect(() => {
    if (!canControlCue) return;

    const publishCueState = () => {
      const cueBall = ballsRef.current.find((b) => b.isWhite && !b.sunk);
      if (!cueBall) return;

      const now = Date.now();
      if (now - lastCueSentAtRef.current < CUE_SEND_INTERVAL_MS) return;
      lastCueSentAtRef.current = now;

      onCueState({
        shot_seq: shotSeqRef.current,
        turn_user_id: turnUserId,
        x: cueBall.x,
        y: cueBall.y,
        angle: normalizeAngle(aimAngleRef.current),
        power,
        is_aiming: true
      });
    };

    publishCueState();
    const timer = window.setInterval(publishCueState, CUE_SEND_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [activeCueBallId, canControlCue, onCueState, power, turnUserId]);

  useEffect(() => {
    if (!incomingSnapshot) {
      return;
    }

    const nextSnapshotVersion = snapshotVersion(incomingSnapshot);
    if (!isNewerSnapshotVersion(nextSnapshotVersion, lastIncomingSnapshotVersionRef.current)) {
      return;
    }

    if (incomingSnapshot.shot_seq < shotSeqRef.current) {
      return;
    }

    if (incomingSnapshot.status === "moving") {
      lastIncomingSnapshotVersionRef.current = nextSnapshotVersion;
      return;
    }

    const previousTurnUserId = turnUserIdRef.current;
    const turnChanged = incomingSnapshot.turn_user_id !== previousTurnUserId;
    lastIncomingSnapshotVersionRef.current = nextSnapshotVersion;

    const snapshotBalls = cloneBalls(incomingSnapshot.balls);
    const canPreserveLocalPositions = hasSameAuditedBallState(
      ballsRef.current,
      snapshotBalls
    );
    ballsRef.current = canPreserveLocalPositions
      ? alignLocalBallsToSnapshot(ballsRef.current, snapshotBalls)
      : snapshotBalls;
    shotSeqRef.current = incomingSnapshot.shot_seq;
    pendingShotRef.current = null;

    if (incomingSnapshot.status === "aiming" && turnChanged) {
      aimAngleRef.current = 0;
      aimVelocityRef.current = 0;
      aimInputRef.current.left = false;
      aimInputRef.current.right = false;
      aimInputRef.current.fine = false;
      setAimAngle(0);
      setPower(50);
    }

    transitionToPockets(incomingSnapshot.pockets, false);
    setBalls([...ballsRef.current]);
    setMatchTurn(incomingSnapshot.turn_user_id);
    setWinnerUserId(incomingSnapshot.winner_user_id);
    setMatchStatus(incomingSnapshot.status);
    setIsAiming(incomingSnapshot.status !== "striking");
    setIsCueAnimating(false);
  }, [incomingSnapshot, setMatchStatus, setMatchTurn, transitionToPockets]);

  const beginShot = useCallback(
    (shot: ShotStartedEvent) => {
      if (shot.shot_seq <= lastRemoteShotSeqRef.current) return;
      lastRemoteShotSeqRef.current = shot.shot_seq;
      const shotAngle = normalizeAngle(shot.angle);

      pendingShotRef.current = {
        ...shot,
        angle: shotAngle
      };
      shotSeqRef.current = Math.max(shotSeqRef.current, shot.shot_seq);
      aimAngleRef.current = shotAngle;
      setAimAngle(shotAngle);
      setPower(clampPower(shot.power));
      setMatchTurn(shot.shooter_user_id);
      setMatchStatus("striking");
      setIsCueAnimating(true);
      setCameraMode("default");
      setShotId((current) => current + 1);
    },
    [setMatchStatus, setMatchTurn]
  );

  useEffect(() => {
    if (!incomingShot) return;
    beginShot(incomingShot);
  }, [beginShot, incomingShot]);

  const handleShoot = useCallback(() => {
    if (!canShoot) return;

    const shot = {
      shot_seq: shotSeqRef.current,
      shooter_user_id: currentUserId,
      angle: normalizeAngle(aimAngleRef.current),
      power
    };
    onLocalShotStarted(shot);
  }, [canShoot, currentUserId, onLocalShotStarted, power]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (isEditableTarget(event.target) || event.ctrlKey || event.metaKey || event.altKey) {
        return;
      }

      if (event.code === "ShiftLeft" || event.code === "ShiftRight") {
        if (!aimInputRef.current.fine) {
          aimVelocityRef.current *= 0.18;
        }
        aimInputRef.current.fine = true;
        return;
      }

      if (event.code === "KeyZ" || event.key.toLowerCase() === "z") {
        event.preventDefault();
        setDefaultZoomed((current) => !current);
        if (cameraMode !== "default") {
          setCameraMode("default");
        }
        return;
      }

      if (!canControlCue) return;

      const powerStep = event.shiftKey ? POWER_FINE_STEP : POWER_STEP;

      switch (event.code) {
        case "ArrowLeft":
          event.preventDefault();
          aimInputRef.current.left = true;
          aimInputRef.current.fine = event.shiftKey;
          break;
        case "ArrowRight":
          event.preventDefault();
          aimInputRef.current.right = true;
          aimInputRef.current.fine = event.shiftKey;
          break;
        case "ArrowUp":
          event.preventDefault();
          setPower((current) => clampPower(current + powerStep));
          break;
        case "ArrowDown":
          event.preventDefault();
          setPower((current) => clampPower(current - powerStep));
          break;
        case "Space":
          event.preventDefault();
          handleShoot();
          break;
        default:
          break;
      }
    };

    const handleKeyUp = (event: KeyboardEvent) => {
      switch (event.code) {
        case "ArrowLeft":
          aimInputRef.current.left = false;
          break;
        case "ArrowRight":
          aimInputRef.current.right = false;
          break;
        case "ShiftLeft":
        case "ShiftRight":
          aimInputRef.current.fine = false;
          break;
        default:
          break;
      }
    };

    const handleBlur = () => {
      aimInputRef.current.left = false;
      aimInputRef.current.right = false;
      aimInputRef.current.fine = false;
      aimVelocityRef.current = 0;
    };

    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("keyup", handleKeyUp);
    window.addEventListener("blur", handleBlur);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("keyup", handleKeyUp);
      window.removeEventListener("blur", handleBlur);
    };
  }, [cameraMode, canControlCue, handleShoot]);

  const handleCueContact = useCallback(() => {
    const cueBall = ballsRef.current.find((b) => b.isWhite && !b.sunk);
    if (!cueBall) {
      setIsCueAnimating(false);
      return;
    }

    const pendingShot = pendingShotRef.current;
    const shotPower = pendingShot?.power ?? power;
    const shotAngle = normalizeAngle(pendingShot?.angle ?? aimAngleRef.current);
    const force = shotPower * FORCE_MULTIPLIER;
    cueBall.vx = Math.cos(shotAngle) * force;
    cueBall.vy = Math.sin(shotAngle) * force;
    cueBall.spinX = 0;
    cueBall.spinY = 0;

    setIsCueAnimating(false);
    setIsAiming(false);
    setMatchStatus("moving");
  }, [power, setMatchStatus]);

  const handleSimulationStopped = useCallback(async () => {
    const cueBall = ballsRef.current.find((b) => b.isWhite);
    const cueBallWasSunk = cueBall ? cueBall.sunk : false;
    const pendingShot = pendingShotRef.current;

    if (cueBall && cueBall.sunk) {
      cueBall.sunk = false;
      cueBall.sinking = false;
      cueBall.sinkProgress = 0;
      const respawnPosition = findCueRespawnPosition(ballsRef.current, cueBall.id);
      cueBall.x = respawnPosition.x;
      cueBall.y = respawnPosition.y;
      cueBall.vx = 0;
      cueBall.vy = 0;
      cueBall.spinX = 0;
      cueBall.spinY = 0;
    }

    setCameraMode("default");
    setBalls([...ballsRef.current]);
    setMatchStatus("moving");

    if (pendingShot?.shooter_user_id === currentUserId) {
      let auditHash: string | undefined;
      try {
        auditHash = (await exportAuditState(ballsRef.current)).hash;
      } catch (err) {
        console.warn("Falha ao gerar hash de auditoria da tacada:", err);
      }

      onShotResult({
        shot_seq: pendingShot.shot_seq,
        shooter_user_id: pendingShot.shooter_user_id,
        balls: cloneBalls(ballsRef.current),
        pockets: clonePockets(pocketsRef.current),
        cue_ball_sunk: cueBallWasSunk,
        audit_hash: auditHash
      });
    }
  }, [currentUserId, onShotResult, setMatchStatus]);

  const handlePowerChange = (nextPower: number) => {
    const clamped = clampPower(nextPower);
    setPower(clamped);
    if (canControlCue && activeCueBall) {
      onCueState({
        shot_seq: shotSeqRef.current,
        turn_user_id: turnUserId,
        x: activeCueBall.x,
        y: activeCueBall.y,
        angle: normalizeAngle(aimAngleRef.current),
        power: clamped,
        is_aiming: true
      });
    }
  };

  const updatePowerFromEvent = (clientY: number, target: HTMLDivElement) => {
    const rect = target.getBoundingClientRect();
    const clickY = clientY - rect.top; // pixel from top of bar
    const pct = 1 - clickY / rect.height; // percentage from bottom
    const nextPower = Math.max(0, Math.min(100, Math.round(pct * 100)));
    handlePowerChange(nextPower);
  };

  const handlePowerMouseDown = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!canControlCue) return;
    const target = e.currentTarget;
    updatePowerFromEvent(e.clientY, target);
    
    const handleMouseMove = (moveEvent: MouseEvent) => {
      updatePowerFromEvent(moveEvent.clientY, target);
    };
    const handleMouseUp = () => {
      window.removeEventListener("mousemove", handleMouseMove);
      window.removeEventListener("mouseup", handleMouseUp);
    };
    window.addEventListener("mousemove", handleMouseMove);
    window.addEventListener("mouseup", handleMouseUp);
  };

  const handlePowerTouchStart = (e: React.TouchEvent<HTMLDivElement>) => {
    if (!canControlCue) return;
    const target = e.currentTarget;
    const touch = e.touches[0];
    updatePowerFromEvent(touch.clientY, target);

    const handleTouchMove = (moveEvent: TouchEvent) => {
      const t = moveEvent.touches[0];
      updatePowerFromEvent(t.clientY, target);
    };
    const handleTouchEnd = () => {
      window.removeEventListener("touchmove", handleTouchMove);
      window.removeEventListener("touchend", handleTouchEnd);
    };
    window.addEventListener("touchmove", handleTouchMove);
    window.addEventListener("touchend", handleTouchEnd);
  };

  return (
    <div className="relative h-full min-h-[560px] overflow-hidden bg-[#f8f7f0]">
      {cameraMode === "custom" && (
        <button
          className="absolute left-4 top-4 z-20 h-12 px-4 rounded-xl border border-white/10 bg-zinc-950/85 hover:bg-zinc-900 text-white font-bold transition flex items-center gap-2 shadow-lg backdrop-blur-md"
          type="button"
          onClick={() => setCameraMode("default")}
        >
          <svg className="h-5 w-5 text-neutral-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
          </svg>
          <span className="text-sm font-black">Câmera Padrão</span>
        </button>
      )}

      <Canvas camera={{ position: [0, 1.5, 0.9], fov: 48 }}>
        <color attach="background" args={["#fbfaf4"]} />

        <ambientLight intensity={0.85} />
        <directionalLight position={[1.5, 3, 1.3]} intensity={1.45} />

        <Table3D
          balls={balls}
          pockets={pockets}
          cueBall={activeCueBallPosition}
          aimAngleRef={aimAngleRef}
          aiming={isAiming && !winnerUserId}
          power={effectivePower}
          shotId={shotId}
          isCueAnimating={isCueAnimating}
          onCueContact={handleCueContact}
          isLocalTurn={isLocalTurn}
        />

        {balls.map((ball) => (
          <Ball3D key={ball.id} ball={ball} />
        ))}

        <SmoothAimController
          isAiming={isAiming}
          isCueAnimating={isCueAnimating}
          canControl={canControlCue}
          aimAngleRef={aimAngleRef}
          inputRef={aimInputRef}
          velocityRef={aimVelocityRef}
          setAimAngle={setAimAngle}
        />

        <OrbitControls
          ref={controlsRef}
          enableDamping
          dampingFactor={0.08}
          enablePan={false}
          enableRotate
          maxPolarAngle={Math.PI / 2 - 0.05}
          minDistance={0.45}
          maxDistance={3.0}
          onStart={() => setCameraMode("custom")}
        />

        <CameraRig
          ballsRef={ballsRef}
          aimAngleRef={aimAngleRef}
          cameraMode={cameraMode}
          defaultZoomed={defaultZoomed}
          controlsRef={controlsRef}
        />

        <SimulationLoop
          ballsRef={ballsRef}
          pocketsRef={pocketsRef}
          isAiming={isAiming}
          setIsAiming={setIsAiming}
          onSimulationStopped={handleSimulationStopped}
        />
      </Canvas>

      <div className="pointer-events-none absolute left-4 top-1/2 -translate-y-1/2 z-20">
        <div className={`pointer-events-auto flex flex-col items-center gap-4 bg-zinc-950/90 border border-white/10 p-4 rounded-2xl shadow-2xl backdrop-blur-xl w-24 transition-opacity duration-300 ${
          canControlCue ? "opacity-100" : "opacity-40 pointer-events-none"
        }`}>
          <div className="text-center">
            <span className="text-[10px] font-black uppercase tracking-widest text-neutral-450 block">Força</span>
            <span className="text-sm font-black text-white block mt-0.5">{Math.round(power)}%</span>
          </div>

          {/* Barra Vertical de Força com Borda Gradiente */}
          <div
            onMouseDown={handlePowerMouseDown}
            onTouchStart={handlePowerTouchStart}
            className="relative w-8 h-48 bg-gradient-to-t from-emerald-500 via-amber-400 to-red-500 p-[2px] rounded-full cursor-pointer select-none"
          >
            <div className="h-full w-full bg-zinc-950 rounded-full overflow-hidden relative flex flex-col justify-end">
              <div
                className="w-full bg-gradient-to-t from-emerald-500/80 via-amber-400/80 to-red-500/80 transition-all duration-75"
                style={{ height: `${power}%` }}
              />
            </div>
          </div>

          <Button
            onClick={handleShoot}
            disabled={!canShoot}
            variant="solid"
            className="h-10 w-full text-xs font-black uppercase tracking-wider"
          >
            Tacada
          </Button>
        </div>
      </div>
    </div>
  );
}

function cloneBalls(balls: Ball[]): Ball[] {
  return balls.map((ball) => ({ ...ball }));
}

function clonePockets(pockets: Pocket[]): Pocket[] {
  return pockets.map((pocket) => ({ ...pocket }));
}
