import { lazy, Suspense, useEffect } from "react";
import { AuthProvider, useAuth } from "./auth/AuthProvider";
import { HomePage } from "./pages/HomePage";
import { LoginPage } from "./pages/LoginPage";
import { ProfilePage } from "./pages/ProfilePage";
import { SignupPage } from "./pages/SignupPage";
import { LobbyRoomPage } from "./pages/LobbyRoomPage";
import { usePathname, navigate } from "./lib/router";
import { Button } from "./components/Button";
import { LobbyNotificationsProvider } from "./lobby/LobbyNotificationsProvider";
import { ThemeProvider } from "./components/ThemeContext";

const GamePage = lazy(() => import("./pages/GamePage").then((module) => ({ default: module.GamePage })));

export function App() {
  return (
    <ThemeProvider>
      <AuthProvider>
        <LobbyNotificationsProvider>
          <Routes />
        </LobbyNotificationsProvider>
      </AuthProvider>
    </ThemeProvider>
  );
}

function BlockedPage() {
  const auth = useAuth();
  return (
    <main className="flex min-h-screen flex-col items-center justify-center bg-white dark:bg-zinc-950 px-6 text-neutral-900 dark:text-white transition-colors duration-300">
      <section className="w-full max-w-[360px] text-center">
        <h1 className="mb-4 text-xl font-medium tracking-normal text-red-600">Acesso Bloqueado</h1>
        <p className="mb-8 text-sm text-neutral-600 dark:text-neutral-400">
          Sua conta foi bloqueada. Por favor, entre em contato com o suporte para obter mais informações.
        </p>
        <Button onClick={() => auth.logout()}>Sair da conta</Button>
      </section>
    </main>
  );
}

function Routes() {
  const path = usePathname();
  const auth = useAuth();

  useEffect(() => {
    if (auth.phase === "checking") {
      return;
    }

    if (auth.phase === "authenticated") {
      if (auth.session?.status === "onboarding_pending" && path !== "/perfil") {
        navigate("/perfil");
      } else if (auth.session?.status !== "onboarding_pending" && path !== "/" && path !== "/perfil" && !path.startsWith("/sala/") && !path.startsWith("/jogar/")) {
        navigate("/");
      }
    } else if (auth.phase === "anonymous") {
      if (path !== "/login" && path !== "/cadastro") {
        navigate("/login");
      }
    }
  }, [auth.phase, path]);

  if (auth.phase === "checking") {
    return <main className="min-h-screen bg-white dark:bg-zinc-950 transition-colors duration-300" />;
  }

  if (auth.phase === "authenticated") {
    if (auth.session?.status === "blocked") {
      return <BlockedPage />;
    }

    if (path === "/perfil") {
      return <ProfilePage />;
    }

    if (path === "/") {
      return <HomePage />;
    }

    if (path.startsWith("/sala/")) {
      const roomId = path.substring(6);
      return <LobbyRoomPage roomId={roomId} />;
    }
    if (path.startsWith("/jogar/")) {
      const roomId = path.substring(7);
      return (
        <Suspense fallback={<main className="min-h-screen bg-neutral-950" />}>
          <GamePage roomId={roomId} />
        </Suspense>
      );
    }
    return <main className="min-h-screen bg-white dark:bg-zinc-950 transition-colors duration-300" />;
  }

  if (path === "/login") {
    return <LoginPage />;
  }

  if (path === "/cadastro") {
    return <SignupPage />;
  }

  return <LoginPage />;
}
