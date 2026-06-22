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
const PHYSICS_BASE_SUBSTEPS = 4;
const PHYSICS_MAX_SUBSTEPS = 8;
const PHYSICS_TARGET_STEP_DISTANCE = BALL_RADIUS * 0.45;
const CUE_RESPAWN_CLEARANCE = BALL_RADIUS * 2.45;
const CUE_SEND_INTERVAL_MS = 33;

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

function getMaxBallSpeed(balls: Ball[]): number {
  return balls.reduce((maxSpeed, ball) => {
    if (ball.sunk || ball.sinking) return maxSpeed;

    const speed = Math.sqrt(ball.vx * ball.vx + ball.vy * ball.vy);
    return Math.max(maxSpeed, speed);
  }, 0);
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
  useFrame((_, delta) => {
    if (isAiming) return;

    const cappedDelta = Math.min(delta, 0.03);
    const maxSpeed = getMaxBallSpeed(ballsRef.current);
    const adaptiveSubsteps = Math.ceil(
      (maxSpeed * cappedDelta) / PHYSICS_TARGET_STEP_DISTANCE
    );
    const substeps = Math.min(
      PHYSICS_MAX_SUBSTEPS,
      Math.max(PHYSICS_BASE_SUBSTEPS, adaptiveSubsteps)
    );
    const subDt = cappedDelta / substeps;

    for (let i = 0; i < substeps; i++) {
      stepSimulation(ballsRef.current, subDt, pocketsRef.current);
    }

    if (isStatic(ballsRef.current)) {
      setIsAiming(true);
      onSimulationStopped();
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
  const lastIncomingSnapshotAtRef = useRef(0);
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
    (nextPockets: Pocket[], includeExitAnimation = true) => {
      const isSamePockets =
        pocketsRef.current.length === nextPockets.length &&
        nextPockets.every((p, idx) => {
          const curr = pocketsRef.current[idx];
          return curr && Math.abs(p.x - curr.x) < 0.0001 && Math.abs(p.y - curr.y) < 0.0001;
        });

      if (isSamePockets) {
        return;
      }

      pocketsRef.current = clonePockets(nextPockets);

      if (pocketTransitionTimerRef.current !== null) {
        window.clearTimeout(pocketTransitionTimerRef.current);
      }

      const enteringPockets = createRenderedPockets(nextPockets, "entering");
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
    lastIncomingSnapshotAtRef.current = 0;
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
    if (!incomingSnapshot || incomingSnapshot.updated_at_ms <= lastIncomingSnapshotAtRef.current) {
      return;
    }

    if (incomingSnapshot.shot_seq < shotSeqRef.current) {
      return;
    }

    if (incomingSnapshot.status === "moving") {
      return;
    }

    const previousTurnUserId = turnUserIdRef.current;
    const turnChanged = incomingSnapshot.turn_user_id !== previousTurnUserId;
    lastIncomingSnapshotAtRef.current = incomingSnapshot.updated_at_ms;

    ballsRef.current = cloneBalls(incomingSnapshot.balls);
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
      const audit = await exportAuditState(ballsRef.current);
      onShotResult({
        shot_seq: pendingShot.shot_seq,
        shooter_user_id: pendingShot.shooter_user_id,
        balls: cloneBalls(ballsRef.current),
        pockets: clonePockets(pocketsRef.current),
        cue_ball_sunk: cueBallWasSunk,
        audit_hash: audit.hash
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

  return (
    <div className="relative h-full min-h-[560px] overflow-hidden bg-[#f8f7f0]">
      {cameraMode === "custom" && (
        <button
          className="absolute left-4 top-4 z-20 border border-neutral-950 bg-white/90 px-3 py-2 text-xs font-semibold text-neutral-950 transition hover:bg-neutral-950 hover:text-white"
          type="button"
          onClick={() => setCameraMode("default")}
        >
          Camera padrao
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

      <div className="pointer-events-none absolute inset-x-4 bottom-4 z-20">
        <div className="pointer-events-auto grid gap-3 border border-neutral-950/15 bg-white/88 p-3 shadow-xl backdrop-blur md:grid-cols-[1fr_auto] md:items-center">
          <div className="grid gap-2">
            <div className="flex items-center justify-between text-xs font-semibold uppercase tracking-[0.14em] text-neutral-600">
              <span>Forca</span>
              <span>{Math.round(power)}%</span>
            </div>
            <input
              type="range"
              min="0"
              max="100"
              value={power}
              onChange={(event) => handlePowerChange(Number(event.target.value))}
              disabled={!canControlCue}
              className="w-full accent-neutral-950 disabled:opacity-40"
            />
            <div className="h-2 overflow-hidden bg-neutral-200">
              <div
                className="h-full bg-[linear-gradient(90deg,#34d399,#facc15,#ef4444)] transition-[width]"
                style={{ width: `${power}%` }}
              />
            </div>
          </div>

          <button
            type="button"
            onClick={handleShoot}
            disabled={!canShoot}
            className="h-11 border border-neutral-950 bg-neutral-950 px-5 text-sm font-semibold text-white transition hover:bg-neutral-800 disabled:cursor-not-allowed disabled:border-neutral-300 disabled:bg-neutral-300 disabled:text-neutral-500"
          >
            Tacada
          </button>
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
