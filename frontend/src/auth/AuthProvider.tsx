import { createContext, useCallback, useContext, useEffect, useMemo, useReducer, type ReactNode } from "react";
import * as authApi from "./authApi";
import { sessionFromAccessToken } from "./token";
import { AuthApiError, type AuthResponse, type AuthSession, type LoginPayload, type SignupPayload } from "./types";

type AuthState =
  | { phase: "checking"; session: null; error: string | null }
  | { phase: "anonymous"; session: null; error: string | null }
  | { phase: "authenticated"; session: AuthSession; error: string | null };

type AuthAction =
  | { type: "BOOTSTRAP_AUTHENTICATED"; session: AuthSession }
  | { type: "BOOTSTRAP_ANONYMOUS" }
  | { type: "AUTHENTICATED"; session: AuthSession }
  | { type: "ANONYMOUS" }
  | { type: "ERROR"; error: string | null };

type AuthContextValue = AuthState & {
  login: (payload: LoginPayload) => Promise<void>;
  signup: (payload: SignupPayload) => Promise<void>;
  loginWithGoogleToken: (idToken: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshSession: () => Promise<void>;
  acceptAccessToken: (accessToken: string) => void;
};

const AuthContext = createContext<AuthContextValue | null>(null);

const initialState: AuthState = {
  phase: "checking",
  session: null,
  error: null
};

let initialBootstrapPromise: Promise<AuthSession | null> | null = null;

function reducer(state: AuthState, action: AuthAction): AuthState {
  switch (action.type) {
    case "BOOTSTRAP_AUTHENTICATED":
      if (state.phase !== "checking") {
        return state;
      }
      return { phase: "authenticated", session: action.session, error: null };
    case "BOOTSTRAP_ANONYMOUS":
      if (state.phase !== "checking") {
        return state;
      }
      return { phase: "anonymous", session: null, error: null };
    case "AUTHENTICATED":
      return { phase: "authenticated", session: action.session, error: null };
    case "ANONYMOUS":
      return { phase: "anonymous", session: null, error: null };
    case "ERROR":
      return { ...state, error: action.error };
    default:
      return state;
  }
}

function sessionFromAuthResponse(response: AuthResponse): AuthSession {
  return {
    ...sessionFromAccessToken(response.token),
    status: response.status
  };
}

function loadInitialSession(): Promise<AuthSession | null> {
  if (!initialBootstrapPromise) {
    initialBootstrapPromise = authApi
      .refresh()
      .then((response) => sessionFromAccessToken(response.access_token))
      .catch(() => null);
  }

  return initialBootstrapPromise;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState);

  const refreshSession = useCallback(async () => {
    try {
      const response = await authApi.refresh();
      dispatch({
        type: "AUTHENTICATED",
        session: sessionFromAccessToken(response.access_token)
      });
    } catch (err) {
      if (err instanceof AuthApiError) {
        dispatch({ type: "ANONYMOUS" });
      } else {
        console.warn("Falha de rede ao renovar sessao, mantendo sessao atual e tentando em 10s:", err);
        setTimeout(() => {
          refreshSession();
        }, 10000);
      }
    }
  }, []);

  useEffect(() => {
    let active = true;

    loadInitialSession().then((session) => {
      if (!active) {
        return;
      }

      if (session) {
        dispatch({ type: "BOOTSTRAP_AUTHENTICATED", session });
      } else {
        dispatch({ type: "BOOTSTRAP_ANONYMOUS" });
      }
    });

    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    if (state.phase !== "authenticated" || !state.session?.expiresAt) {
      return;
    }

    const expiresAt = state.session.expiresAt;
    const now = Date.now();
    const delay = expiresAt - now - 60000;
    const timeoutDelay = Math.max(0, delay);

    const timer = setTimeout(() => {
      refreshSession();
    }, timeoutDelay);

    return () => clearTimeout(timer);
  }, [state.phase, state.session, refreshSession]);

  const login = useCallback(async (payload: LoginPayload) => {
    dispatch({ type: "ERROR", error: null });
    try {
      const response = await authApi.login(payload);
      dispatch({
        type: "AUTHENTICATED",
        session: sessionFromAuthResponse(response)
      });
    } catch (error) {
      dispatch({ type: "ERROR", error: error instanceof Error ? error.message : "Falha ao entrar." });
      throw error;
    }
  }, []);

  const signup = useCallback(async (payload: SignupPayload) => {
    dispatch({ type: "ERROR", error: null });
    try {
      const response = await authApi.signup(payload);
      dispatch({
        type: "AUTHENTICATED",
        session: sessionFromAuthResponse(response)
      });
    } catch (error) {
      dispatch({ type: "ERROR", error: error instanceof Error ? error.message : "Falha ao cadastrar." });
      throw error;
    }
  }, []);

  const loginWithGoogleToken = useCallback(async (idToken: string) => {
    dispatch({ type: "ERROR", error: null });
    try {
      const response = await authApi.loginWithGoogle(idToken);
      dispatch({
        type: "AUTHENTICATED",
        session: sessionFromAuthResponse(response)
      });
    } catch (error) {
      dispatch({ type: "ERROR", error: error instanceof Error ? error.message : "Falha ao entrar com Google." });
      throw error;
    }
  }, []);

  const logout = useCallback(async () => {
    const token = state.session?.accessToken;
    dispatch({ type: "ERROR", error: null });

    try {
      if (token) {
        await authApi.logout(token);
      }
    } finally {
      dispatch({ type: "ANONYMOUS" });
    }
  }, [state.session?.accessToken]);

  const acceptAccessToken = useCallback((accessToken: string) => {
    dispatch({
      type: "AUTHENTICATED",
      session: sessionFromAccessToken(accessToken)
    });
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      ...state,
      login,
      signup,
      loginWithGoogleToken,
      logout,
      refreshSession,
      acceptAccessToken
    }),
    [login, logout, refreshSession, signup, loginWithGoogleToken, acceptAccessToken, state]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth deve ser usado dentro de AuthProvider.");
  }
  return context;
}
