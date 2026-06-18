import { useCallback, useEffect, useRef } from "react";
import { useFrame } from "@react-three/fiber";
import type { ThreeEvent } from "@react-three/fiber";
import { Line } from "@react-three/drei";
import { TABLE_WIDTH, TABLE_HEIGHT, POCKETS, POCKET_RADIUS, BALL_RADIUS } from "../physics/engine";
import type { Group, Mesh } from "three";

type Table3DProps = {
  cueBall: { x: number; y: number } | null;
  aimAngle: number;
  aiming: boolean;
  power: number;
  shotId: number;
  onTablePointerMove: (x: number, z: number) => void;
  onCueContact: () => void;
};

type CueStickProps = {
  cueBall: { x: number; y: number };
  aimAngle: number;
  power: number;
  shotId: number;
  onCueContact: () => void;
};

const TIP_LENGTH = 0.018;
const SHAFT_LENGTH = 0.82;
const REST_TIP_DISTANCE = BALL_RADIUS + 0.06;
const CONTACT_TIP_DISTANCE = BALL_RADIUS + 0.003;
const MAX_PULLBACK = 0.32;
const PULLBACK_SECONDS = 0.18;
const STRIKE_SECONDS = 0.095;

function easeOutCubic(value: number): number {
  return 1 - Math.pow(1 - value, 3);
}

function easeInQuad(value: number): number {
  return value * value;
}

function CueStick({ cueBall, aimAngle, power, shotId, onCueContact }: CueStickProps) {
  const groupRef = useRef<Group>(null);
  const tipRef = useRef<Mesh>(null);
  const shaftRef = useRef<Mesh>(null);
  const onCueContactRef = useRef(onCueContact);
  const phaseRef = useRef<"idle" | "pulling" | "striking">("idle");
  const elapsedRef = useRef(0);
  const pullDistanceRef = useRef(0);
  const contactSentRef = useRef(false);

  const setCueTipDistance = useCallback((tipDistance: number) => {
    if (tipRef.current) {
      tipRef.current.position.z = tipDistance + TIP_LENGTH / 2;
    }

    if (shaftRef.current) {
      shaftRef.current.position.z = tipDistance + TIP_LENGTH + SHAFT_LENGTH / 2;
    }
  }, []);

  useEffect(() => {
    onCueContactRef.current = onCueContact;
  }, [onCueContact]);

  useEffect(() => {
    if (shotId <= 0) {
      phaseRef.current = "idle";
      elapsedRef.current = 0;
      contactSentRef.current = false;
      setCueTipDistance(REST_TIP_DISTANCE);
      return;
    }

    phaseRef.current = "pulling";
    elapsedRef.current = 0;
    contactSentRef.current = false;
    pullDistanceRef.current = Math.max(0.08, (power / 100) * MAX_PULLBACK);
    setCueTipDistance(REST_TIP_DISTANCE);
  }, [power, setCueTipDistance, shotId]);

  useFrame((_, delta) => {
    if (groupRef.current) {
      groupRef.current.position.set(cueBall.x, BALL_RADIUS, cueBall.y);
      groupRef.current.rotation.set(0, -aimAngle - Math.PI / 2, 0);
    }

    const phase = phaseRef.current;
    if (phase === "idle") return;

    const cappedDelta = Math.min(delta, 0.03);
    elapsedRef.current += cappedDelta;

    if (phase === "pulling") {
      const progress = Math.min(elapsedRef.current / PULLBACK_SECONDS, 1);
      const tipDistance =
        REST_TIP_DISTANCE + pullDistanceRef.current * easeOutCubic(progress);
      setCueTipDistance(tipDistance);

      if (progress >= 1) {
        phaseRef.current = "striking";
        elapsedRef.current = 0;
      }

      return;
    }

    const progress = Math.min(elapsedRef.current / STRIKE_SECONDS, 1);
    const pulledDistance = REST_TIP_DISTANCE + pullDistanceRef.current;
    const tipDistance =
      pulledDistance - (pulledDistance - CONTACT_TIP_DISTANCE) * easeInQuad(progress);
    setCueTipDistance(tipDistance);

    if (progress >= 1 && !contactSentRef.current) {
      contactSentRef.current = true;
      phaseRef.current = "idle";
      onCueContactRef.current();
    }
  });

  return (
    <group
      ref={groupRef}
      position={[cueBall.x, BALL_RADIUS, cueBall.y]}
      rotation={[0, -aimAngle - Math.PI / 2, 0]}
    >
      <mesh
        ref={shaftRef}
        position={[0, 0, REST_TIP_DISTANCE + TIP_LENGTH + SHAFT_LENGTH / 2]}
        rotation={[Math.PI / 2, 0, 0]}
      >
        <cylinderGeometry args={[0.006, 0.012, SHAFT_LENGTH, 16]} />
        <meshStandardMaterial color="#d97706" roughness={0.4} />
      </mesh>

      <mesh
        ref={tipRef}
        position={[0, 0, REST_TIP_DISTANCE + TIP_LENGTH / 2]}
        rotation={[Math.PI / 2, 0, 0]}
      >
        <cylinderGeometry args={[0.006, 0.006, TIP_LENGTH, 16]} />
        <meshStandardMaterial color="#f3f4f6" roughness={0.1} />
      </mesh>
    </group>
  );
}

export function Table3D({
  cueBall,
  aimAngle,
  aiming,
  power,
  shotId,
  onTablePointerMove,
  onCueContact
}: Table3DProps) {
  const feltRef = useRef<Mesh>(null);

  // Handle pointer move to aim
  const handlePointerMove = (event: ThreeEvent<PointerEvent>) => {
    event.stopPropagation();
    if (!aiming || !cueBall) return;
    const point = event.point; // 3D point of intersection
    onTablePointerMove(point.x, point.z);
  };

  // Aiming line points
  const getAimLinePoints = () => {
    if (!cueBall) return [];
    const length = 0.9;
    const startX = cueBall.x + Math.cos(aimAngle) * BALL_RADIUS * 1.25;
    const startZ = cueBall.y + Math.sin(aimAngle) * BALL_RADIUS * 1.25;
    const endX = cueBall.x + Math.cos(aimAngle) * length;
    const endZ = cueBall.y + Math.sin(aimAngle) * length;
    return [
      [startX, BALL_RADIUS, startZ],
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
        <CueStick
          cueBall={cueBall}
          aimAngle={aimAngle}
          power={power}
          shotId={shotId}
          onCueContact={onCueContact}
        />
      )}
    </group>
  );
}
