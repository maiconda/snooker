import { useEffect } from "react";
import { AuthProvider, useAuth } from "./auth/AuthProvider";
import { HomePage } from "./pages/HomePage";
import { LoginPage } from "./pages/LoginPage";
import { SignupPage } from "./pages/SignupPage";
import { usePathname, navigate } from "./lib/router";
import { Button } from "./components/Button";

export function App() {
  return (
    <AuthProvider>
      <Routes />
    </AuthProvider>
  );
}

function BlockedPage() {
  const auth = useAuth();
  return (
    <main className="flex min-h-screen flex-col items-center justify-center bg-white px-6 text-black">
      <section className="w-full max-w-[360px] text-center">
        <h1 className="mb-4 text-xl font-medium tracking-normal text-red-600">Acesso Bloqueado</h1>
        <p className="mb-8 text-sm text-neutral-600">
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
      if (path !== "/") {
        navigate("/");
      }
    } else if (auth.phase === "anonymous") {
      if (path !== "/login" && path !== "/cadastro") {
        navigate("/login");
      }
    }
  }, [auth.phase, path]);

  if (auth.phase === "checking") {
    return <main className="min-h-screen bg-white" />;
  }

  if (auth.phase === "authenticated") {
    if (auth.session?.status === "blocked") {
      return <BlockedPage />;
    }

    if (path === "/") {
      return <HomePage />;
    }
    return <main className="min-h-screen bg-white" />;
  }

  if (path === "/login") {
    return <LoginPage />;
  }

  if (path === "/cadastro") {
    return <SignupPage />;
  }

  return <LoginPage />;
}
