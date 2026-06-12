import { createContext, useCallback, useContext, useEffect, useMemo, useReducer, type ReactNode } from "react";
import * as authApi from "./authApi";
import { sessionFromAccessToken } from "./token";
import type { AuthSession, LoginPayload, SignupPayload } from "./types";

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
};

const AuthContext = createContext<AuthContextValue | null>(null);

const initialState: AuthState = {
  phase: "checking",
  session: null,
  error: null
};

function reducer(state: AuthState, action: AuthAction): AuthState {
  switch (action.type) {
    case "BOOTSTRAP_AUTHENTICATED":
    case "AUTHENTICATED":
      return { phase: "authenticated", session: action.session, error: null };
    case "BOOTSTRAP_ANONYMOUS":
    case "ANONYMOUS":
      return { phase: "anonymous", session: null, error: null };
    case "ERROR":
      return { ...state, error: action.error };
    default:
      return state;
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState);

  const refreshSession = useCallback(async () => {
    try {
      const response = await authApi.refresh();
      dispatch({
        type: "BOOTSTRAP_AUTHENTICATED",
        session: sessionFromAccessToken(response.access_token)
      });
    } catch {
      dispatch({ type: "BOOTSTRAP_ANONYMOUS" });
    }
  }, []);

  useEffect(() => {
    void refreshSession();
  }, [refreshSession]);

  const login = useCallback(async (payload: LoginPayload) => {
    dispatch({ type: "ERROR", error: null });
    try {
      const response = await authApi.login(payload);
      dispatch({
        type: "AUTHENTICATED",
        session: {
          accessToken: response.token,
          status: response.status
        }
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
        session: {
          accessToken: response.token,
          status: response.status
        }
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
        session: {
          accessToken: response.token,
          status: response.status
        }
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

  const value = useMemo<AuthContextValue>(
    () => ({
      ...state,
      login,
      signup,
      loginWithGoogleToken,
      logout,
      refreshSession
    }),
    [login, logout, refreshSession, signup, loginWithGoogleToken, state]
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
