import type { Ball } from "../physics/engine";

export type AuditState = {
  positions: { id: number; x: number; y: number; sunk: boolean }[];
  hash: string;
  timestamp: number;
};

// Exporta o estado atual de todas as bolas e gera um hash SHA-256 para auditoria de integridade física
export async function exportAuditState(balls: Ball[]): Promise<AuditState> {
  // Ordena pelo ID para garantir consistência
  const positions = [...balls]
    .sort((a, b) => a.id - b.id)
    .map((b) => ({
      id: b.id,
      // Arredonda para 4 casas decimais para evitar ruídos de precisão decimal
      x: parseFloat(b.x.toFixed(4)),
      y: parseFloat(b.y.toFixed(4)),
      sunk: b.sunk
    }));

  const jsonString = JSON.stringify(positions);
  const msgUint8 = new TextEncoder().encode(jsonString);
  
  // Usa a Web Crypto API nativa do navegador para calcular o hash SHA-256
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
