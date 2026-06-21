import type { Ball } from "../physics/engine";

export type AuditState = {
  positions: { id: number; x: number; y: number; sunk: boolean }[];
  hash: string;
  timestamp: number;
};

export async function exportAuditState(balls: Ball[]): Promise<AuditState> {
  // Sort by ID to guarantee exact ordering
  const positions = [...balls]
    .sort((a, b) => a.id - b.id)
    .map((b) => ({
      id: b.id,
      // Round to 4 decimal places (0.1 mm precision) to eliminate floating point noise
      x: parseFloat(b.x.toFixed(4)),
      y: parseFloat(b.y.toFixed(4)),
      sunk: b.sunk
    }));

  const jsonString = JSON.stringify(positions);
  const msgUint8 = new TextEncoder().encode(jsonString);
  
  // Use Browser Web Crypto API for zero-dependency SHA-256
  const hashBuffer = await window.crypto.subtle.digest("SHA-256", msgUint8);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  const hash = hashArray
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");

  return {
    positions,
    hash,
    timestamp: Date.now()
  };
}
