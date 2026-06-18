import { useRef } from "react";
import { useFrame } from "@react-three/fiber";
import * as THREE from "three";
import type { Ball } from "../physics/engine";

type Ball3DProps = {
  ball: Ball;
};

export function Ball3D({ ball }: Ball3DProps) {
  const meshRef = useRef<THREE.Mesh>(null);

  useFrame((_, delta) => {
    if (!meshRef.current) return;

    // Synchronize position
    meshRef.current.position.set(ball.x, ball.radius, ball.y);

    // Hide if pocketed
    if (ball.sunk) {
      meshRef.current.visible = false;
      return;
    } else {
      meshRef.current.visible = true;
    }

    // Accumulate rolling rotation
    const vx = ball.vx;
    const vy = ball.vy;
    const speed = Math.sqrt(vx * vx + vy * vy);

    if (speed > 0.002) {
      const angularSpeed = speed / ball.radius;
      // Cap delta to prevent crazy rotations on frame drops
      const cappedDelta = Math.min(delta, 0.03);
      const angleChange = angularSpeed * cappedDelta;

      // Axis of rotation = direction X UP. 
      // Dir = [vx, 0, vy], UP = [0, 1, 0]
      // Dir X UP = [vy, 0, -vx]
      const axis = new THREE.Vector3(vy, 0, -vx).normalize();
      meshRef.current.rotateOnWorldAxis(axis, angleChange);
    }
  });

  return (
    <mesh ref={meshRef} castShadow receiveShadow>
      <sphereGeometry args={[ball.radius, 32, 32]} />
      {/* Glossy material for billiard ball look */}
      <meshStandardMaterial
        color={ball.color}
        roughness={0.1}
        metalness={0.05}
      />
    </mesh>
  );
}
