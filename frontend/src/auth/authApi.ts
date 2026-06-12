import { AuthApiError, type ApiErrorBody, type AuthResponse, type LoginPayload, type RefreshResponse, type SignupPayload } from "./types";

const API_PREFIX = "/api/v1/auth";

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`${API_PREFIX}${path}`, {
    ...init,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
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

export function signup(payload: SignupPayload): Promise<AuthResponse> {
  return request<AuthResponse>("/signup", {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export function login(payload: LoginPayload): Promise<AuthResponse> {
  return request<AuthResponse>("/login", {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export function loginWithGoogle(idToken: string): Promise<AuthResponse> {
  return request<AuthResponse>("/google", {
    method: "POST",
    body: JSON.stringify({ id_token: idToken })
  });
}

export function refresh(): Promise<RefreshResponse> {
  return request<RefreshResponse>("/refresh", {
    method: "POST"
  });
}

export function logout(accessToken: string): Promise<{ message: string }> {
  return request<{ message: string }>("/logout", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${accessToken}`
    }
  });
}
