import { useRef } from "react";
import { useFrame } from "@react-three/fiber";
import { CylinderGeometry, BoxGeometry, RingGeometry } from "three";
import type { Group } from "three";
import type { Ball } from "../physics/engine";

type Ball3DProps = {
  ball: Ball;
};

const TOKEN_HEIGHT = 0.018;
const BALL_RADIUS = 0.035;

// Criação estática das geometrias para otimizar o uso de memória na GPU
const cylinderGeom = new CylinderGeometry(BALL_RADIUS, BALL_RADIUS, TOKEN_HEIGHT, 32);
const boxGeom = new BoxGeometry(BALL_RADIUS * 1.08, 0.0001, 0.004);
const ringGeom1 = new RingGeometry(BALL_RADIUS * 0.72, BALL_RADIUS * 0.78, 32);
const ringGeom2 = new RingGeometry(BALL_RADIUS * 0.96, BALL_RADIUS, 32);

// Interpolação suave para transições
function smoothStep(value: number): number {
  return value * value * (3 - 2 * value);
}

// Componente que renderiza a bola no espaço 3D com rotações e animação de caçapa
export function Ball3D({ ball }: Ball3DProps) {
  const groupRef = useRef<Group>(null);

  useFrame((_, delta) => {
    if (!groupRef.current) return;

    if (ball.sunk) {
      groupRef.current.visible = false;
      return;
    }

    const sinkProgress = ball.sinking ? smoothStep(Math.min(1, ball.sinkProgress ?? 0)) : 0;
    const sinkScale = Math.max(0.12, 1 - sinkProgress * 0.86);
    const sinkY = TOKEN_HEIGHT / 2 - sinkProgress * TOKEN_HEIGHT * 1.45;

    groupRef.current.position.set(ball.x, sinkY, ball.y);
    groupRef.current.scale.set(sinkScale, sinkScale, sinkScale);
    groupRef.current.visible = true;

    const angularSpeed = Math.sqrt(ball.spinX * ball.spinX + ball.spinY * ball.spinY);
    if (angularSpeed > 0.08) {
      groupRef.current.rotation.y += angularSpeed * Math.min(delta, 0.03) * 0.08;
    } else if (ball.sinking) {
      groupRef.current.rotation.y += Math.min(delta, 0.03) * 5.2;
    }
  });

  const tokenColor = ball.isWhite ? "#f9f9f5" : ball.color;
  const markColor = ball.isWhite || tokenColor === "#f2f2f0" ? "#111111" : "#f7f7f2";
  const ringColor = ball.isWhite ? "#9fd3ee" : markColor;

  return (
    <group ref={groupRef} position={[ball.x, TOKEN_HEIGHT / 2, ball.y]}>
      <mesh geometry={cylinderGeom}>
        <meshStandardMaterial color={tokenColor} roughness={0.94} metalness={0} />
      </mesh>

      <mesh position={[0, TOKEN_HEIGHT / 2 + 0.0001, 0]} geometry={boxGeom}>
        <meshBasicMaterial color={markColor} />
      </mesh>

      <mesh position={[0, TOKEN_HEIGHT / 2 + 0.0002, 0]} rotation={[-Math.PI / 2, 0, 0]} geometry={ringGeom1}>
        <meshBasicMaterial color={ringColor} />
      </mesh>

      <mesh position={[0, TOKEN_HEIGHT / 2 + 0.0003, 0]} rotation={[-Math.PI / 2, 0, 0]} geometry={ringGeom2}>
        <meshBasicMaterial color="#b9dff0" />
      </mesh>
    </group>
  );
}
