import { AuthApiError, type ApiErrorBody } from "../auth/types";
import type { Room, RoomInvite, RoomInviteCleared } from "./types";

const API_PREFIX = resolveLobbyApiPrefix(__LOBBY_API_ORIGIN__);
const API_ROOT = API_PREFIX.replace(/\/rooms$/, "");

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

export function leaveRoom(accessToken: string, codeOrId: string): Promise<Room> {
  return request<Room>(accessToken, `/${codeOrId}/leave`, {
    method: "POST"
  });
}

export function inviteUser(accessToken: string, codeOrId: string, userId: string): Promise<RoomInvite> {
  return request<RoomInvite>(accessToken, `/${codeOrId}/invite`, {
    method: "POST",
    body: JSON.stringify({ user_id: userId })
  });
}

export function declineInvite(accessToken: string, invitationId: string): Promise<RoomInviteCleared | null> {
  return requestFromRoot<RoomInviteCleared | null>(accessToken, `/notifications/invites/${invitationId}/decline`, {
    method: "POST"
  });
}

async function requestFromRoot<T>(accessToken: string, path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${API_ROOT}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${accessToken}`,
      ...(init.headers ?? {})
    }
  });

  if (response.status === 204) {
    return null as T;
  }

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
