import { AuthApiError, type ApiErrorBody } from "../auth/types";
import type { Room } from "./types";

const API_PREFIX = resolveLobbyApiPrefix(__LOBBY_API_ORIGIN__);

async function request<T>(accessToken: string, path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${API_PREFIX}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${accessToken}`,
      ...(init.headers ?? {})
    }
  });

  if (!response.ok) {
    let body: ApiErrorBody = {};
    try {
      body = (await response.json()) as ApiErrorBody;
    } catch {
      body = {};
    }

    throw new AuthApiError(
      body.error?.message ?? "Nao foi possivel concluir a operacao.",
      body.error?.code ?? "REQUEST_FAILED",
      response.status,
      body.error?.details ?? []
    );
  }

  return (await response.json()) as T;
}

export function createRoom(accessToken: string, isPrivate: boolean): Promise<Room> {
  return request<Room>(accessToken, "", {
    method: "POST",
    body: JSON.stringify({ is_private: isPrivate })
  });
}

export function listPublicRooms(accessToken: string): Promise<Room[]> {
  return request<Room[]>(accessToken, "/public");
}

export function getRoom(accessToken: string, codeOrId: string): Promise<Room> {
  return request<Room>(accessToken, `/${codeOrId}`);
}

export function joinRoom(accessToken: string, codeOrId: string): Promise<Room> {
  return request<Room>(accessToken, `/${codeOrId}/join`, {
    method: "POST"
  });
}

function resolveLobbyApiPrefix(configuredOrigin: string): string {
  const origin = configuredOrigin.trim().replace(/\/+$/, "");

  if (!origin || origin === "/") {
    return "/api/v1/rooms";
  }

  if (origin.endsWith("/api/v1/rooms")) {
    return origin;
  }

  if (origin.endsWith("/api")) {
    return `${origin}/v1/rooms`;
  }

  return `${origin}/api/v1/rooms`;
}
