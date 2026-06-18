import { Canvas, useFrame } from "@react-three/fiber";
import { OrbitControls } from "@react-three/drei";
import { useRef, useState, useEffect } from "react";
import { initBalls, stepSimulation, isStatic } from "./physics/engine";
import type { Ball } from "./physics/engine";
import { Table3D } from "./components/Table3D";
import { Ball3D } from "./components/Ball3D";
import { exportAuditState } from "./utils/stateExporter";
import type { AuditState } from "./utils/stateExporter";
import "./App.css";

type SimulationLoopProps = {
  ballsRef: React.MutableRefObject<Ball[]>;
  isAiming: boolean;
  setIsAiming: (aiming: boolean) => void;
  onSimulationStopped: () => void;
};

function SimulationLoop({ ballsRef, isAiming, setIsAiming, onSimulationStopped }: SimulationLoopProps) {
  useFrame((_, delta) => {
    if (isAiming) return;

    // Cap delta to prevent huge jumps
    const cappedDelta = Math.min(delta, 0.03);

    // Sub-stepping physics for stability and tunneling prevention
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

export function App() {
  const ballsRef = useRef<Ball[]>(initBalls());
  const [balls, setBalls] = useState<Ball[]>([]);
  const [isAiming, setIsAiming] = useState(true);
  const [aimAngle, setAimAngle] = useState(0);
  const [power, setPower] = useState(50);
  const [auditLogs, setAuditLogs] = useState<AuditState[]>([]);

  // Sync initial state
  useEffect(() => {
    setBalls([...ballsRef.current]);
  }, []);

  const handleTablePointerMove = (x: number, z: number) => {
    if (!isAiming) return;
    const cueBall = ballsRef.current.find((b) => b.isWhite && !b.sunk);
    if (cueBall) {
      const dx = x - cueBall.x;
      const dz = z - cueBall.y;
      setAimAngle(Math.atan2(dz, dx));
    }
  };

  const handleShoot = () => {
    if (!isAiming) return;
    const cueBall = ballsRef.current.find((b) => b.isWhite && !b.sunk);
    if (cueBall) {
      const forceMultiplier = 0.05;
      const force = power * forceMultiplier;
      cueBall.vx = Math.cos(aimAngle) * force;
      cueBall.vy = Math.sin(aimAngle) * force;
      setIsAiming(false);
    }
  };

  const handleSimulationStopped = async () => {
    // 1. Respawn white cue ball if pocketed
    const cueBall = ballsRef.current.find((b) => b.isWhite);
    if (cueBall && cueBall.sunk) {
      cueBall.sunk = false;
      cueBall.x = -0.5;
      cueBall.y = 0;
      cueBall.vx = 0;
      cueBall.vy = 0;
    }

    // 2. Check if all target balls are sunk, if so restart table
    const targetBalls = ballsRef.current.filter((b) => !b.isWhite);
    if (targetBalls.every((b) => b.sunk)) {
      ballsRef.current = initBalls();
    }

    // 3. Sync React state to trigger UI render
    setBalls([...ballsRef.current]);

    // 4. Export audit state and generate SHA-256 hash
    const audit = await exportAuditState(ballsRef.current);
    setAuditLogs((prev) => [audit, ...prev.slice(0, 9)]);
  };

  const handleReset = () => {
    ballsRef.current = initBalls();
    setBalls([...ballsRef.current]);
    setIsAiming(true);
    setAimAngle(0);
  };

  const activeCueBall = balls.find((b) => b.isWhite && !b.sunk);

  return (
    <div style={{ display: "flex", width: "100vw", height: "100vh", overflow: "hidden" }}>
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
            cueBall={activeCueBall ? { x: activeCueBall.x, y: activeCueBall.y } : null}
            aimAngle={aimAngle}
            aiming={isAiming}
            onTablePointerMove={handleTablePointerMove}
          />

          {/* Balls Component */}
          {balls.map((ball) => (
            <Ball3D key={ball.id} ball={ball} />
          ))}

          {/* Camera controls */}
          <OrbitControls
            enablePan={false}
            maxPolarAngle={Math.PI / 2 - 0.05}
            minDistance={0.8}
            maxDistance={3.0}
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
            <li>Ajuste a força na barra deslizante.</li>
            <li>Rotacione a câmera arrastando fora da mesa.</li>
          </ul>
        </div>

        <div className="control-group">
          <span className="control-label">Status da Física</span>
          <div className={`status-badge ${!isAiming ? "moving" : ""}`}>
            {isAiming ? "Estático (Pronto)" : "Simulando..."}
          </div>
        </div>

        <div className="control-group">
          <span className="control-label">Força da Tacada ({power}%)</span>
          <div className="slider-container">
            <input
              type="range"
              min="10"
              max="100"
              value={power}
              onChange={(e) => setPower(parseInt(e.target.value))}
              disabled={!isAiming}
            />
          </div>
        </div>

        <button
          className="btn"
          onClick={handleShoot}
          disabled={!isAiming || !activeCueBall}
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
            Histórico de Auditoria (SHA-256)
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
