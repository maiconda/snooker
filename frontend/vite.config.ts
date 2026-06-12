import path from "node:path";
import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const rootEnv = loadEnv(mode, path.resolve(__dirname, ".."), "");
  const authApiOrigin = rootEnv.VITE_AUTH_API_ORIGIN || rootEnv.AUTH_API_ORIGIN || "http://localhost:8081";
  const googleClientId = rootEnv.VITE_GOOGLE_CLIENT_ID || rootEnv.GOOGLE_CLIENT_ID || "";

  return {
    plugins: [react()],
    envDir: path.resolve(__dirname, ".."),
    define: {
      __AUTH_API_ORIGIN__: JSON.stringify(authApiOrigin),
      __GOOGLE_CLIENT_ID__: JSON.stringify(googleClientId)
    },
    server: {
      port: 3000,
      proxy: {
        "/api": {
          target: authApiOrigin,
          changeOrigin: true
        }
      }
    }
  };
});
