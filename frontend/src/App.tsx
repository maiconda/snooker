import { useEffect } from "react";
import { AuthProvider, useAuth } from "./auth/AuthProvider";
import { HomePage } from "./pages/HomePage";
import { LoginPage } from "./pages/LoginPage";
import { SignupPage } from "./pages/SignupPage";
import { RouterEvents, usePathname, navigate } from "./lib/router";

export function App() {
  return (
    <AuthProvider>
      <RouterEvents />
      <Routes />
    </AuthProvider>
  );
}

function Routes() {
  const path = usePathname();
  const auth = useAuth();

  useEffect(() => {
    if (auth.phase === "anonymous" && path !== "/login" && path !== "/cadastro") {
      navigate("/login");
    } else if (auth.phase === "authenticated" && (path === "/login" || path === "/cadastro")) {
      navigate("/");
    }
  }, [auth.phase, path]);

  if (auth.phase === "checking") {
    return (
      <main className="min-h-screen flex items-center justify-center bg-white text-neutral-800" aria-label="Carregando">
        <p className="text-sm font-medium">Carregando...</p>
      </main>
    );
  }

  if (auth.phase === "anonymous" && path !== "/login" && path !== "/cadastro") {
    return null;
  }
  
  if (auth.phase === "authenticated" && (path === "/login" || path === "/cadastro")) {
    return null;
  }

  if (path === "/login") {
    return <LoginPage />;
  }

  if (path === "/cadastro") {
    return <SignupPage />;
  }

  return <HomePage />;
}

