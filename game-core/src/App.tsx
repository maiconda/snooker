import { Canvas, useFrame, useThree } from "@react-three/fiber";
import { OrbitControls } from "@react-three/drei";
import { useCallback, useEffect, useRef, useState } from "react";
import type { ComponentRef, Dispatch, MutableRefObject, SetStateAction } from "react";
import { Vector3 } from "three";
import {
  BALL_RADIUS,
  CUE_BALL_START,
  TABLE_RADIUS,
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

type CueBallPosition = { x: number; y: number };
type CameraMode = "default" | "custom";
type OrbitControlsHandle = ComponentRef<typeof OrbitControls>;

const FORCE_MULTIPLIER = 0.04; // Multiplicador para converter força em velocidade física
const AIM_RESPONSE = 6.4; // Sensibilidade de rotação da mira padrão
const AIM_FINE_RESPONSE = 3.8; // Sensibilidade de rotação da mira precisa
const AIM_RELEASE_RESPONSE = 6.2; // Suavização de parada da mira ao soltar os botões
const AIM_MAX_SPEED = 1.28; // Velocidade máxima da mira
const AIM_FINE_MULTIPLIER = 0.07; // Multiplicador de precisão da mira
const AIM_HUD_UPDATE_INTERVAL = 0.08; // Intervalo de atualização do HUD de mira
const POWER_STEP = 5; // Passo padrão de ajuste de força
const POWER_FINE_STEP = 1; // Passo preciso de ajuste de força
const POCKET_TRANSITION_MS = 560; // Tempo de transição visual das caçapas
const PHYSICS_FIXED_STEP_SECONDS = 1 / 240; // Intervalo fixo da simulação física (240hz)
const PHYSICS_MAX_FRAME_DELTA_SECONDS = 0.1; // Delta máximo de física por frame para evitar travamento
const PHYSICS_MAX_TICKS_PER_FRAME = 24; // Máximo de passos físicos calculados em um único frame
const CUE_RESPAWN_CLEARANCE = BALL_RADIUS * 2.45; // Distância mínima de segurança para reposicionar a bola branca

type AimInputState = {
  left: boolean;
  right: boolean;
  fine: boolean;
};

// Limita o valor de força entre 0% e 100%
function clampPower(value: number): number {
  return Math.min(100, Math.max(0, value));
}

// Verifica se a área no entorno da posição candidata de retorno está desimpedida de outras bolas
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

// Busca uma coordenada desocupada na mesa para o retorno da bola branca após cair na caçapa
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

// Executa passos da simulação física em sincronia com o loop de frames do React Three Fiber
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
  aimAngleRef: MutableRefObject<number>;
  inputRef: MutableRefObject<AimInputState>;
  velocityRef: MutableRefObject<number>;
  setAimAngle: Dispatch<SetStateAction<number>>;
};

// Controla a rotação suave e amortecida do ângulo de mira com base nas teclas pressionadas
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

// Atualiza a posição e o ponto de foco da câmera acompanhando a mira de forma suave
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

// Verifica se o foco do teclado está em um campo de texto ou botão para evitar acionar atalhos de mira/tacada
function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return ["INPUT", "TEXTAREA", "SELECT", "BUTTON"].includes(target.tagName);
}

// Componente raiz do ambiente de testes e simulação física autônoma do jogo
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

  // Instancia as caçapas visuais adicionando IDs únicos e definindo seu estado de animação
  const createRenderedPockets = useCallback(
    (nextPockets: Pocket[], state: RenderedPocket["state"]): RenderedPocket[] =>
      nextPockets.map((pocket) => ({
        ...pocket,
        id: pocketIdRef.current++,
        state
      })),
    []
  );

  // Gerencia a transição com animação de escala de entrada/saída ao atualizar as caçapas ativas na mesa
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

  // Prepara e dispara o início visual da tacada (iniciando animação do taco)
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

      if (event.code === "KeyZ" || event.key.toLowerCase() === "z") {
        event.preventDefault();
        setDefaultZoomed((current) => !current);
        if (cameraMode !== "default") {
          setCameraMode("default");
        }
        return;
      }

      if (!isAiming || isCueAnimating) return;

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
  }, [cameraMode, handleShoot, isAiming, isCueAnimating]);

  // Aplica a força física e ângulo do taco na bola branca ao ocorrer o impacto
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

  // Trata a finalização do movimento de todas as bolas, gerando novas caçapas e a auditoria SHA-256
  const handleSimulationStopped = useCallback(async () => {
    const cueBall = ballsRef.current.find((b) => b.isWhite);
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

  // Reinicia completamente o tabuleiro e as configurações da partida
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
          <div className="key-row">
            <span className="keycap">Z</span>
            <span>Zoom</span>
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
