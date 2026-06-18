import { Canvas, useFrame, useThree } from "@react-three/fiber";
import { OrbitControls } from "@react-three/drei";
import { useCallback, useEffect, useRef, useState } from "react";
import type { ComponentRef, MutableRefObject } from "react";
import { Vector3 } from "three";
import { BALL_RADIUS, initBalls, isStatic, stepSimulation } from "./physics/engine";
import type { Ball } from "./physics/engine";
import { Table3D } from "./components/Table3D";
import { Ball3D } from "./components/Ball3D";
import { exportAuditState } from "./utils/stateExporter";
import type { AuditState } from "./utils/stateExporter";
import "./App.css";

type SimulationLoopProps = {
  ballsRef: MutableRefObject<Ball[]>;
  isAiming: boolean;
  setIsAiming: (aiming: boolean) => void;
  onSimulationStopped: () => void;
};

type CueBallPosition = { x: number; y: number };
type OrbitControlsHandle = ComponentRef<typeof OrbitControls>;

const FORCE_MULTIPLIER = 0.04;

function SimulationLoop({
  ballsRef,
  isAiming,
  setIsAiming,
  onSimulationStopped
}: SimulationLoopProps) {
  useFrame((_, delta) => {
    if (isAiming) return;

    // Cap delta to prevent huge jumps.
    const cappedDelta = Math.min(delta, 0.03);

    // Sub-stepping physics for stability and tunneling prevention.
    const substeps = 4;
    const subDt = cappedDelta / substeps;

    for (let i = 0; i < substeps; i++) {
      stepSimulation(ballsRef.current, subDt);
    }

    if (isStatic(ballsRef.current)) {
      setIsAiming(true);
      onSimulationStopped();
    }
  });

  return null;
}

type CameraRigProps = {
  cueBall: CueBallPosition | null;
  aimAngle: number;
  isAiming: boolean;
  controlsRef: MutableRefObject<OrbitControlsHandle | null>;
};

function CameraRig({ cueBall, aimAngle, isAiming, controlsRef }: CameraRigProps) {
  const { camera } = useThree();
  const desiredPosition = useRef(new Vector3());
  const desiredTarget = useRef(new Vector3());

  useFrame((_, delta) => {
    const cappedDelta = Math.min(delta, 0.05);
    const smoothFactor = 1 - Math.exp(-cappedDelta * 7);

    if (isAiming && cueBall) {
      const aimX = Math.cos(aimAngle);
      const aimZ = Math.sin(aimAngle);

      desiredPosition.current.set(
        cueBall.x - aimX * 0.78,
        0.46,
        cueBall.y - aimZ * 0.78
      );
      desiredTarget.current.set(
        cueBall.x + aimX * 0.2,
        BALL_RADIUS * 0.9,
        cueBall.y + aimZ * 0.2
      );
    } else {
      desiredPosition.current.set(0, 1.65, 0.95);
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

export function App() {
  const ballsRef = useRef<Ball[]>(initBalls());
  const controlsRef = useRef<OrbitControlsHandle | null>(null);
  const [balls, setBalls] = useState<Ball[]>([]);
  const [isAiming, setIsAiming] = useState(true);
  const [isCueAnimating, setIsCueAnimating] = useState(false);
  const [aimAngle, setAimAngle] = useState(0);
  const [power, setPower] = useState(50);
  const [shotId, setShotId] = useState(0);
  const [auditLogs, setAuditLogs] = useState<AuditState[]>([]);

  // Sync initial state.
  useEffect(() => {
    setBalls([...ballsRef.current]);
  }, []);

  const handleTablePointerMove = (x: number, z: number) => {
    if (!isAiming || isCueAnimating) return;
    const cueBall = ballsRef.current.find((b) => b.isWhite && !b.sunk);
    if (cueBall) {
      const dx = x - cueBall.x;
      const dz = z - cueBall.y;
      setAimAngle(Math.atan2(dz, dx));
    }
  };

  const handleShoot = () => {
    if (!isAiming || isCueAnimating) return;
    const cueBall = ballsRef.current.find((b) => b.isWhite && !b.sunk);
    if (cueBall) {
      setIsCueAnimating(true);
      setShotId((current) => current + 1);
    }
  };

  const handleCueContact = useCallback(() => {
    const cueBall = ballsRef.current.find((b) => b.isWhite && !b.sunk);
    if (!cueBall) {
      setIsCueAnimating(false);
      return;
    }

    const force = power * FORCE_MULTIPLIER;
    cueBall.vx = Math.cos(aimAngle) * force;
    cueBall.vy = Math.sin(aimAngle) * force;
    cueBall.spinX = 0;
    cueBall.spinY = 0;

    setIsCueAnimating(false);
    setIsAiming(false);
  }, [aimAngle, power]);

  const handleSimulationStopped = useCallback(async () => {
    // 1. Respawn white cue ball if pocketed.
    const cueBall = ballsRef.current.find((b) => b.isWhite);
    if (cueBall && cueBall.sunk) {
      cueBall.sunk = false;
      cueBall.x = -0.5;
      cueBall.y = 0;
      cueBall.vx = 0;
      cueBall.vy = 0;
      cueBall.spinX = 0;
      cueBall.spinY = 0;
    }

    // 2. Check if all target balls are sunk, if so restart table.
    const targetBalls = ballsRef.current.filter((b) => !b.isWhite);
    if (targetBalls.every((b) => b.sunk)) {
      ballsRef.current = initBalls();
    }

    // 3. Sync React state to trigger UI render.
    setBalls([...ballsRef.current]);

    // 4. Export audit state and generate SHA-256 hash.
    const audit = await exportAuditState(ballsRef.current);
    setAuditLogs((prev) => [audit, ...prev.slice(0, 9)]);
  }, []);

  const handleReset = () => {
    ballsRef.current = initBalls();
    setBalls([...ballsRef.current]);
    setIsAiming(true);
    setIsCueAnimating(false);
    setAimAngle(0);
    setShotId(0);
  };

  const activeCueBall = balls.find((b) => b.isWhite && !b.sunk);
  const activeCueBallPosition = activeCueBall
    ? { x: activeCueBall.x, y: activeCueBall.y }
    : null;
  const statusLabel = !isAiming
    ? "Simulando..."
    : isCueAnimating
      ? "Tacada em preparo..."
      : "Estatico (Pronto)";

  return (
    <div className="app-shell">
      {/* 3D Canvas viewport */}
      <div className="game-container">
        <Canvas shadows camera={{ position: [0, 1.8, 1.4], fov: 50 }}>
          <color attach="background" args={["#0b0f19"]} />

          <ambientLight intensity={0.4} />
          <directionalLight
            position={[1, 3, 1]}
            intensity={1.2}
            castShadow
            shadow-mapSize={[1024, 1024]}
          />
          <pointLight position={[-1, 2, -1]} intensity={0.5} />

          {/* Table Component */}
          <Table3D
            cueBall={activeCueBallPosition}
            aimAngle={aimAngle}
            aiming={isAiming}
            power={power}
            shotId={shotId}
            onTablePointerMove={handleTablePointerMove}
            onCueContact={handleCueContact}
          />

          {/* Balls Component */}
          {balls.map((ball) => (
            <Ball3D key={ball.id} ball={ball} />
          ))}

          {/* Camera controls */}
          <OrbitControls
            ref={controlsRef}
            enablePan={false}
            enableRotate={!isAiming}
            maxPolarAngle={Math.PI / 2 - 0.05}
            minDistance={0.45}
            maxDistance={3.0}
          />

          <CameraRig
            cueBall={activeCueBallPosition}
            aimAngle={aimAngle}
            isAiming={isAiming}
            controlsRef={controlsRef}
          />

          {/* Simulation Loop */}
          <SimulationLoop
            ballsRef={ballsRef}
            isAiming={isAiming}
            setIsAiming={setIsAiming}
            onSimulationStopped={handleSimulationStopped}
          />
        </Canvas>
      </div>

      {/* Control Sidebar HUD */}
      <div className="sidebar">
        <h2>Snooker Core Engine</h2>

        <div className="instructions">
          <strong>Como Jogar:</strong>
          <ul style={{ margin: "8px 0 0", paddingLeft: "16px" }}>
            <li>Passe o mouse sobre a mesa para mirar.</li>
            <li>Ajuste a forca na barra deslizante.</li>
            <li>Rotacione a camera arrastando fora da mesa.</li>
          </ul>
        </div>

        <div className="control-group">
          <span className="control-label">Status da Fisica</span>
          <div className={`status-badge ${!isAiming ? "moving" : ""}`}>
            {statusLabel}
          </div>
        </div>

        <div className="control-group">
          <span className="control-label">Forca da Tacada ({power}%)</span>
          <div className="slider-container">
            <input
              type="range"
              min="10"
              max="100"
              value={power}
              onChange={(e) => setPower(parseInt(e.target.value))}
              disabled={!isAiming || isCueAnimating}
            />
          </div>
        </div>

        <button
          className="btn"
          onClick={handleShoot}
          disabled={!isAiming || isCueAnimating || !activeCueBall}
        >
          Dar Tacada
        </button>

        <button className="btn btn-secondary" onClick={handleReset}>
          Reiniciar Mesa
        </button>

        <div className="audit-log">
          <span
            className="control-label"
            style={{ borderBottom: "1px solid #1f2937", paddingBottom: "6px" }}
          >
            Historico de Auditoria (SHA-256)
          </span>
          <div className="audit-list">
            {auditLogs.length === 0 ? (
              <div style={{ fontSize: "0.75rem", color: "#6b7280", fontStyle: "italic" }}>
                Nenhuma tacada auditada ainda.
              </div>
            ) : (
              auditLogs.map((log, index) => (
                <div className="audit-item" key={index}>
                  <div style={{ color: "#9ca3af", marginBottom: "4px" }}>
                    Jogada #{auditLogs.length - index} ({new Date(log.timestamp).toLocaleTimeString()})
                  </div>
                  <div className="audit-hash">{log.hash}</div>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

export default App;
