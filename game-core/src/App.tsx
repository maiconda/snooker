import { Canvas, useFrame, useThree } from "@react-three/fiber";
import { OrbitControls } from "@react-three/drei";
import { useCallback, useEffect, useRef, useState } from "react";
import type { ComponentRef, Dispatch, MutableRefObject, SetStateAction } from "react";
import { Vector3 } from "three";
import {
  BALL_RADIUS,
  CUE_BALL_START,
  createRandomPockets,
  initBalls,
  isStatic,
  stepSimulation
} from "./physics/engine";
import type { Ball, Pocket } from "./physics/engine";
import { Table3D } from "./components/Table3D";
import type { RenderedPocket } from "./components/Table3D";
import { Ball3D } from "./components/Ball3D";
import { exportAuditState } from "./utils/stateExporter";
import type { AuditState } from "./utils/stateExporter";
import "./App.css";

type SimulationLoopProps = {
  ballsRef: MutableRefObject<Ball[]>;
  pocketsRef: MutableRefObject<Pocket[]>;
  isAiming: boolean;
  setIsAiming: (aiming: boolean) => void;
  onSimulationStopped: () => void;
};

type CameraMode = "default" | "custom";
type OrbitControlsHandle = ComponentRef<typeof OrbitControls>;

const FORCE_MULTIPLIER = 0.04;
const AIM_RESPONSE = 6.4;
const AIM_FINE_RESPONSE = 3.8;
const AIM_RELEASE_RESPONSE = 6.2;
const AIM_MAX_SPEED = 1.28;
const AIM_FINE_MULTIPLIER = 0.07;
const AIM_HUD_UPDATE_INTERVAL = 0.08;
const POWER_STEP = 5;
const POWER_FINE_STEP = 1;
const POCKET_TRANSITION_MS = 560;

type AimInputState = {
  left: boolean;
  right: boolean;
  fine: boolean;
};

function clampPower(value: number): number {
  return Math.min(100, Math.max(0, value));
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
    const substeps = 4;
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
  aimAngleRef: MutableRefObject<number>;
  inputRef: MutableRefObject<AimInputState>;
  velocityRef: MutableRefObject<number>;
  setAimAngle: Dispatch<SetStateAction<number>>;
};

function SmoothAimController({
  isAiming,
  isCueAnimating,
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

    if (!isAiming || isCueAnimating) {
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
      setAimAngle(aimAngleRef.current);
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

export function App() {
  const ballsRef = useRef<Ball[]>(initBalls());
  const pocketsRef = useRef<Pocket[]>([]);
  const pocketIdRef = useRef(0);
  const pocketTransitionTimerRef = useRef<number | null>(null);
  const controlsRef = useRef<OrbitControlsHandle | null>(null);
  const aimAngleRef = useRef(0);
  const aimInputRef = useRef<AimInputState>({ left: false, right: false, fine: false });
  const aimVelocityRef = useRef(0);
  const [balls, setBalls] = useState<Ball[]>([]);
  const [pockets, setPockets] = useState<RenderedPocket[]>([]);
  const [isAiming, setIsAiming] = useState(true);
  const [isCueAnimating, setIsCueAnimating] = useState(false);
  const [cameraMode, setCameraMode] = useState<CameraMode>("default");
  const [defaultZoomed, setDefaultZoomed] = useState(false);
  const [aimAngle, setAimAngle] = useState(0);
  const [power, setPower] = useState(50);
  const [shotId, setShotId] = useState(0);
  const [auditLogs, setAuditLogs] = useState<AuditState[]>([]);

  const createRenderedPockets = useCallback(
    (nextPockets: Pocket[], state: RenderedPocket["state"]): RenderedPocket[] =>
      nextPockets.map((pocket) => ({
        ...pocket,
        id: pocketIdRef.current++,
        state
      })),
    []
  );

  const transitionToPockets = useCallback(
    (nextPockets: Pocket[], includeExitAnimation = true) => {
      pocketsRef.current = nextPockets;

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

  useEffect(() => {
    const nextPockets = createRandomPockets(ballsRef.current);
    transitionToPockets(nextPockets, false);
    setBalls([...ballsRef.current]);

    return () => {
      if (pocketTransitionTimerRef.current !== null) {
        window.clearTimeout(pocketTransitionTimerRef.current);
      }
    };
  }, [transitionToPockets]);

  const handleShoot = useCallback(() => {
    if (!isAiming || isCueAnimating || power <= 0) return;
    const cueBall = ballsRef.current.find((b) => b.isWhite && !b.sunk);
    if (!cueBall) return;

    setIsCueAnimating(true);
    setCameraMode("default");
    setShotId((current) => current + 1);
  }, [isAiming, isCueAnimating, power]);

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

      if (!isAiming || isCueAnimating) return;

      const powerStep = event.shiftKey ? POWER_FINE_STEP : POWER_STEP;

      switch (event.code) {
        case "KeyZ":
          if (cameraMode === "default") {
            event.preventDefault();
            setDefaultZoomed((current) => !current);
          }
          break;
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
  }, [cameraMode, handleShoot, isAiming, isCueAnimating]);

  const handleCueContact = useCallback(() => {
    const cueBall = ballsRef.current.find((b) => b.isWhite && !b.sunk);
    if (!cueBall) {
      setIsCueAnimating(false);
      return;
    }

    const force = power * FORCE_MULTIPLIER;
    const shotAngle = aimAngleRef.current;
    cueBall.vx = Math.cos(shotAngle) * force;
    cueBall.vy = Math.sin(shotAngle) * force;
    cueBall.spinX = 0;
    cueBall.spinY = 0;

    setIsCueAnimating(false);
    setIsAiming(false);
  }, [power]);

  const handleSimulationStopped = useCallback(async () => {
    const cueBall = ballsRef.current.find((b) => b.isWhite);
    if (cueBall && cueBall.sunk) {
      cueBall.sunk = false;
      cueBall.sinking = false;
      cueBall.sinkProgress = 0;
      cueBall.x = CUE_BALL_START.x;
      cueBall.y = CUE_BALL_START.y;
      cueBall.vx = 0;
      cueBall.vy = 0;
      cueBall.spinX = 0;
      cueBall.spinY = 0;
    }

    const targetBalls = ballsRef.current.filter((b) => !b.isWhite);
    if (targetBalls.every((b) => b.sunk)) {
      ballsRef.current = initBalls();
    }

    const nextPockets = createRandomPockets(ballsRef.current);
    transitionToPockets(nextPockets);
    setCameraMode("default");
    setBalls([...ballsRef.current]);

    const audit = await exportAuditState(ballsRef.current);
    setAuditLogs((prev) => [audit, ...prev.slice(0, 9)]);
  }, [transitionToPockets]);

  const handleReset = () => {
    ballsRef.current = initBalls();
    const nextPockets = createRandomPockets(ballsRef.current);
    transitionToPockets(nextPockets);
    setBalls([...ballsRef.current]);
    setIsAiming(true);
    setIsCueAnimating(false);
    setCameraMode("default");
    aimAngleRef.current = 0;
    setAimAngle(0);
    aimVelocityRef.current = 0;
    aimInputRef.current.left = false;
    aimInputRef.current.right = false;
    aimInputRef.current.fine = false;
    setPower(50);
    setShotId(0);
  };

  const activeCueBall = balls.find((b) => b.isWhite && !b.sunk);
  const activeCueBallPosition = activeCueBall
    ? { x: activeCueBall.x, y: activeCueBall.y }
    : null;
  const statusLabel = !isAiming
    ? "Movimento"
    : isCueAnimating
      ? "Tacada"
      : "Pronto";
  const aimDegrees = Math.round((aimAngle * 180) / Math.PI);

  return (
    <div className="app-shell">
      <div className="game-container">
        {cameraMode === "custom" && (
          <button
            className="camera-reset"
            type="button"
            onClick={() => setCameraMode("default")}
          >
            Camera padrao
          </button>
        )}

        <Canvas camera={{ position: [0, 1.5, 0.9], fov: 48 }}>
          <color attach="background" args={["#fbfaf4"]} />

          <ambientLight intensity={0.85} />
          <directionalLight
            position={[1.5, 3, 1.3]}
            intensity={1.45}
          />

          <Table3D
            balls={balls}
            pockets={pockets}
            cueBall={activeCueBallPosition}
            aimAngleRef={aimAngleRef}
            aiming={isAiming}
            power={power}
            shotId={shotId}
            isCueAnimating={isCueAnimating}
            onCueContact={handleCueContact}
          />

          {balls.map((ball) => (
            <Ball3D key={ball.id} ball={ball} />
          ))}

          <SmoothAimController
            isAiming={isAiming}
            isCueAnimating={isCueAnimating}
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
      </div>

      <aside className="sidebar">
        <header className="panel-header">
          <p className="eyebrow">Paper playfield</p>
          <h1>Paper Shot</h1>
        </header>

        <section className="readout-grid" aria-label="Estado da jogada">
          <div className="readout-item">
            <span>Status</span>
            <strong>{statusLabel}</strong>
          </div>
          <div className="readout-item">
            <span>Forca</span>
            <strong>{power}%</strong>
          </div>
          <div className="readout-item">
            <span>Mira</span>
            <strong>{aimDegrees} deg</strong>
          </div>
          <div className="readout-item">
            <span>Camera</span>
            <strong>
              {cameraMode === "default" ? (defaultZoomed ? "Zoom" : "Padrao") : "Livre"}
            </strong>
          </div>
        </section>

        <section className="control-strip" aria-label="Controles">
          <div className="key-row">
            <span className="keycap wide">Left / Right</span>
            <span>Mira</span>
          </div>
          <div className="key-row">
            <span className="keycap wide">Up / Down</span>
            <span>Forca</span>
          </div>
          <div className="key-row">
            <span className="keycap wide">Espaco</span>
            <span>Tacada</span>
          </div>
        </section>

        <div className="control-group">
          <span className="control-label">Forca</span>
          <input
            className="power-range"
            type="range"
            min="0"
            max="100"
            value={power}
            onChange={(event) => setPower(clampPower(parseInt(event.target.value)))}
            disabled={!isAiming || isCueAnimating}
          />
        </div>

        <button
          className="btn"
          type="button"
          onClick={handleShoot}
          disabled={!isAiming || isCueAnimating || !activeCueBall || power <= 0}
        >
          Tacada
        </button>

        <button className="btn btn-secondary" type="button" onClick={handleReset}>
          Reiniciar
        </button>

        <section className="audit-log">
          <span className="control-label">Auditoria SHA-256</span>
          <div className="audit-list">
            {auditLogs.length === 0 ? (
              <div className="empty-state">Sem jogadas auditadas.</div>
            ) : (
              auditLogs.map((log, index) => (
                <div className="audit-item" key={log.timestamp}>
                  <div className="audit-title">
                    Jogada {auditLogs.length - index}
                    <span>{new Date(log.timestamp).toLocaleTimeString()}</span>
                  </div>
                  <div className="audit-hash">{log.hash}</div>
                </div>
              ))
            )}
          </div>
        </section>
      </aside>
    </div>
  );
}

export default App;
