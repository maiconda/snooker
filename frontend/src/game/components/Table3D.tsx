import { useCallback, useEffect, useRef } from "react";
import { useFrame } from "@react-three/fiber";
import { RoundedBox } from "@react-three/drei";
import {
  TABLE_RADIUS,
  POCKET_RADIUS,
  BALL_RADIUS
} from "../physics/engine";
import type { Ball, Pocket } from "../physics/engine";
import type { RefObject } from "react";
import type { Group, Mesh, MeshStandardMaterial } from "three";
import { CircleGeometry, RingGeometry, InstancedMesh, Object3D } from "three";

export type RenderedPocket = Pocket & {
  id: number;
  state: "entering" | "active" | "exiting";
};

type Table3DProps = {
  balls: Ball[];
  pockets: RenderedPocket[];
  cueBall: { x: number; y: number } | null;
  aimAngleRef: RefObject<number>;
  aiming: boolean;
  power: number;
  shotId: number;
  isCueAnimating: boolean;
  onCueContact: () => void;
  isLocalTurn?: boolean;
};

type MinimalCueProps = {
  cueBall: { x: number; y: number };
  aimAngleRef: RefObject<number>;
  aiming: boolean;
  power: number;
  shotId: number;
  isAnimating: boolean;
  onCueContact: () => void;
  isLocalTurn?: boolean;
};

const CUE_WIDTH = 0.016;
const CUE_HEIGHT = 0.01;
const CUE_LENGTH = 0.25;
const CUE_RADIUS = 0.003;
const CUE_CENTER_Y = 0.038;
const CUE_VISUAL_TILT = -0.15;
const CUE_FADE_SECONDS = 0.48;
const CONTACT_TIP_DISTANCE = BALL_RADIUS + 0.003;
const REST_TIP_DISTANCE = CONTACT_TIP_DISTANCE + 0.014;
const MAX_PULLBACK = 0.07;
const SURFACE_LINE_Y = 0.006;
const AIM_ROUTE_Y = SURFACE_LINE_Y + 0.002;
const AIM_ROUTE_WIDTH = 0.0035;
const AIM_ROUTE_START = BALL_RADIUS * 1.03;
const AIM_ROUTE_MIN_LENGTH = 0.006;
const AIM_PREDICTION_BUFFER = 0;
const AIM_VISUAL_STOP_GAP = 0.0015;
const SECONDARY_ROUTE_WIDTH = 0.0026;
const SECONDARY_ROUTE_START = BALL_RADIUS * 1.04;
const SECONDARY_ROUTE_MAX_LENGTH = 0.58;
const SECONDARY_ROUTE_MIN_LENGTH = 0.045;
const SECONDARY_ROUTE_SEGMENTS = 9;
const MAIN_ROUTE_SEGMENTS = 12;
const POCKET_SCALE_RESPONSE = 10;
const REMOTE_AIM_RESPONSE = 24;
const REMOTE_CUE_POSE_RESPONSE = 24;
const TABLE_WALL_HEIGHT = 0.018;
const TABLE_WALL_THICKNESS = 0.032;
const TABLE_WALL_SEGMENTS = 96;

const WALL_RADIUS = TABLE_RADIUS + TABLE_WALL_THICKNESS / 2;
const WALL_TANGENT_LENGTH = ((WALL_RADIUS * Math.PI * 2) / TABLE_WALL_SEGMENTS) * 1.04;

type AimHit = {
  ball: Ball;
  lineEndDistance: number;
  secondaryAngle: number;
};

// Calcula uma interpolação Hermite suave para transições visuais
function smoothStep(value: number): number {
  return value * value * (3 - 2 * value);
}

const wallSegments = Array.from({ length: TABLE_WALL_SEGMENTS }, (_, index) => {
  const angle = (index / TABLE_WALL_SEGMENTS) * Math.PI * 2;
  const radius = TABLE_RADIUS + TABLE_WALL_THICKNESS / 2;

  return {
    angle,
    x: Math.cos(angle) * radius,
    z: Math.sin(angle) * radius
  };
});

// Calcula a distância da bola branca até a borda da mesa circular
function getTableEdgeDistance(
  cueBall: { x: number; y: number },
  directionX: number,
  directionY: number
): number {
  const playableRadius = TABLE_RADIUS - BALL_RADIUS;
  const projectedCenter = cueBall.x * directionX + cueBall.y * directionY;
  const distanceFromCenterSq = cueBall.x * cueBall.x + cueBall.y * cueBall.y;
  
  const discriminant =
    projectedCenter * projectedCenter - distanceFromCenterSq + playableRadius * playableRadius;

  if (discriminant <= 0) return 0;

  return Math.max(0, -projectedCenter + Math.sqrt(discriminant));
}

// Encontra a primeira bola atingida pela linha de mira
function getFirstBallAimHit(
  cueBall: { x: number; y: number },
  balls: Ball[],
  directionX: number,
  directionY: number
): AimHit | null {
  let nearestHit: AimHit | null = null;
  let nearestContactDistance = Infinity;

  for (const ball of balls) {
    if (ball.sunk || ball.sinking || ball.isWhite) continue;

    const relativeX = ball.x - cueBall.x;
    const relativeY = ball.y - cueBall.y;
    
    const projection = relativeX * directionX + relativeY * directionY;
    if (projection <= AIM_ROUTE_START) continue;

    const closestDistanceSq =
      relativeX * relativeX + relativeY * relativeY - projection * projection;
    
    const physicalContactRadius = BALL_RADIUS + ball.radius;
    const predictionRadius = physicalContactRadius + AIM_PREDICTION_BUFFER;
    const predictionRadiusSq = predictionRadius * predictionRadius;
    
    if (closestDistanceSq > predictionRadiusSq) continue;

    const contactDistance = projection - Math.sqrt(predictionRadiusSq - closestDistanceSq);
    if (contactDistance <= 0) continue;

    const visualRadiusSq = ball.radius * ball.radius;
    const lineEndDistance =
      closestDistanceSq < visualRadiusSq
        ? projection - Math.sqrt(visualRadiusSq - closestDistanceSq) - AIM_VISUAL_STOP_GAP
        : contactDistance + BALL_RADIUS * 0.72;
        
    const physicalContactDistance =
      closestDistanceSq <= physicalContactRadius * physicalContactRadius
        ? projection -
          Math.sqrt(physicalContactRadius * physicalContactRadius - closestDistanceSq)
        : contactDistance;
    const cueContactX = cueBall.x + directionX * physicalContactDistance;
    const cueContactY = cueBall.y + directionY * physicalContactDistance;
    
    const normalX = ball.x - cueContactX;
    const normalY = ball.y - cueContactY;
    const normalLength = Math.max(0.0001, Math.sqrt(normalX * normalX + normalY * normalY));

    if (contactDistance < nearestContactDistance) {
      nearestContactDistance = contactDistance;
      nearestHit = {
        ball,
        lineEndDistance: Math.max(0, lineEndDistance),
        // Rota de saída baseada na normal do ponto de impacto
        secondaryAngle: Math.atan2(normalY / normalLength, normalX / normalLength)
      };
    }
  }

  return nearestHit;
}

// Criação estática das geometrias das caçapas para otimizar o uso da GPU
const pocketCircleGeom = new CircleGeometry(POCKET_RADIUS, 32);
const pocketRingGeom = new RingGeometry(POCKET_RADIUS * 1.12, POCKET_RADIUS * 1.28, 32);

// Componente que renderiza a caçapa no espaço 3D com animação de escala
function PocketVisual({ pocket }: { pocket: RenderedPocket }) {
  const groupRef = useRef<Group>(null);
  const scaleRef = useRef(pocket.state === "entering" ? 0.001 : 1);

  useFrame((_, delta) => {
    if (!groupRef.current) return;

    const targetScale = pocket.state === "exiting" ? 0.001 : 1;
    const blend = 1 - Math.exp(-POCKET_SCALE_RESPONSE * Math.min(delta, 0.04));

    scaleRef.current += (targetScale - scaleRef.current) * blend;
    groupRef.current.position.set(pocket.x, 0.006, pocket.y);
    groupRef.current.scale.set(scaleRef.current, scaleRef.current, scaleRef.current);
  });

  return (
    <group
      ref={groupRef}
      position={[pocket.x, 0.006, pocket.y]}
      rotation={[-Math.PI / 2, 0, 0]}
    >
      <mesh geometry={pocketCircleGeom}>
        <meshBasicMaterial color="#222222" />
      </mesh>
      <mesh position={[0, 0, 0.001]} geometry={pocketRingGeom}>
        <meshBasicMaterial color="#f8f6ee" />
      </mesh>
    </group>
  );
}

// Calcula o recuo do taco com base na força selecionada
function getCueDistanceForPower(
  power: number
): number {
  return REST_TIP_DISTANCE + (power / 100) * MAX_PULLBACK;
}

// Controla e renderiza o taco em 3D
function MinimalCue({
  cueBall,
  aimAngleRef,
  aiming,
  power,
  shotId,
  isAnimating,
  onCueContact,
  isLocalTurn = true
}: MinimalCueProps) {
  const groupRef = useRef<Group>(null);
  const cueRef = useRef<Mesh>(null);
  const cueMaterialRef = useRef<MeshStandardMaterial>(null);
  const onCueContactRef = useRef(onCueContact);
  const lastAnimatedShotRef = useRef(0);
  const phaseRef = useRef<"appearing" | "idle" | "striking" | "followThrough" | "fading" | "hidden">("appearing");
  const cueDistanceRef = useRef(REST_TIP_DISTANCE);
  const cueVelocityRef = useRef(0);
  const contactSentRef = useRef(false);
  const fadeElapsedRef = useRef(0);
  const strikeStartDistanceRef = useRef(REST_TIP_DISTANCE);
  const strikeElapsedRef = useRef(0);
  const lockedPoseRef = useRef({ x: cueBall.x, y: cueBall.y, angle: 0 });
  const visualPoseRef = useRef({ x: cueBall.x, y: cueBall.y, angle: aimAngleRef.current });

  // Ajusta a distância visual da ponta do taco em relação à bola branca no eixo Z
  const setCueTipDistance = useCallback((tipDistance: number) => {
    cueDistanceRef.current = tipDistance;

    if (cueRef.current) {
      // Recuo e avanço do taco no eixo Z
      cueRef.current.position.z = tipDistance + CUE_LENGTH / 2;
    }
  }, []);

  useEffect(() => {
    onCueContactRef.current = onCueContact;
  }, [onCueContact]);

  useEffect(() => {
    if (!isAnimating) {
      if (phaseRef.current === "fading" || phaseRef.current === "followThrough") return;

      phaseRef.current = "idle";
      contactSentRef.current = false;
      cueVelocityRef.current = 0;
      setCueTipDistance(getCueDistanceForPower(power));
      return;
    }

    if (shotId <= 0 || lastAnimatedShotRef.current === shotId) return;

    lastAnimatedShotRef.current = shotId;
    phaseRef.current = "striking";
    contactSentRef.current = false;
    fadeElapsedRef.current = 0;
    strikeElapsedRef.current = 0;
    strikeStartDistanceRef.current = getCueDistanceForPower(power);
    setCueTipDistance(strikeStartDistanceRef.current);
  }, [isAnimating, power, setCueTipDistance, shotId]);

  useFrame((_, delta) => {
    const phase = phaseRef.current;
    const cappedDelta = Math.min(delta, 0.03);

    if (phase === "hidden" && aiming) {
      phaseRef.current = "appearing";
      fadeElapsedRef.current = 0;
      if (cueMaterialRef.current) cueMaterialRef.current.opacity = 0;
    }

    if (groupRef.current) {
      const aimAngle = aimAngleRef.current;
      const targetPose =
        phase === "fading" || phase === "followThrough"
          ? lockedPoseRef.current
          : { x: cueBall.x, y: cueBall.y, angle: aimAngle };

      if (!isLocalTurn) {
        const lerpFactor = 1 - Math.exp(-REMOTE_CUE_POSE_RESPONSE * cappedDelta);
        visualPoseRef.current.x += (targetPose.x - visualPoseRef.current.x) * lerpFactor;
        visualPoseRef.current.y += (targetPose.y - visualPoseRef.current.y) * lerpFactor;
        visualPoseRef.current.angle = targetPose.angle;
      } else {
        visualPoseRef.current.x = targetPose.x;
        visualPoseRef.current.y = targetPose.y;
        visualPoseRef.current.angle = targetPose.angle;
      }

      // Atualiza posição e rotação do taco
      groupRef.current.position.set(visualPoseRef.current.x, CUE_CENTER_Y, visualPoseRef.current.y);
      groupRef.current.rotation.set(0, -visualPoseRef.current.angle - Math.PI / 2, 0);
      groupRef.current.visible =
        aiming ||
        isAnimating ||
        phase === "striking" ||
        phase === "fading" ||
        phase === "appearing" ||
        phase === "followThrough";
    }

    if (cueMaterialRef.current && phase !== "fading" && phase !== "appearing") {
      cueMaterialRef.current.opacity = 1;
    }

    // Simula o movimento do taco usando física de mola (amortecimento e rigidez) para suavidade
    const springTo = (target: number, stiffness: number, damping: number) => {
      const displacement = target - cueDistanceRef.current;
      const acceleration = displacement * stiffness - cueVelocityRef.current * damping;
      cueVelocityRef.current += acceleration * cappedDelta;
      setCueTipDistance(cueDistanceRef.current + cueVelocityRef.current * cappedDelta);
    };

    if (phase === "hidden") {
      return;
    }

    if (phase === "appearing") {
      fadeElapsedRef.current += cappedDelta;
      const fadeProgress = Math.min(1, fadeElapsedRef.current / CUE_FADE_SECONDS);

      if (cueMaterialRef.current) {
        cueMaterialRef.current.opacity = smoothStep(fadeProgress);
      }

      springTo(getCueDistanceForPower(power), 38, 12);

      if (fadeProgress >= 1) {
        phaseRef.current = "idle";
      }
      return;
    }

    if (phase === "idle") {
      springTo(getCueDistanceForPower(power), 38, 12);
      return;
    }

    if (phase === "followThrough") {
      if (aiming && !isAnimating) {
        phaseRef.current = "fading";
        fadeElapsedRef.current = 0;
      }
      return;
    }

    if (phase === "fading") {
      fadeElapsedRef.current += cappedDelta;
      const fadeProgress = Math.min(1, fadeElapsedRef.current / CUE_FADE_SECONDS);

      if (cueMaterialRef.current) {
        cueMaterialRef.current.opacity = 1 - smoothStep(fadeProgress);
      }

      if (fadeProgress >= 1) {
        phaseRef.current = aiming ? "appearing" : "hidden";
        fadeElapsedRef.current = 0;
        if (cueMaterialRef.current) {
          cueMaterialRef.current.opacity = aiming ? 0 : 1;
        }
        if (groupRef.current && !aiming) groupRef.current.visible = false;
      }

      return;
    }

    // Animação de impacto do taco na bola branca
    if (phase === "striking") {
      strikeElapsedRef.current += cappedDelta;
      const duration = 0.18 - (0.18 - 0.06) * (power / 100);
      const progress = Math.min(1, strikeElapsedRef.current / duration);
      const t = progress * progress;
      const currentDistance =
        strikeStartDistanceRef.current + (CONTACT_TIP_DISTANCE - strikeStartDistanceRef.current) * t;

      setCueTipDistance(currentDistance);

      if (progress >= 1 && !contactSentRef.current) {
        contactSentRef.current = true;
        lockedPoseRef.current = {
          x: cueBall.x,
          y: cueBall.y,
          angle: aimAngleRef.current
        };
        phaseRef.current = "followThrough";
        setCueTipDistance(CONTACT_TIP_DISTANCE);
        onCueContactRef.current(); // Dispara o impacto físico
      }
    }
  });

  return (
    <group
      ref={groupRef}
      position={[cueBall.x, CUE_CENTER_Y, cueBall.y]}
      rotation={[0, -Math.PI / 2, 0]}
    >
      {/* Inclinação do taco para simular a postura real */}
      <RoundedBox
        ref={cueRef}
        position={[0, 0, REST_TIP_DISTANCE + CUE_LENGTH / 2]}
        rotation={[CUE_VISUAL_TILT, 0, 0]}
        args={[CUE_WIDTH, CUE_HEIGHT, CUE_LENGTH]}
        radius={CUE_RADIUS}
        smoothness={5}
      >
        <meshStandardMaterial
          ref={cueMaterialRef}
          color="#8d8d89"
          roughness={0.88}
          transparent
          opacity={1}
        />
      </RoundedBox>
    </group>
  );
}

// Componente principal que renderiza a mesa de sinuca circular, caçapas e guias de mira
export function Table3D({
  balls,
  pockets,
  cueBall,
  aimAngleRef,
  aiming,
  power,
  shotId,
  isCueAnimating,
  onCueContact,
  isLocalTurn = true
}: Table3DProps) {
  const feltRef = useRef<Mesh>(null);
  const aimLineRef = useRef<Group>(null);
  const mainRouteSegmentRefs = useRef<Mesh[]>([]);
  const secondaryLineRef = useRef<Group>(null);
  const secondaryRouteSegmentRefs = useRef<Mesh[]>([]);
  const wallRef = useRef<InstancedMesh>(null);

  const visualAimAngleRef = useRef(aimAngleRef.current);
  const visualPowerRef = useRef(power);

  // Renderização otimizada das bordas da mesa em um único Draw Call
  useEffect(() => {
    if (!wallRef.current) return;

    const tempObject = new Object3D();
    wallSegments.forEach((segment, index) => {
      tempObject.position.set(segment.x, TABLE_WALL_HEIGHT / 2, segment.z);
      tempObject.rotation.set(0, Math.PI / 2 - segment.angle, 0);
      tempObject.updateMatrix();
      wallRef.current!.setMatrixAt(index, tempObject.matrix);
    });
    
    wallRef.current.instanceMatrix.needsUpdate = true;
    wallRef.current.computeBoundingBox();
    wallRef.current.computeBoundingSphere();
  }, []);

  useFrame((_, delta) => {
    if (!aimLineRef.current || !cueBall) return;

    const cappedDelta = Math.min(delta, 0.03);
    const targetAngle = aimAngleRef.current;

    if (!isLocalTurn) {
      const lerpFactor = 1 - Math.exp(-REMOTE_AIM_RESPONSE * cappedDelta);
      let diffAngle = targetAngle - visualAimAngleRef.current;
      diffAngle = Math.atan2(Math.sin(diffAngle), Math.cos(diffAngle));
      visualAimAngleRef.current += diffAngle * lerpFactor;
      visualPowerRef.current += (power - visualPowerRef.current) * lerpFactor;
    } else {
      visualAimAngleRef.current = targetAngle;
      visualPowerRef.current = power;
    }

    const aimAngle = visualAimAngleRef.current;
    const visualPower = visualPowerRef.current;

    const directionX = Math.cos(aimAngle);
    const directionY = Math.sin(aimAngle);
    const edgeDistance = getTableEdgeDistance(cueBall, directionX, directionY);
    const aimHit = getFirstBallAimHit(
      cueBall,
      balls,
      directionX,
      directionY
    );
    const routeEndDistance = Math.min(edgeDistance, aimHit?.lineEndDistance ?? edgeDistance);
    const routeLength = Math.max(0, routeEndDistance - AIM_ROUTE_START);
    const routeVisible = aiming && routeLength >= AIM_ROUTE_MIN_LENGTH;

    aimLineRef.current.position.set(cueBall.x, 0, cueBall.y);
    aimLineRef.current.rotation.set(0, -aimAngle - Math.PI / 2, 0);
    aimLineRef.current.visible = routeVisible;

    const mainSegmentLength = routeLength / MAIN_ROUTE_SEGMENTS;

    mainRouteSegmentRefs.current.forEach((segment, index) => {
      if (!segment) return;

      segment.visible = routeVisible && routeLength >= AIM_ROUTE_MIN_LENGTH;
      segment.position.set(
        0,
        AIM_ROUTE_Y,
        -(AIM_ROUTE_START + mainSegmentLength * (index + 0.5))
      );
      segment.scale.set(1, mainSegmentLength, 1);
    });

    if (!secondaryLineRef.current) return;

    const secondaryVisible = Boolean(aiming && routeVisible && aimHit && visualPower > 0);
    secondaryLineRef.current.visible = secondaryVisible;

    if (!secondaryVisible || !aimHit) return;

    const secondaryDirectionX = Math.cos(aimHit.secondaryAngle);
    const secondaryDirectionY = Math.sin(aimHit.secondaryAngle);
    const secondaryEdgeDistance = getTableEdgeDistance(
      aimHit.ball,
      secondaryDirectionX,
      secondaryDirectionY
    );
    const secondaryMaxLength = Math.max(0, secondaryEdgeDistance - SECONDARY_ROUTE_START);
    const secondaryLength = Math.min(
      secondaryMaxLength,
      SECONDARY_ROUTE_MIN_LENGTH + (visualPower / 100) * SECONDARY_ROUTE_MAX_LENGTH
    );
    const segmentLength = secondaryLength / SECONDARY_ROUTE_SEGMENTS;

    secondaryLineRef.current.position.set(aimHit.ball.x, 0, aimHit.ball.y);
    secondaryLineRef.current.rotation.set(0, -aimHit.secondaryAngle - Math.PI / 2, 0);

    secondaryRouteSegmentRefs.current.forEach((segment, index) => {
      if (!segment) return;

      segment.visible = secondaryLength >= AIM_ROUTE_MIN_LENGTH;
      segment.position.set(
        0,
        AIM_ROUTE_Y + 0.001,
        -(SECONDARY_ROUTE_START + segmentLength * (index + 0.5))
      );
      segment.scale.set(1, segmentLength, 1);
    });
  });

  return (
    <group>
      <mesh position={[0, -0.032, 0]}>
        <cylinderGeometry args={[TABLE_RADIUS + 0.04, TABLE_RADIUS + 0.04, 0.05, 128]} />
        <meshStandardMaterial color="#f0eee6" roughness={0.98} />
      </mesh>

      <mesh
        ref={feltRef}
        rotation={[-Math.PI / 2, 0, 0]}
        position={[0, 0, 0]}
      >
        <circleGeometry args={[TABLE_RADIUS, 96]} />
        <meshStandardMaterial color="#fbfaf4" roughness={0.98} metalness={0} />
      </mesh>

      <instancedMesh
        ref={wallRef}
        args={[undefined, undefined, TABLE_WALL_SEGMENTS]}
        frustumCulled={false}
      >
        <boxGeometry
          args={[WALL_TANGENT_LENGTH, TABLE_WALL_HEIGHT, TABLE_WALL_THICKNESS]}
        />
        <meshStandardMaterial color="#8d8e8a" roughness={0.94} />
      </instancedMesh>

      {pockets.map((pocket) => (
        <PocketVisual key={pocket.id} pocket={pocket} />
      ))}

      {cueBall && (
        <group
          ref={aimLineRef}
          position={[cueBall.x, 0, cueBall.y]}
          rotation={[0, -Math.PI / 2, 0]}
          visible={aiming}
        >
          {Array.from({ length: MAIN_ROUTE_SEGMENTS }, (_, index) => {
            const opacity = 0.62 - 0.42 * Math.pow(index / MAIN_ROUTE_SEGMENTS, 1.2);

            return (
              <mesh
                key={`main-route-${index}`}
                ref={(mesh) => {
                  if (mesh) mainRouteSegmentRefs.current[index] = mesh;
                }}
                rotation={[-Math.PI / 2, 0, 0]}
              >
                <planeGeometry args={[AIM_ROUTE_WIDTH, 1]} />
                <meshBasicMaterial
                  color="#1f1f1f"
                  transparent
                  opacity={opacity}
                  depthWrite={false}
                />
              </mesh>
            );
          })}
        </group>
      )}

      <group ref={secondaryLineRef} visible={false}>
        {Array.from({ length: SECONDARY_ROUTE_SEGMENTS }, (_, index) => {
          const opacity = 0.42 * Math.pow(1 - index / SECONDARY_ROUTE_SEGMENTS, 1.5);

          return (
            <mesh
              key={`secondary-route-${index}`}
              ref={(mesh) => {
                if (mesh) secondaryRouteSegmentRefs.current[index] = mesh;
              }}
              rotation={[-Math.PI / 2, 0, 0]}
            >
              <planeGeometry args={[SECONDARY_ROUTE_WIDTH, 1]} />
              <meshBasicMaterial
                color="#1f1f1f"
                transparent
                opacity={opacity}
                depthWrite={false}
              />
            </mesh>
          );
        })}
      </group>

      {cueBall && (
        <MinimalCue
          cueBall={cueBall}
          aimAngleRef={visualAimAngleRef}
          aiming={aiming}
          power={power}
          shotId={shotId}
          isAnimating={isCueAnimating}
          onCueContact={onCueContact}
          isLocalTurn={isLocalTurn}
        />
      )}
    </group>
  );
}
