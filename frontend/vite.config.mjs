import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

const dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig(({ mode }) => {
  const rootEnv = { ...loadEnv(mode, path.resolve(dirname, ".."), ""), ...process.env };
  const authApiOrigin = resolveClientAuthOrigin(rootEnv);
  const authProxyTarget = resolveAuthProxyTarget(rootEnv);
  const profileApiOrigin = resolveClientProfileOrigin(rootEnv);
  const profileProxyTarget = resolveProfileProxyTarget(rootEnv);
  const lobbyApiOrigin = resolveClientGameOrigin(rootEnv);
  const lobbyProxyTarget = resolveGameProxyTarget(rootEnv);
  const storageProxyTarget = resolveStorageProxyTarget(rootEnv);
  const googleClientId = rootEnv.VITE_GOOGLE_CLIENT_ID || rootEnv.GOOGLE_CLIENT_ID || "";

  return {
    plugins: [react()],
    envDir: path.resolve(dirname, ".."),
    define: {
      __AUTH_API_ORIGIN__: JSON.stringify(authApiOrigin),
      __PROFILE_API_ORIGIN__: JSON.stringify(profileApiOrigin),
      __LOBBY_API_ORIGIN__: JSON.stringify(lobbyApiOrigin),
      __GOOGLE_CLIENT_ID__: JSON.stringify(googleClientId)
    },
    build: {
      chunkSizeWarningLimit: 1000
    },
    server: {
      port: 3000,
      allowedHosts: true,
      proxy: {
        "/api/v1/rooms": {
          target: lobbyProxyTarget,
          changeOrigin: true,
          ws: true
        },
        "/api/v1/notifications": {
          target: lobbyProxyTarget,
          changeOrigin: true,
          ws: true
        },
        "/api/v1/profiles": {
          target: profileProxyTarget,
          changeOrigin: true
        },
        "/api": {
          target: authProxyTarget,
          changeOrigin: true
        },
        "/snooker-profiles": {
          target: storageProxyTarget,
          changeOrigin: false
        }
      }
    }
  };
});

function resolveClientAuthOrigin(env) {
  if (env.VITE_AUTH_API_ORIGIN) {
    return env.VITE_AUTH_API_ORIGIN;
  }

  if (env.AUTH_API_ORIGIN?.startsWith("/")) {
    return env.AUTH_API_ORIGIN;
  }

  return "/api";
}

function resolveClientProfileOrigin(env) {
  if (env.VITE_PROFILE_API_ORIGIN) {
    return env.VITE_PROFILE_API_ORIGIN;
  }

  if (env.PROFILE_API_ORIGIN?.startsWith("/")) {
    return env.PROFILE_API_ORIGIN;
  }

  return "/api";
}

function resolveClientGameOrigin(env) {
  if (env.VITE_GAME_API_ORIGIN) {
    return env.VITE_GAME_API_ORIGIN;
  }

  if (env.GAME_API_ORIGIN?.startsWith("/")) {
    return env.GAME_API_ORIGIN;
  }

  if (env.VITE_LOBBY_API_ORIGIN) {
    return env.VITE_LOBBY_API_ORIGIN;
  }

  if (env.LOBBY_API_ORIGIN?.startsWith("/")) {
    return env.LOBBY_API_ORIGIN;
  }

  return "/api";
}

function resolveAuthProxyTarget(env) {
  const target = env.VITE_AUTH_PROXY_TARGET || (!env.AUTH_API_ORIGIN?.startsWith("/") ? env.AUTH_API_ORIGIN : "") || "http://localhost:8081";
  return target.replace(/\/api(?:\/v1\/auth)?\/?$/, "");
}

function resolveProfileProxyTarget(env) {
  const target = env.VITE_PROFILE_PROXY_TARGET || (!env.PROFILE_API_ORIGIN?.startsWith("/") ? env.PROFILE_API_ORIGIN : "") || "http://localhost:8082";
  return target.replace(/\/api(?:\/v1\/profiles)?\/?$/, "");
}

function resolveGameProxyTarget(env) {
  const target =
    env.VITE_GAME_PROXY_TARGET ||
    (!env.GAME_API_ORIGIN?.startsWith("/") ? env.GAME_API_ORIGIN : "") ||
    env.VITE_LOBBY_PROXY_TARGET ||
    (!env.LOBBY_API_ORIGIN?.startsWith("/") ? env.LOBBY_API_ORIGIN : "") ||
    "http://localhost:8083";
  return target.replace(/\/api(?:\/v1\/rooms)?\/?$/, "");
}

function resolveStorageProxyTarget(env) {
  const target = env.VITE_STORAGE_PROXY_TARGET || env.STORAGE_PROXY_TARGET || "http://localhost:9005";
  return target.replace(/\/snooker-profiles\/?$/, "");
}
