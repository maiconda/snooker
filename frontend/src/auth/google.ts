const GOOGLE_AUTH_URL = "https://accounts.google.com/o/oauth2/v2/auth";
const GOOGLE_STATE_KEY = "snooker.google.state";
const GOOGLE_NONCE_KEY = "snooker.google.nonce";

export type GoogleCallbackResult =
  | { ok: true; idToken: string }
  | { ok: false; error: string }
  | { ok: false; error: null };

export function startGoogleLogin(): void {
  if (!__GOOGLE_CLIENT_ID__) {
    throw new Error("Google Client ID nao configurado.");
  }

  const state = randomValue();
  const nonce = randomValue();
  sessionStorage.setItem(GOOGLE_STATE_KEY, state);
  sessionStorage.setItem(GOOGLE_NONCE_KEY, nonce);

  const origin = window.location.origin.replace("127.0.0.1", "localhost");
  const redirectUri = `${origin}/login`;
  const url = new URL(GOOGLE_AUTH_URL);
  url.searchParams.set("client_id", __GOOGLE_CLIENT_ID__);
  url.searchParams.set("redirect_uri", redirectUri);
  url.searchParams.set("response_type", "id_token");
  url.searchParams.set("scope", "openid email profile");
  url.searchParams.set("state", state);
  url.searchParams.set("nonce", nonce);
  url.searchParams.set("prompt", "select_account");

  window.location.assign(url.toString());
}

export function consumeGoogleCallback(): GoogleCallbackResult {
  if (!window.location.hash) {
    return { ok: false, error: null };
  }

  const params = new URLSearchParams(window.location.hash.slice(1));
  const idToken = params.get("id_token");
  const state = params.get("state");
  const error = params.get("error");

  window.history.replaceState(null, document.title, `${window.location.pathname}${window.location.search}`);

  if (error) {
    clearGoogleLoginState();
    return { ok: false, error };
  }

  if (!idToken || !state) {
    clearGoogleLoginState();
    return { ok: false, error: "Resposta do Google incompleta." };
  }

  const expectedState = sessionStorage.getItem(GOOGLE_STATE_KEY);
  clearGoogleLoginState();

  if (!expectedState || expectedState !== state) {
    return { ok: false, error: "Estado de autenticacao invalido." };
  }

  return { ok: true, idToken };
}

function clearGoogleLoginState(): void {
  sessionStorage.removeItem(GOOGLE_STATE_KEY);
  sessionStorage.removeItem(GOOGLE_NONCE_KEY);
}

function randomValue(): string {
  const bytes = new Uint8Array(24);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
}
