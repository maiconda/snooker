import { AuthApiError, type ApiErrorBody } from "../auth/types";
import type { CompleteProfileResponse, PhotoUploadURLResponse, Profile, ProfilePayload, UpdateProfilePayload } from "./types";

const API_PREFIX = resolveProfileApiPrefix(__PROFILE_API_ORIGIN__);

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

export function getMyProfile(accessToken: string): Promise<Profile> {
  return request<Profile>(accessToken, "/me");
}

export function getPublicProfile(accessToken: string, userId: string): Promise<Profile> {
  return request<Profile>(accessToken, `/${userId}`);
}

export function createPhotoUploadURL(accessToken: string, contentType: string, fileSize: number): Promise<PhotoUploadURLResponse> {
  return request<PhotoUploadURLResponse>(accessToken, "/me/photo-upload-url", {
    method: "POST",
    body: JSON.stringify({
      content_type: contentType,
      file_size: fileSize
    })
  });
}

export async function uploadPhoto(uploadUrl: string, blob: Blob): Promise<void> {
  try {
    const response = await fetch(uploadUrl, {
      method: "PUT",
      headers: {
        "Content-Type": blob.type
      },
      body: blob
    });

    if (!response.ok) {
      throw new AuthApiError("Nao foi possivel enviar a imagem.", "PHOTO_UPLOAD_FAILED", response.status);
    }
  } catch (err) {
    if (err instanceof TypeError && err.message === "Failed to fetch") {
      throw new Error(`Falha no upload (Failed to fetch). Verifique se a URL de upload (${new URL(uploadUrl).origin}) esta correta, usa HTTPS (se o site for HTTPS) e esta acessivel pelo navegador.`);
    }
    throw err;
  }
}

export function completeProfile(accessToken: string, payload: ProfilePayload): Promise<CompleteProfileResponse> {
  return request<CompleteProfileResponse>(accessToken, "/me/complete", {
    method: "POST",
    body: JSON.stringify(toApiPayload(payload))
  });
}

export function updateProfile(accessToken: string, payload: UpdateProfilePayload): Promise<Profile> {
  return request<Profile>(accessToken, "/me", {
    method: "PATCH",
    body: JSON.stringify(toApiPayload(payload))
  });
}

function toApiPayload(payload: ProfilePayload | UpdateProfilePayload) {
  return {
    nickname: payload.nickname,
    bio: payload.bio,
    photo_upload_id: payload.photo_upload_id
  };
}

function resolveProfileApiPrefix(configuredOrigin: string): string {
  const origin = configuredOrigin.trim().replace(/\/+$/, "");

  if (!origin || origin === "/") {
    return "/api/v1/profiles";
  }

  if (origin.endsWith("/api/v1/profiles")) {
    return origin;
  }

  if (origin.endsWith("/api")) {
    return `${origin}/v1/profiles`;
  }

  return `${origin}/api/v1/profiles`;
}
