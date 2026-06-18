import { useRef } from "react";
import { Line } from "@react-three/drei";
import { TABLE_WIDTH, TABLE_HEIGHT, POCKETS, POCKET_RADIUS, BALL_RADIUS } from "../physics/engine";

type Table3DProps = {
  cueBall: { x: number; y: number } | null;
  aimAngle: number;
  aiming: boolean;
  onTablePointerMove: (x: number, z: number) => void;
};

export function Table3D({ cueBall, aimAngle, aiming, onTablePointerMove }: Table3DProps) {
  const feltRef = useRef<any>(null);

  // Handle pointer move to aim
  const handlePointerMove = (event: any) => {
    event.stopPropagation();
    if (!aiming || !cueBall) return;
    const point = event.point; // 3D point of intersection
    onTablePointerMove(point.x, point.z);
  };

  // Aiming line points
  const getAimLinePoints = () => {
    if (!cueBall) return [];
    const length = 0.8;
    const endX = cueBall.x + Math.cos(aimAngle) * length;
    const endZ = cueBall.y + Math.sin(aimAngle) * length;
    return [
      [cueBall.x, BALL_RADIUS, cueBall.y],
      [endX, BALL_RADIUS, endZ]
    ] as [number, number, number][];
  };

  const aimPoints = getAimLinePoints();

  return (
    <group>
      {/* 1. Felt (Table Surface) */}
      <mesh
        ref={feltRef}
        rotation={[-Math.PI / 2, 0, 0]}
        position={[0, 0, 0]}
        onPointerMove={handlePointerMove}
        receiveShadow
      >
        <planeGeometry args={[TABLE_WIDTH, TABLE_HEIGHT]} />
        <meshStandardMaterial color="#15803d" roughness={0.8} metalness={0.1} />
      </mesh>

      {/* Under-table support structure */}
      <mesh position={[0, -0.05, 0]}>
        <boxGeometry args={[TABLE_WIDTH + 0.1, 0.09, TABLE_HEIGHT + 0.1]} />
        <meshStandardMaterial color="#1e1b4b" roughness={0.6} />
      </mesh>

      {/* 2. Pockets (rendered as flat black cylinders) */}
      {POCKETS.map((pocket, idx) => (
        <mesh key={idx} position={[pocket.x, 0.001, pocket.y]} rotation={[-Math.PI / 2, 0, 0]}>
          <ringGeometry args={[0, POCKET_RADIUS, 32]} />
          <meshBasicMaterial color="#111111" />
        </mesh>
      ))}

      {/* 3. Cushions / Borders */}
      {/* Top Border */}
      <mesh position={[0, 0.02, TABLE_HEIGHT / 2 + 0.035]}>
        <boxGeometry args={[TABLE_WIDTH + 0.07, 0.04, 0.07]} />
        <meshStandardMaterial color="#451a03" roughness={0.5} />
      </mesh>
      {/* Bottom Border */}
      <mesh position={[0, 0.02, -TABLE_HEIGHT / 2 - 0.035]}>
        <boxGeometry args={[TABLE_WIDTH + 0.07, 0.04, 0.07]} />
        <meshStandardMaterial color="#451a03" roughness={0.5} />
      </mesh>
      {/* Left Border */}
      <mesh position={[-TABLE_WIDTH / 2 - 0.035, 0.02, 0]}>
        <boxGeometry args={[0.07, 0.04, TABLE_HEIGHT + 0.07]} />
        <meshStandardMaterial color="#451a03" roughness={0.5} />
      </mesh>
      {/* Right Border */}
      <mesh position={[TABLE_WIDTH / 2 + 0.035, 0.02, 0]}>
        <boxGeometry args={[0.07, 0.04, TABLE_HEIGHT + 0.07]} />
        <meshStandardMaterial color="#451a03" roughness={0.5} />
      </mesh>

      {/* 4. Aiming Line */}
      {aiming && aimPoints.length > 0 && (
        <Line
          points={aimPoints}
          color="#ffffff"
          lineWidth={2}
          dashed
          dashSize={0.03}
          gapSize={0.02}
        />
      )}

      {/* 5. Cue Stick (Taco) */}
      {aiming && cueBall && (
        <group
          position={[cueBall.x, BALL_RADIUS, cueBall.y]}
          rotation={[0, -aimAngle + Math.PI, 0]} // rotate toward the cue ball
        >
          {/* Cylinder representing the stick, offset so it sits behind the cue ball */}
          <mesh position={[0, 0, 0.45]} rotation={[Math.PI / 2, 0, 0]}>
            <cylinderGeometry args={[0.006, 0.012, 0.8, 16]} />
            <meshStandardMaterial color="#d97706" roughness={0.4} />
          </mesh>
          {/* Tip of the stick */}
          <mesh position={[0, 0, 0.045]} rotation={[Math.PI / 2, 0, 0]}>
            <cylinderGeometry args={[0.006, 0.006, 0.01, 16]} />
            <meshStandardMaterial color="#f3f4f6" roughness={0.1} />
          </mesh>
        </group>
      )}
    </group>
  );
}
