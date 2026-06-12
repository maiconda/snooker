import type { AccountStatus, AuthSession } from "./types";

type JwtPayload = {
  email?: string;
  status?: AccountStatus;
  sub?: string;
  exp?: number;
};

export function sessionFromAccessToken(accessToken: string): AuthSession {
  const payload = decodeJwtPayload(accessToken);

  return {
    accessToken,
    status: payload.status ?? "onboarding_pending",
    email: payload.email
  };
}

function decodeJwtPayload(token: string): JwtPayload {
  const [, payload] = token.split(".");
  if (!payload) {
    return {};
  }

  try {
    const normalized = payload.replace(/-/g, "+").replace(/_/g, "/");
    const padded = normalized.padEnd(normalized.length + ((4 - (normalized.length % 4)) % 4), "=");
    const json = atob(padded);
    return JSON.parse(json) as JwtPayload;
  } catch {
    return {};
  }
}
