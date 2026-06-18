import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

const dirname = path.dirname(fileURLToPath(import.meta.url));

export default defineConfig(({ mode }) => {
  const rootEnv = { ...loadEnv(mode, path.resolve(dirname, ".."), ""), ...process.env };
  const authApiOrigin = resolveClientAuthOrigin(rootEnv);
  const authProxyTarget = resolveAuthProxyTarget(rootEnv);
  const googleClientId = rootEnv.VITE_GOOGLE_CLIENT_ID || rootEnv.GOOGLE_CLIENT_ID || "";

  return {
    plugins: [react()],
    envDir: path.resolve(dirname, ".."),
    define: {
      __AUTH_API_ORIGIN__: JSON.stringify(authApiOrigin),
      __GOOGLE_CLIENT_ID__: JSON.stringify(googleClientId)
    },
    server: {
      port: 3000,
      proxy: {
        "/api": {
          target: authProxyTarget,
          changeOrigin: true
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

function resolveAuthProxyTarget(env) {
  const target = env.VITE_AUTH_PROXY_TARGET || (!env.AUTH_API_ORIGIN?.startsWith("/") ? env.AUTH_API_ORIGIN : "") || "http://localhost:8081";
  return target.replace(/\/api(?:\/v1\/auth)?\/?$/, "");
}
