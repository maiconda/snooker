import { useAuth } from "../auth/AuthProvider";
import { Button } from "../components/Button";

export function HomePage() {
  const auth = useAuth();

  return (
    <main className="flex min-h-screen items-center justify-center bg-white px-6 py-10 text-black" aria-label="Pagina inicial">
      <section className="w-full max-w-[360px] space-y-6">
        <div className="space-y-2">
          <h1 className="text-xl font-medium tracking-normal text-black">Página Inicial</h1>
          <p className="text-sm text-neutral-600">Você está logado como: {auth.session?.email}</p>
        </div>

        <Button onClick={() => auth.logout()} variant="outline">
          Sair da conta
        </Button>
      </section>
    </main>
  );
}


